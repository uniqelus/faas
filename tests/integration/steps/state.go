package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/uniqelus/faas/tests/integration/harness"
)

const (
	apiGatewayAdmin = "http://127.0.0.1:18081"
	apiGatewayHTTP  = "http://127.0.0.1:18080"

	controlPlaneAdmin = "http://127.0.0.1:19081"
	controlPlaneGRPC  = "127.0.0.1:19000"
)

type ScenarioState struct {
	projectRoot string

	runtime    *harness.Runtime
	httpClient *harness.HTTPClient
	sqlClient  *harness.SQLClient

	statusCode int
	body       []byte

	functionName string
}

var (
	suiteState *ScenarioState
	suiteOnce  sync.Once
)

func SuiteState(projectRoot string) *ScenarioState {
	suiteOnce.Do(func() {
		suiteState = NewScenarioState(projectRoot)
	})
	return suiteState
}

func NewScenarioState(projectRoot string) *ScenarioState {
	logsDir := filepath.Join(projectRoot, "tests", "integration", "logs")
	return &ScenarioState{
		projectRoot: projectRoot,
		runtime:     harness.NewRuntime(projectRoot, logsDir),
		httpClient:  harness.NewHTTPClient(),
		sqlClient:   harness.NewSQLClient(),
	}
}

func (s *ScenarioState) Reset() {
	s.statusCode = 0
	s.body = nil
	s.functionName = fmt.Sprintf("fn-%d", time.Now().UnixNano())
}

func (s *ScenarioState) LastJSON() (map[string]any, error) {
	if len(s.body) == 0 {
		return nil, fmt.Errorf("last response body is empty")
	}
	var out map[string]any
	if err := json.Unmarshal(s.body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal last response: %w", err)
	}
	return out, nil
}

func (s *ScenarioState) shutdown(ctx context.Context) error {
	return s.runtime.StopAll(ctx, 5*time.Second)
}
