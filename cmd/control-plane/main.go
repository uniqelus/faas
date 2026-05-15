package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/uniqelus/faas/internal/control-plane/app"
	pkgconfig "github.com/uniqelus/faas/pkg/config"
	pkglog "github.com/uniqelus/faas/pkg/log"
)

const defaultShutdownTimeout = 30 * time.Second

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "control-plane failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var configPath string

	flag.StringVar(&configPath, "config", "", "path to config file")
	flag.Parse()

	cfg, err := pkgconfig.ReadConfig[app.Config](configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	log, err := pkglog.NewLogger(cfg.Log.Options()...)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() {
		_ = log.Sync()
	}()

	application, err := app.NewApp(cfg, log)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}

	startCtx, startCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer startCancel()

	if err := application.Start(startCtx); err != nil {
		return fmt.Errorf("start app: %w", err)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
	defer stopCancel()

	if err := application.Stop(stopCtx); err != nil {
		return fmt.Errorf("stop app: %w", err)
	}

	return nil
}
