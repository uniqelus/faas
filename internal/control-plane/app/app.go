package app

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	pkggrpc "github.com/uniqelus/faas/pkg/grpc"
	pkghttp "github.com/uniqelus/faas/pkg/http"
	pkgobs "github.com/uniqelus/faas/pkg/observability"
)

type App struct {
	log            *zap.Logger
	grpcServer     *pkggrpc.ServerComponent
	gatewayServer  *pkggrpc.GatewayServerComponent
	metricsHandler *pkgobs.MetricsProvider
}

func NewApp(cfg *Config, log *zap.Logger) (*App, error) {
	log = log.With(zap.String("application", "control-plane"))

	metricsProvider, err := pkgobs.NewMetricsProvider(cfg.Observability.MetricsOptions()...)
	if err != nil {
		return nil, fmt.Errorf("init metrics provider: %w", err)
	}

	grpcServer := pkggrpc.NewServerComponent(
		append(cfg.GrpcServer.Options(), pkggrpc.WithLog(log))...,
	)

	gatewayHTTPOptions := append(
		cfg.HTTPServer.Options(),
		pkghttp.WithLog(log),
		pkghttp.WithMetricsHandler(metricsProvider.Handler()),
	)

	gatewayServer, err := pkggrpc.NewGatewayServerComponent(
		pkggrpc.WithGatewayLog(log),
		pkggrpc.WithGatewayHTTPServerOptions(gatewayHTTPOptions...),
	)
	if err != nil {
		return nil, fmt.Errorf("init gateway server: %w", err)
	}

	return &App{
		log:            log,
		grpcServer:     grpcServer,
		gatewayServer:  gatewayServer,
		metricsHandler: metricsProvider,
	}, nil
}

func (a *App) Start(ctx context.Context) error {
	a.log.Info("starting application")

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.grpcServer.Start(gCtx)
	})
	g.Go(func() error {
		return a.gatewayServer.Start(gCtx)
	})

	if err := g.Wait(); err != nil {
		a.log.Error("failed to start application", zap.Error(err))
		return err
	}

	return nil
}

func (a *App) Stop(ctx context.Context) error {
	a.log.Info("stopping application")

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.gatewayServer.Stop(gCtx)
	})
	g.Go(func() error {
		return a.grpcServer.Stop(gCtx)
	})
	g.Go(func() error {
		return a.metricsHandler.Shutdown(gCtx)
	})

	if err := g.Wait(); err != nil {
		a.log.Error("failed to stop application", zap.Error(err))
		return err
	}

	return nil
}
