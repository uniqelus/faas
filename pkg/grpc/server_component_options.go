package pkggrpc

import (
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

const (
	DefaultServerCopmonentHost = "0.0.0.0"
	DefaultServerCopmonentPort = "50051"
)

type serverComponentOptions struct {
	host                 string
	port                 string
	log                  *zap.Logger
	serverOptions        []grpc.ServerOption
	serviceRegistrations []ServiceRegistration
}

func defaultServerComponentOptions() []ServerComponentOption {
	return []ServerComponentOption{
		WithHost(DefaultServerCopmonentHost),
		WithPort(DefaultServerCopmonentPort),
		WithLog(zap.NewNop()),
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

func WithLog(log *zap.Logger) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.log = log
	}
}

func WithServerOptions(serverOptions ...grpc.ServerOption) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.serverOptions = serverOptions
	}
}

func WithServiceRegistrations(serviceRegistrations ...ServiceRegistration) ServerComponentOption {
	return func(opts *serverComponentOptions) {
		opts.serviceRegistrations = serviceRegistrations
	}
}
