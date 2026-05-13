package app

import (
	"context"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	pkggrpc "github.com/uniqelus/faas/pkg/grpc"
)

type App struct {
	log        *zap.Logger
	grpcServer *pkggrpc.ServerComponent
}

func NewApp(cfg *Config, log *zap.Logger) (*App, error) {
	log = log.With(zap.String("application", "api-gateway"))
	grpcServer := pkggrpc.NewServerComponent(append(cfg.GrpcServer.Options(), pkggrpc.WithLog(log))...)

	return &App{grpcServer: grpcServer, log: log}, nil
}

func (a *App) Start(ctx context.Context) error {
	a.log.Info("starting application")

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.grpcServer.Start(gCtx)
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
		return a.grpcServer.Stop(gCtx)
	})

	if err := g.Wait(); err != nil {
		a.log.Error("failed to stop application", zap.Error(err))
		return err
	}

	return nil
}
