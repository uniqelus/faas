package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cucumber/godog"
)

func RegisterScenarioSteps(ctx *godog.ScenarioContext) {
	projectRoot, err := resolveProjectRoot()
	if err != nil {
		panic(err)
	}

	state := SuiteState(projectRoot)

	registerProcessSteps(ctx, state)
	registerHTTPSteps(ctx, state)
	registerLifecycleSteps(ctx, state)
	registerGRPCSteps(ctx, state)
	registerSQLSteps(ctx, state)

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		state.Reset()
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		return ctx, nil
	})
}

func StopSuite() error {
	projectRoot, err := resolveProjectRoot()
	if err != nil {
		return err
	}
	state := SuiteState(projectRoot)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return state.shutdown(shutdownCtx)
}

func resolveProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	current := filepath.Clean(wd)
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("unable to locate project root from %s", wd)
		}
		current = parent
	}
}
