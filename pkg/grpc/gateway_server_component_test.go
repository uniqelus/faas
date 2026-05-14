package pkggrpc_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	faasv1 "github.com/uniqelus/faas/pkg/api/faas/v1"
	pkggrpc "github.com/uniqelus/faas/pkg/grpc"
	pkghttp "github.com/uniqelus/faas/pkg/http"
)

// mockFunctionService satisfies the FunctionServiceServer contract with
// behavior controllable by individual tests.
type mockFunctionService struct {
	faasv1.UnimplementedFunctionServiceServer

	mu        sync.Mutex
	createReq *faasv1.CreateFunctionRequest

	logsToSend []*faasv1.LogEntry
	logsErr    error
}

func (m *mockFunctionService) Create(_ context.Context, req *faasv1.CreateFunctionRequest) (*faasv1.CreateFunctionResponse, error) {
	m.mu.Lock()
	m.createReq = req
	m.mu.Unlock()

	echoed := req.GetFunction()
	if echoed == nil {
		echoed = &faasv1.Function{}
	}
	echoed.Version = 1
	echoed.CreatedAt = timestamppb.New(time.Unix(1_700_000_000, 0).UTC())
	echoed.UpdatedAt = echoed.CreatedAt
	return &faasv1.CreateFunctionResponse{Function: echoed}, nil
}

func (m *mockFunctionService) GetLogs(_ *faasv1.GetFunctionLogsRequest, stream grpc.ServerStreamingServer[faasv1.LogEntry]) error {
	for _, entry := range m.logsToSend {
		if err := stream.Send(entry); err != nil {
			return err
		}
	}
	return m.logsErr
}

func startGateway(t *testing.T, opts ...pkggrpc.GatewayServerComponentOption) (*pkggrpc.GatewayServerComponent, context.CancelFunc) {
	t.Helper()

	log := zaptest.NewLogger(t)
	baseHTTPOpts := []pkghttp.ServerComponentOption{
		pkghttp.WithHost("127.0.0.1"),
		pkghttp.WithPort("0"),
		pkghttp.WithAdminHost("127.0.0.1"),
		pkghttp.WithAdminPort("0"),
		pkghttp.WithShutdownTimeout(2 * time.Second),
	}
	opts = append([]pkggrpc.GatewayServerComponentOption{
		pkggrpc.WithGatewayLog(log),
		pkggrpc.WithGatewayHTTPServerOptions(baseHTTPOpts...),
		pkggrpc.WithGatewayHandlerRegistrations(faasv1.RegisterFunctionServiceHandler),
	}, opts...)

	comp, err := pkggrpc.NewGatewayServerComponent(opts...)
	require.NoError(t, err)

	startCtx, cancel := context.WithCancel(context.Background())
	startDone := make(chan error, 1)
	go func() { startDone <- comp.Start(startCtx) }()

	require.Eventually(t, func() bool {
		addr := comp.Address()
		if addr == "" || strings.HasSuffix(addr, ":0") {
			return false
		}
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 2*time.Second, 10*time.Millisecond, "gateway never started accepting connections")

	stop := func() {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		if err := comp.Stop(stopCtx); err != nil {
			t.Logf("stop error: %v", err)
		}
		select {
		case <-startDone:
		case <-time.After(3 * time.Second):
			t.Errorf("Start did not return after Stop")
		}
	}
	return comp, stop
}

func TestGatewayServerComponent_ProxiesUnaryRPC(t *testing.T) {
	t.Parallel()

	mock := &mockFunctionService{}
	register := func(s *grpc.Server) {
		faasv1.RegisterFunctionServiceServer(s, mock)
	}

	comp, stop := startGateway(t,
		pkggrpc.WithGatewayServiceRegistrations(register),
	)
	t.Cleanup(stop)

	body := bytes.NewBufferString(`{"name":"demo","image":"ghcr.io/example/demo:1.0","replicas":3}`)
	url := fmt.Sprintf("http://%s/v1/functions", comp.Address())
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "response: %s", string(respBody))

	var decoded struct {
		Function struct {
			Name     string `json:"name"`
			Image    string `json:"image"`
			Replicas int32  `json:"replicas"`
			Version  string `json:"version"`
		} `json:"function"`
	}
	require.NoError(t, json.Unmarshal(respBody, &decoded))
	assert.Equal(t, "demo", decoded.Function.Name)
	assert.Equal(t, "ghcr.io/example/demo:1.0", decoded.Function.Image)
	assert.Equal(t, int32(3), decoded.Function.Replicas)
	assert.Equal(t, "1", decoded.Function.Version)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	require.NotNil(t, mock.createReq)
	assert.Equal(t, "demo", mock.createReq.GetFunction().GetName())
}

