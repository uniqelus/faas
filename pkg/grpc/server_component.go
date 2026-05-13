package pkggrpc

import (
	"context"
	"fmt"
	"net"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type ServiceRegistration func(*grpc.Server)

type ServerComponent struct {
	log     *zap.Logger
	server  *grpc.Server
	address string
}

func NewServerComponent(opts ...ServerComponentOption) *ServerComponent {
	options := newServerComponentOptions(opts...)

	server := grpc.NewServer(options.serverOptions...)
	for _, registration := range options.serviceRegistrations {
		registration(server)
	}

	return &ServerComponent{
		address: net.JoinHostPort(options.host, options.port),
		log:     options.log.With(zap.String("component", "grpc-server")),
		server:  server,
	}
}

func (c *ServerComponent) Start(ctx context.Context) error {
	c.log.Info("starting server", zap.String("address", c.address))

	listener, err := net.Listen("tcp", c.address)
	if err != nil {
		c.log.Error("failed to listen", zap.Error(err))
		return err
	}

	errCh := make(chan error)
	go func() {
		select {
		case errCh <- c.server.Serve(listener):
		case <-ctx.Done():
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		c.log.Error("failed to serve", zap.Error(err))
		return err
	case <-ctx.Done():
		c.log.Info("context done, stopping server")
		return nil
	}
}

func (c *ServerComponent) Stop(ctx context.Context) error {
	c.log.Info("stopping server")

	done := make(chan struct{})
	go func() {
		c.server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		c.log.Info("server stopped gracefully")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("failed to stop gracefully: %w", ctx.Err())
	}
}
