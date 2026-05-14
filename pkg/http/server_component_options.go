package pkghttp

import (
	"expvar"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const (
	DefaultServerComponentHost              = "0.0.0.0"
	DefaultServerComponentPort              = "8080"
	DefaultServerComponentAdminHost         = "0.0.0.0"
	DefaultServerComponentAdminPort         = "8081"
	DefaultServerComponentReadHeaderTimeout = 5 * time.Second
	DefaultServerComponentReadTimeout       = 30 * time.Second
	DefaultServerComponentWriteTimeout      = 30 * time.Second
	DefaultServerComponentIdleTimeout       = 120 * time.Second
	DefaultServerComponentShutdownTimeout   = 30 * time.Second
)

type serverComponentOptions struct {
	host              string
	port              string
	adminHost         string
	adminPort         string
	adminDisabled     bool
	log               *zap.Logger
	handler           http.Handler
	metricsHandler    http.Handler
	healthzHandler    http.Handler
	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	shutdownTimeout   time.Duration
}

func defaultServerComponentOptions() []ServerComponentOption {
	return []ServerComponentOption{
		WithHost(DefaultServerComponentHost),
		WithPort(DefaultServerComponentPort),
		WithAdminHost(DefaultServerComponentAdminHost),
		WithAdminPort(DefaultServerComponentAdminPort),
		WithLog(zap.NewNop()),
		WithHandler(http.NewServeMux()),
		WithMetricsHandler(expvar.Handler()),
		WithHealthzHandler(defaultHealthzHandler()),
		WithReadHeaderTimeout(DefaultServerComponentReadHeaderTimeout),
		WithReadTimeout(DefaultServerComponentReadTimeout),
		WithWriteTimeout(DefaultServerComponentWriteTimeout),
		WithIdleTimeout(DefaultServerComponentIdleTimeout),
		WithShutdownTimeout(DefaultServerComponentShutdownTimeout),
	}
}

func newServerComponentOptions(opts ...ServerComponentOption) *serverComponentOptions {
	options := &serverComponentOptions{}

	toApply := append(defaultServerComponentOptions(), opts...)
	for _, opt := range toApply {
		opt(options)
	}

	return options
}

// ServerComponentOption configures a [ServerComponent].
type ServerComponentOption func(*serverComponentOptions)

func WithHost(host string) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.host = host
	}
}

func WithPort(port string) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.port = port
	}
}

func WithAdminHost(host string) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.adminHost = host
	}
}

func WithAdminPort(port string) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.adminPort = port
	}
}

// WithAdminDisabled disables the admin listener entirely. When set, no admin
// HTTP server is started and /metrics and /healthz are not exposed.
func WithAdminDisabled(disabled bool) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.adminDisabled = disabled
	}
}

func WithLog(log *zap.Logger) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.log = log
	}
}

// WithHandler sets the handler used by the main HTTP server.
func WithHandler(handler http.Handler) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.handler = handler
	}
}

// WithMetricsHandler overrides the handler served at /metrics on the admin
// listener.
func WithMetricsHandler(handler http.Handler) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.metricsHandler = handler
	}
}

// WithHealthzHandler overrides the handler served at /healthz on the admin
// listener.
func WithHealthzHandler(handler http.Handler) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.healthzHandler = handler
	}
}

func WithReadHeaderTimeout(d time.Duration) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.readHeaderTimeout = d
	}
}

func WithReadTimeout(d time.Duration) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.readTimeout = d
	}
}

func WithWriteTimeout(d time.Duration) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.writeTimeout = d
	}
}

func WithIdleTimeout(d time.Duration) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.idleTimeout = d
	}
}

// WithShutdownTimeout sets a hard cap on graceful shutdown. The deadline is
// applied on top of the context passed to [ServerComponent.Stop].
func WithShutdownTimeout(d time.Duration) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.shutdownTimeout = d
	}
}

func defaultHealthzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}