func TestGatewayServerComponent_ServerStreamMapsToChunkedHTTP(t *testing.T) {
	t.Parallel()

	mock := &mockFunctionService{
		logsToSend: []*faasv1.LogEntry{
			{Pod: "pod-a", Line: "line-1", Timestamp: timestamppb.New(time.Unix(1_700_000_001, 0).UTC())},
			{Pod: "pod-a", Line: "line-2", Timestamp: timestamppb.New(time.Unix(1_700_000_002, 0).UTC())},
			{Pod: "pod-b", Line: "line-3", Timestamp: timestamppb.New(time.Unix(1_700_000_003, 0).UTC())},
		},
	}
	register := func(s *grpc.Server) {
		faasv1.RegisterFunctionServiceServer(s, mock)
	}

	comp, stop := startGateway(t,
		pkggrpc.WithGatewayServiceRegistrations(register),
	)
	t.Cleanup(stop)

	url := fmt.Sprintf("http://%s/v1/functions/demo/logs", comp.Address())
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, []string{"chunked"}, resp.TransferEncoding,
		"grpc-gateway must map server streaming to HTTP chunked encoding")

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)

	type chunk struct {
		Result struct {
			Pod  string `json:"pod"`
			Line string `json:"line"`
		} `json:"result"`
		Error any `json:"error"`
	}

	var lines []string
	for scanner.Scan() {
		var c chunk
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &c), "raw line: %s", scanner.Text())
		if c.Result.Line == "" {
			continue
		}
		lines = append(lines, c.Result.Pod+":"+c.Result.Line)
	}
	require.NoError(t, scanner.Err())
	assert.Equal(t, []string{
		"pod-a:line-1",
		"pod-a:line-2",
		"pod-b:line-3",
	}, lines)
}

func TestGatewayServerComponent_InterceptorPropagatesContext(t *testing.T) {
	t.Parallel()

	const traceHeader = "X-Trace-Id"
	const expectedTrace = "trace-from-http-001"

	mock := &mockFunctionService{}
	register := func(s *grpc.Server) {
		faasv1.RegisterFunctionServiceServer(s, mock)
	}

	type observation struct {
		ctx      context.Context
		metadata metadata.MD
	}
	observed := make(chan observation, 1)

	interceptor := func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		select {
		case observed <- observation{ctx: ctx, metadata: md.Copy()}:
		default:
		}
		_ = info
		return handler(ctx, req)
	}

	// Forward the trace header into outgoing gRPC metadata so the server-side
	// interceptor can verify that context flows end-to-end.
	muxOpt := runtime.WithMetadata(func(_ context.Context, req *http.Request) metadata.MD {
		if v := req.Header.Get(traceHeader); v != "" {
			return metadata.Pairs(strings.ToLower(traceHeader), v)
		}
		return metadata.MD{}
	})

	comp, stop := startGateway(t,
		pkggrpc.WithGatewayServiceRegistrations(register),
		pkggrpc.WithGatewayServerOptions(grpc.UnaryInterceptor(interceptor)),
		pkggrpc.WithGatewayMuxOptions(muxOpt),
	)
	t.Cleanup(stop)

	url := fmt.Sprintf("http://%s/v1/functions", comp.Address())
	body := bytes.NewBufferString(`{"name":"trace-demo","image":"img:latest"}`)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(traceHeader, expectedTrace)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	select {
	case obs := <-observed:
		require.NotNil(t, obs.ctx)
		got := obs.metadata.Get(strings.ToLower(traceHeader))
		assert.Equal(t, []string{expectedTrace}, got,
			"interceptor must observe trace metadata propagated from HTTP layer")
	case <-time.After(2 * time.Second):
		t.Fatal("interceptor never observed the RPC")
	}
}

func TestGatewayServerComponent_BadRegistrationReturnsError(t *testing.T) {
	t.Parallel()

	failingReg := func(_ context.Context, _ *runtime.ServeMux, _ *grpc.ClientConn) error {
		return errors.New("boom")
	}
	_, err := pkggrpc.NewGatewayServerComponent(
		pkggrpc.WithGatewayHandlerRegistrations(failingReg),
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "boom")
}
