//go:build integration

package integration_test

import (
	"os"
	"testing"

	"github.com/cucumber/godog"

	"github.com/uniqelus/faas/tests/integration/steps"
)

func TestFeatures(t *testing.T) {
	t.Helper()

	tags := os.Getenv("GODOG_TAGS")

	suite := godog.TestSuite{
		Name:                "integration",
		ScenarioInitializer: steps.RegisterScenarioSteps,
		TestSuiteInitializer: func(ctx *godog.TestSuiteContext) {
			ctx.AfterSuite(func() {
				_ = steps.StopSuite()
			})
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			Tags:     tags,
			Strict:   true,
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fail()
	}
}
