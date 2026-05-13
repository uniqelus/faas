package main

import (
	"context"
	"flag"
	"os/signal"
	"syscall"

	"github.com/uniqelus/faas/internal/api-gateway/app"
	pkgconfig "github.com/uniqelus/faas/pkg/config"
	pkgerrors "github.com/uniqelus/faas/pkg/errors"
	pkglog "github.com/uniqelus/faas/pkg/log"
)

func main() {
	var configPath string

	flag.StringVar(&configPath, "config", "", "path to config file")
	flag.Parse()

	cfg := pkgerrors.Must(pkgconfig.ReadConfig[app.Config](configPath))
	log := pkgerrors.Must(pkglog.NewLogger(cfg.Log.Options()...))

	app := pkgerrors.Must(app.NewApp(cfg, log))

	startCtx, startCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer startCancel()

	if err := app.Start(startCtx); err != nil {
		panic(err)
	}

	<-startCtx.Done()
	startCancel()

	stopCtx, stopCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		panic(err)
	}
}
