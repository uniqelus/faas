package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"
)

func registerLifecycleSteps(ctx *godog.ScenarioContext, state *ScenarioState) {
	createPayload := func(name string) []byte {
		return []byte(fmt.Sprintf(`{"function":{"name":"%s","image":"busybox:latest","replicas":1}}`, name))
	}

	doRequest := func(method, path string, body []byte) error {
		status, respBody, err := state.httpClient.Do(context.Background(), method, apiGatewayHTTP+path, body)
		if err != nil {
			return err
		}
		state.statusCode = status
		state.body = respBody
		return nil
	}

	ctx.When(`^I create a function via gateway API$`, func() error {
		return doRequest(http.MethodPost, "/v1/functions", createPayload(state.functionName))
	})

	ctx.When(`^I request the function via gateway API$`, func() error {
		return doRequest(http.MethodGet, "/v1/functions/"+state.functionName, nil)
	})

	ctx.When(`^I request the function list via gateway API$`, func() error {
		return doRequest(http.MethodGet, "/v1/functions", nil)
	})

	ctx.When(`^I delete the function via gateway API$`, func() error {
		return doRequest(http.MethodDelete, "/v1/functions/"+state.functionName, nil)
	})

	ctx.Then(`^response contains the created function$`, func() error {
		return assertFunctionNameInBody(state.body, state.functionName)
	})

	ctx.Then(`^lifecycle action "([^"]*)" returns status (\d+)$`, func(action string, status int) error {
		switch action {
		case "create":
			if err := doRequest(http.MethodPost, "/v1/functions", createPayload(state.functionName)); err != nil {
				return err
			}
		case "get":
			if err := doRequest(http.MethodGet, "/v1/functions/"+state.functionName, nil); err != nil {
				return err
			}
		case "list":
			if err := doRequest(http.MethodGet, "/v1/functions", nil); err != nil {
				return err
			}
		case "delete":
			if err := doRequest(http.MethodDelete, "/v1/functions/"+state.functionName, nil); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown action %q", action)
		}

		if state.statusCode != status {
			return fmt.Errorf("expected status %d, got %d", status, state.statusCode)
		}
		return nil
	})
}

func assertFunctionNameInBody(body []byte, functionName string) error {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	raw, ok := doc["function"]
	if !ok {
		return fmt.Errorf("response does not contain function")
	}
	fnObj, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("function field is not an object")
	}
	name, _ := fnObj["name"].(string)
	if name != functionName {
		return fmt.Errorf("expected function name %q, got %q", functionName, name)
	}
	return nil
}
