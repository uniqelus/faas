package pkggrpc

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pkghttp "github.com/uniqelus/faas/pkg/http"
)

// GatewayServerComponent serves the grpc-gateway HTTP API. It composes an
// in-process gRPC server (over bufconn), an in-process client connection, and
// a [pkghttp.ServerComponent] for the external HTTP listener. All three are
// torn down by [GatewayServerComponent.Stop] in dependency order to preserve
// in-flight requests.
//
// The point of using bufconn is to avoid binding a second TCP port purely so
// the gateway can dial the public gRPC listener.
type GatewayServerComponent struct {
	log         *zap.Logger
	bufListener *bufconn.Listener
	grpcServer  *grpc.Server
	clientConn  *grpc.ClientConn
	httpServer  *pkghttp.ServerComponent
}

// NewGatewayServerComponent wires the in-process gRPC server, the bufconn
// dial, and the gateway-fronted HTTP listener.
func NewGatewayServerComponent(opts ...GatewayServerComponentOption) (*GatewayServerComponent, error) {
	options := newGatewayServerComponentOptions(opts...)

	componentLog := options.log.With(zap.String("component", "grpc-gateway-server"))

	bufListener := bufconn.Listen(options.bufconnSize)

	grpcServer := grpc.NewServer(options.serverOptions...)
	for _, register := range options.serviceRegistrations {
		register(grpcServer)
	}

	dialOpts := append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return bufListener.DialContext(ctx)
		}),
	}, options.dialOptions...)

	conn, err := grpc.NewClient("passthrough:///bufnet", dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("dial bufconn: %w", err)
	}

	mux := runtime.NewServeMux(options.muxOptions...)
	for _, register := range options.handlerRegistrations {
		if err := register(context.Background(), mux, conn); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("register gateway handler: %w", err)
		}
	}

	httpOpts := append([]pkghttp.ServerComponentOption{
		pkghttp.WithLog(options.log),
	}, options.httpServerOptions...)
	httpOpts = append(httpOpts, pkghttp.WithHandler(mux))
	httpServer := pkghttp.NewServerComponent(httpOpts...)

	return &GatewayServerComponent{
		log:         componentLog,
		bufListener: bufListener,
		grpcServer:  grpcServer,
		clientConn:  conn,
		httpServer:  httpServer,
	}, nil
}

// Address returns the resolved HTTP listener address. See
// [pkghttp.ServerComponent.Address] for semantics.
func (c *GatewayServerComponent) Address() string {
	return c.httpServer.Address()
}

// AdminAddress returns the resolved admin HTTP listener address.
func (c *GatewayServerComponent) AdminAddress() string {
	return c.httpServer.AdminAddress()
}

// Start launches the in-process gRPC server and the gateway HTTP server. It
// returns when the context is canceled (nil) or when one of the servers fails
// (error). The in-process gRPC server runs on bufconn, so no second TCP port
// is consumed.
func (c *GatewayServerComponent) Start(ctx context.Context) error {
	c.log.Info("starting gateway")

	grpcErrCh := make(chan error, 1)
	go func() {
		err := c.grpcServer.Serve(c.bufListener)
		if errors.Is(err, grpc.ErrServerStopped) {
			err = nil
		}
		grpcErrCh <- err
	}()

	httpErrCh := make(chan error, 1)
	go func() {
		httpErrCh <- c.httpServer.Start(ctx)
	}()

	select {
	case err := <-grpcErrCh:
		if err != nil {
			c.log.Error("in-process gRPC server failed", zap.Error(err))
		}
		return err
	case err := <-httpErrCh:
		if err != nil {
			c.log.Error("gateway HTTP server failed", zap.Error(err))
		}
		return err
	case <-ctx.Done():
		c.log.Info("context done, gateway exiting Start")
		return nil
	}
}

// Stop drains the HTTP server, then the gRPC server, then closes the
// in-process client connection. The bufconn listener is also closed so any
// remaining accept loops return promptly.
func (c *GatewayServerComponent) Stop(ctx context.Context) error {
	c.log.Info("stopping gateway")

	var firstErr error

	if err := c.httpServer.Stop(ctx); err != nil {
		c.log.Error("failed to stop HTTP server", zap.Error(err))
		firstErr = fmt.Errorf("stop http: %w", err)
	}

	grpcStopped := make(chan struct{})
	go func() {
		c.grpcServer.GracefulStop()
		close(grpcStopped)
	}()
	select {
	case <-grpcStopped:
	case <-ctx.Done():
		c.log.Warn("graceful gRPC stop did not finish before context cancel, forcing stop",
			zap.Error(ctx.Err()),
		)
		c.grpcServer.Stop()
		<-grpcStopped
		if firstErr == nil {
			firstErr = fmt.Errorf("stop grpc: %w", ctx.Err())
		}
	}

	if err := c.clientConn.Close(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("close client conn: %w", err)
	}
	if err := c.bufListener.Close(); err != nil && firstErr == nil && !errors.Is(err, net.ErrClosed) {
		firstErr = fmt.Errorf("close bufconn listener: %w", err)
	}

	if firstErr != nil {
		return firstErr
	}
	c.log.Info("gateway stopped")
	return nil
}
