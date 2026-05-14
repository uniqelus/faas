// Package pkghttp provides a reusable HTTP server component with graceful
// shutdown semantics matching pkg/grpc, plus a separate admin listener for
// operator endpoints such as /metrics and /healthz.
package pkghttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// ServerComponent runs a main HTTP server and, unless explicitly disabled, an
// independent admin HTTP server exposing /metrics and /healthz. Both servers
// share a single graceful shutdown path so in-flight requests are allowed to
// finish while new connections are rejected.
type ServerComponent struct {
	log             *zap.Logger
	server          *http.Server
	adminServer     *http.Server
	address         atomic.Pointer[string]
	adminAddress    atomic.Pointer[string]
	adminDisabled   bool
	shutdownTimeout time.Duration
}

// NewServerComponent assembles a [ServerComponent] from the given options.
func NewServerComponent(opts ...ServerComponentOption) *ServerComponent {
	options := newServerComponentOptions(opts...)

	componentLog := options.log.With(zap.String("component", "http-server"))

	mainAddr := net.JoinHostPort(options.host, options.port)
	server := &http.Server{
		Addr:              mainAddr,
		Handler:           options.handler,
		ReadHeaderTimeout: options.readHeaderTimeout,
		ReadTimeout:       options.readTimeout,
		WriteTimeout:      options.writeTimeout,
		IdleTimeout:       options.idleTimeout,
	}

	var (
		adminServer *http.Server
		adminAddr   string
	)
	if !options.adminDisabled {
		adminAddr = net.JoinHostPort(options.adminHost, options.adminPort)
		adminMux := http.NewServeMux()
		adminMux.Handle("/healthz", options.healthzHandler)
		adminMux.Handle("/metrics", options.metricsHandler)
		adminServer = &http.Server{
			Addr:              adminAddr,
			Handler:           adminMux,
			ReadHeaderTimeout: options.readHeaderTimeout,
			ReadTimeout:       options.readTimeout,
			WriteTimeout:      options.writeTimeout,
			IdleTimeout:       options.idleTimeout,
		}
	}

	c := &ServerComponent{
		log:             componentLog,
		server:          server,
		adminServer:     adminServer,
		adminDisabled:   options.adminDisabled,
		shutdownTimeout: options.shutdownTimeout,
	}
	c.address.Store(&mainAddr)
	c.adminAddress.Store(&adminAddr)
	return c
}

// Address returns the resolved address of the main listener. It is populated
// after [ServerComponent.Start] binds the listener, so the value also reflects
// any port that was assigned dynamically (for tests using ":0").
func (c *ServerComponent) Address() string {
	if p := c.address.Load(); p != nil {
		return *p
	}
	return ""
}

// AdminAddress returns the resolved address of the admin listener, or an empty
// string when the admin listener is disabled.
func (c *ServerComponent) AdminAddress() string {
	if p := c.adminAddress.Load(); p != nil {
		return *p
	}
	return ""
}

// Start binds the configured listeners and serves requests until the context
// is canceled or one of the servers fails. It mirrors the semantics of
// pkg/grpc.ServerComponent.Start: a canceled context yields a nil return,
// while an unexpected serve error is propagated.
func (c *ServerComponent) Start(ctx context.Context) error {
	configuredMain := c.Address()
	configuredAdmin := c.AdminAddress()

	c.log.Info("starting server",
		zap.String("address", configuredMain),
		zap.String("admin_address", configuredAdmin),
		zap.Bool("admin_disabled", c.adminDisabled),
	)

	listener, err := net.Listen("tcp", configuredMain)
	if err != nil {
		c.log.Error("failed to listen", zap.Error(err))
		return fmt.Errorf("listen %s: %w", configuredMain, err)
	}
	resolvedMain := listener.Addr().String()
	c.address.Store(&resolvedMain)
	c.server.Addr = resolvedMain

	var adminListener net.Listener
	if c.adminServer != nil {
		adminListener, err = net.Listen("tcp", configuredAdmin)
		if err != nil {
			_ = listener.Close()
			c.log.Error("failed to listen on admin", zap.Error(err))
			return fmt.Errorf("listen admin %s: %w", configuredAdmin, err)
		}
		resolvedAdmin := adminListener.Addr().String()
		c.adminAddress.Store(&resolvedAdmin)
		c.adminServer.Addr = resolvedAdmin
	}

	serverErrCh := make(chan error, 1)
	go func() {
		err := c.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverErrCh <- err
	}()

	adminErrCh := make(chan error, 1)
	if adminListener != nil {
		go func() {
			err := c.adminServer.Serve(adminListener)
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			adminErrCh <- err
		}()
	}

	select {
	case err := <-serverErrCh:
		if err != nil {
			c.log.Error("server failed", zap.Error(err))
		}
		return err
	case err := <-adminErrCh:
		if err != nil {
			c.log.Error("admin server failed", zap.Error(err))
		}
		return err
	case <-ctx.Done():
		c.log.Info("context done, stopping server")
		return nil
	}
}

// Stop drains both servers, allowing in-flight requests to complete and
// rejecting new ones. The shutdown is bounded by the supplied context as well
// as the optional shutdownTimeout.
func (c *ServerComponent) Stop(ctx context.Context) error {
	c.log.Info("stopping server")

	if c.shutdownTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.shutdownTimeout)
		defer cancel()
	}

	type result struct {
		name string
		err  error
	}
	results := make(chan result, 2)
	pending := 1

	go func() {
		results <- result{name: "main", err: c.server.Shutdown(ctx)}
	}()
	if c.adminServer != nil {
		pending++
		go func() {
			results <- result{name: "admin", err: c.adminServer.Shutdown(ctx)}
		}()
	}

	var firstErr error
	for i := 0; i < pending; i++ {
		r := <-results
		if r.err != nil && firstErr == nil {
			firstErr = fmt.Errorf("shutdown %s server: %w", r.name, r.err)
		}
	}
	if firstErr != nil {
		c.log.Error("failed to stop gracefully", zap.Error(firstErr))
		return firstErr
	}

	c.log.Info("server stopped gracefully")
	return nil
}
