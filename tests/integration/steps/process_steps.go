package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"

	"github.com/uniqelus/faas/tests/integration/harness"
)

func registerProcessSteps(ctx *godog.ScenarioContext, state *ScenarioState) {
	ctx.Given(`^api gateway is running$`, func() error {
		return state.runtime.StartService(context.Background(), harness.ServiceDefinition{
			Name:      "api-gateway",
			ConfigRel: "tests/integration/features/api-gateway/testdata/api-gateway.test.yaml",
			AdminBase: apiGatewayAdmin,
			GRPCAddr:  "127.0.0.1:20000",
		})
	})

	ctx.Given(`^control-plane is running$`, func() error {
		return state.runtime.StartService(context.Background(), harness.ServiceDefinition{
			Name:      "control-plane",
			ConfigRel: "tests/integration/features/control-plane/testdata/control-plane.test.yaml",
			AdminBase: controlPlaneAdmin,
			GRPCAddr:  controlPlaneGRPC,
		})
	})

	ctx.Then(`^control-plane gRPC port is reachable$`, func() error {
		dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		conn, err := harness.DialGRPC(dialCtx, controlPlaneGRPC)
		if err != nil {
			return err
		}
		return conn.Close()
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if err == nil {
			return ctx, nil
		}
		return ctx, fmt.Errorf("scenario failed: %w", err)
	})
}
