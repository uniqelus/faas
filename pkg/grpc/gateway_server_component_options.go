package pkggrpc

import (
	"context"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pkghttp "github.com/uniqelus/faas/pkg/http"
)

const (
	// DefaultGatewayBufconnSize matches the buffer size used by the upstream
	// grpc-gateway examples. It is enough to cover routine control-plane
	// payloads without forcing a producer to block.
	DefaultGatewayBufconnSize = 1024 * 1024
)

// GatewayHandlerRegistration registers a grpc-gateway HTTP handler against the
// shared runtime mux, using the provided in-process client connection.
type GatewayHandlerRegistration func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error

type gatewayServerComponentOptions struct {
	log                  *zap.Logger
	bufconnSize          int
	serverOptions        []grpc.ServerOption
	serviceRegistrations []ServiceRegistration
	dialOptions          []grpc.DialOption
	handlerRegistrations []GatewayHandlerRegistration
	muxOptions           []runtime.ServeMuxOption
	httpServerOptions    []pkghttp.ServerComponentOption
}

func defaultGatewayServerComponentOptions() []GatewayServerComponentOption {
	return []GatewayServerComponentOption{
		WithGatewayLog(zap.NewNop()),
		WithGatewayBufconnSize(DefaultGatewayBufconnSize),
	}
}

func newGatewayServerComponentOptions(opts ...GatewayServerComponentOption) *gatewayServerComponentOptions {
	options := &gatewayServerComponentOptions{}

	toApply := append(defaultGatewayServerComponentOptions(), opts...)
	for _, opt := range toApply {
		opt(options)
	}

	return options
}

// GatewayServerComponentOption configures a [GatewayServerComponent].
type GatewayServerComponentOption func(*gatewayServerComponentOptions)

// WithGatewayLog overrides the logger used by the gateway component.
func WithGatewayLog(log *zap.Logger) GatewayServerComponentOption {
	return func(opts *gatewayServerComponentOptions) {
		opts.log = log
	}
}

// WithGatewayBufconnSize overrides the buffer size used by the in-process
// gRPC listener.
func WithGatewayBufconnSize(size int) GatewayServerComponentOption {
	return func(opts *gatewayServerComponentOptions) {
		opts.bufconnSize = size
	}
}

// WithGatewayServerOptions appends gRPC server options applied to the
// in-process gRPC server fronted by the gateway. This is the place to wire
// OpenTelemetry server interceptors and any other middleware that should
// observe gateway-originated traffic.
func WithGatewayServerOptions(serverOptions ...grpc.ServerOption) GatewayServerComponentOption {
	return func(opts *gatewayServerComponentOptions) {
		opts.serverOptions = append(opts.serverOptions, serverOptions...)
	}
}

// WithGatewayServiceRegistrations registers gRPC service implementations on
// the in-process gRPC server fronted by the gateway.
func WithGatewayServiceRegistrations(registrations ...ServiceRegistration) GatewayServerComponentOption {
	return func(opts *gatewayServerComponentOptions) {
		opts.serviceRegistrations = append(opts.serviceRegistrations, registrations...)
	}
}

// WithGatewayDialOptions appends gRPC dial options used by the in-process
// client connection that the gateway uses to reach the in-process server.
func WithGatewayDialOptions(dialOptions ...grpc.DialOption) GatewayServerComponentOption {
	return func(opts *gatewayServerComponentOptions) {
		opts.dialOptions = append(opts.dialOptions, dialOptions...)
	}
}

// WithGatewayHandlerRegistrations registers grpc-gateway handlers on the
// shared mux. Each registration receives the gateway runtime mux and the
// in-process client connection.
func WithGatewayHandlerRegistrations(registrations ...GatewayHandlerRegistration) GatewayServerComponentOption {
	return func(opts *gatewayServerComponentOptions) {
		opts.handlerRegistrations = append(opts.handlerRegistrations, registrations...)
	}
}

// WithGatewayMuxOptions appends grpc-gateway runtime mux options. Use this to
// set metadata propagation, marshalers, error handlers, etc.
func WithGatewayMuxOptions(muxOptions ...runtime.ServeMuxOption) GatewayServerComponentOption {
	return func(opts *gatewayServerComponentOptions) {
		opts.muxOptions = append(opts.muxOptions, muxOptions...)
	}
}

// WithGatewayHTTPServerOptions forwards options to the underlying
// [pkghttp.ServerComponent]. The gateway component owns the handler wiring;
// supplying [pkghttp.WithHandler] here has no effect.
func WithGatewayHTTPServerOptions(httpOptions ...pkghttp.ServerComponentOption) GatewayServerComponentOption {
	return func(opts *gatewayServerComponentOptions) {
		opts.httpServerOptions = append(opts.httpServerOptions, httpOptions...)
	}
}
