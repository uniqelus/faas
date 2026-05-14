package pkghttp_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	pkghttp "github.com/uniqelus/faas/pkg/http"
)

// startServer launches the component and waits until it accepts connections on
// both listeners. The returned cancel function tears the server down
// gracefully.
func startServer(t *testing.T, opts ...pkghttp.ServerComponentOption) (*pkghttp.ServerComponent, context.CancelFunc) {
	t.Helper()

	log := zaptest.NewLogger(t)
	opts = append([]pkghttp.ServerComponentOption{
		pkghttp.WithLog(log),
		pkghttp.WithHost("127.0.0.1"),
		pkghttp.WithPort("0"),
		pkghttp.WithAdminHost("127.0.0.1"),
		pkghttp.WithAdminPort("0"),
		pkghttp.WithShutdownTimeout(2 * time.Second),
	}, opts...)
	comp := pkghttp.NewServerComponent(opts...)

	startCtx, cancel := context.WithCancel(context.Background())
	startDone := make(chan error, 1)
	go func() {
		startDone <- comp.Start(startCtx)
	}()

	require.Eventually(t, func() bool {
		if comp.Address() == "" || comp.Address() == "127.0.0.1:0" {
			return false
		}
		conn, err := net.DialTimeout("tcp", comp.Address(), 100*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 2*time.Second, 10*time.Millisecond, "main listener never accepted connections")

	stop := func() {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		if err := comp.Stop(stopCtx); err != nil {
			t.Logf("stop error: %v", err)
		}
		select {
		case err := <-startDone:
			if err != nil {
				t.Logf("start returned: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Errorf("start did not return after stop")
		}
	}
	return comp, stop
}

func TestServerComponent_AdminListenerExposesHealthzAndMetrics(t *testing.T) {
	t.Parallel()

	comp, stop := startServer(t)
	t.Cleanup(stop)

	require.NotEmpty(t, comp.AdminAddress(), "admin address must be set")

	cases := []struct {
		name     string
		path     string
		wantCode int
	}{
		{"healthz", "/healthz", http.StatusOK},
		{"metrics", "/metrics", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := fmt.Sprintf("http://%s%s", comp.AdminAddress(), tc.path)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)
			assert.Equal(t, tc.wantCode, resp.StatusCode)
		})
	}
}

func TestServerComponent_GracefulShutdownWaitsForInFlight(t *testing.T) {
	t.Parallel()

	const handlerDelay = 300 * time.Millisecond

	var handlerStarted = make(chan struct{}, 1)
	handler := http.NewServeMux()
	handler.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		select {
		case handlerStarted <- struct{}{}:
		default:
		}
		time.Sleep(handlerDelay)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("served"))
	})

	comp, stop := startServer(t, pkghttp.WithHandler(handler))
	t.Cleanup(stop)

	addr := comp.Address()

	type clientResult struct {
		body string
		err  error
	}
	resultCh := make(chan clientResult, 1)

	client := &http.Client{Timeout: 5 * time.Second}
	go func() {
		req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/slow", nil)
		resp, err := client.Do(req)
		if err != nil {
			resultCh <- clientResult{err: err}
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		resultCh <- clientResult{body: string(body), err: err}
	}()

	select {
	case <-handlerStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("slow handler did not start")
	}

	// Initiate graceful shutdown while the request is still in-flight.
	stopDone := make(chan error, 1)
	go func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		stopDone <- comp.Stop(stopCtx)
	}()

	// Wait a brief moment for Shutdown to start refusing new connections.
	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err != nil {
			return true
		}
		_ = conn.Close()
		return false
	}, 2*time.Second, 20*time.Millisecond, "listener kept accepting connections after Stop")

	// Verify a freshly attempted request is rejected.
	rejectClient := &http.Client{Timeout: 500 * time.Millisecond}
	_, rejectErr := rejectClient.Get("http://" + addr + "/slow")
	require.Error(t, rejectErr, "expected new connection to be rejected during shutdown")

	// Original in-flight request must complete successfully.
	select {
	case res := <-resultCh:
		require.NoError(t, res.err)
		assert.Equal(t, "served", res.body)
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight request did not complete")
	}

	// Graceful shutdown must report success.
	select {
	case err := <-stopDone:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish")
	}
}

func TestServerComponent_ShutdownTimeoutPropagates(t *testing.T) {
	t.Parallel()

	handler := http.NewServeMux()
	hold := make(chan struct{})
	released := make(chan struct{})
	handler.HandleFunc("/hold", func(w http.ResponseWriter, r *http.Request) {
		<-hold
		w.WriteHeader(http.StatusOK)
		close(released)
		_ = r
	})

	log := zaptest.NewLogger(t)
	comp := pkghttp.NewServerComponent(
		pkghttp.WithLog(log),
		pkghttp.WithHost("127.0.0.1"),
		pkghttp.WithPort("0"),
		pkghttp.WithAdminHost("127.0.0.1"),
		pkghttp.WithAdminPort("0"),
		pkghttp.WithHandler(handler),
		pkghttp.WithShutdownTimeout(100*time.Millisecond),
	)

	startCtx, cancel := context.WithCancel(context.Background())
	startDone := make(chan error, 1)
	go func() { startDone <- comp.Start(startCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-hold:
		default:
			close(hold)
		}
		<-startDone
	})

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", comp.Address(), 100*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 2*time.Second, 10*time.Millisecond)

	// Start a request that will hang inside the handler.
	clientCh := make(chan error, 1)
	go func() {
		req, _ := http.NewRequest(http.MethodGet, "http://"+comp.Address()+"/hold", nil)
		resp, err := http.DefaultClient.Do(req)
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		clientCh <- err
	}()

	time.Sleep(50 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer stopCancel()
	err := comp.Stop(stopCtx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "expected DeadlineExceeded, got %v", err)

	_ = released
}

func TestServerComponent_AdminDisabled(t *testing.T) {
	t.Parallel()

	log := zaptest.NewLogger(t)
	comp := pkghttp.NewServerComponent(
		pkghttp.WithLog(log),
		pkghttp.WithHost("127.0.0.1"),
		pkghttp.WithPort("0"),
		pkghttp.WithAdminDisabled(true),
	)

	startCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startDone := make(chan error, 1)
	go func() { startDone <- comp.Start(startCtx) }()

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", comp.Address(), 100*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 2*time.Second, 10*time.Millisecond)

	assert.Empty(t, comp.AdminAddress(), "admin address must be empty when disabled")

	cancel()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	require.NoError(t, comp.Stop(stopCtx))
	<-startDone
}

func TestServerComponentConfig_Options(t *testing.T) {
	t.Parallel()

	cfg := pkghttp.ServerComponentConfig{
		Host:              "127.0.0.1",
		Port:              "0",
		AdminHost:         "127.0.0.1",
		AdminPort:         "0",
		AdminDisabled:     false,
		ReadHeaderTimeout: 1 * time.Second,
		ReadTimeout:       2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	}

	opts := cfg.Options()
	assert.NotEmpty(t, opts)
	comp := pkghttp.NewServerComponent(opts...)
	assert.NotNil(t, comp)
}
