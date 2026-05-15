package steps

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"

	"github.com/uniqelus/faas/tests/integration/harness"
)

func registerHTTPSteps(ctx *godog.ScenarioContext, state *ScenarioState) {
	ctx.When(`^I request admin endpoint "([^"]*)"$`, func(path string) error {
		status, body, err := state.httpClient.Do(context.Background(), http.MethodGet, apiGatewayAdmin+path, nil)
		if err != nil {
			return err
		}
		state.statusCode = status
		state.body = body
		return nil
	})

	ctx.When(`^I request control-plane admin endpoint "([^"]*)"$`, func(path string) error {
		status, body, err := state.httpClient.Do(context.Background(), http.MethodGet, controlPlaneAdmin+path, nil)
		if err != nil {
			return err
		}
		state.statusCode = status
		state.body = body
		return nil
	})

	ctx.Then(`^response status should be (\d+)$`, func(code int) error {
		if state.statusCode != code {
			return fmt.Errorf("expected status %d, got %d", code, state.statusCode)
		}
		return nil
	})

	ctx.Then(`^response contains "([^"]*)"$`, func(substr string) error {
		return harness.AssertContains(state.body, substr)
	})
}
