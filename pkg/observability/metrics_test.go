package pkgobs_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/uniqelus/faas/pkg/observability"
)

func TestMetricsProvider_RegistersRuntimeAndProcessCollectors(t *testing.T) {
	t.Parallel()

	p, err := pkgobs.NewMetricsProvider(
		pkgobs.WithMetricsEnabled(true),
		pkgobs.WithMetricsLog(zaptest.NewLogger(t)),
	)
	require.NoError(t, err)
	require.True(t, p.Enabled())

	families, err := p.Gatherer().Gather()
	require.NoError(t, err)

	names := make(map[string]bool, len(families))
	for _, mf := range families {
		names[mf.GetName()] = true
	}

	assert.True(t,
		names["go_goroutines"] || names["go_gc_duration_seconds"],
		"go runtime collector must be registered (got %v)", names,
	)
	assert.True(t,
		names["process_cpu_seconds_total"] || names["process_resident_memory_bytes"],
		"process collector must be registered (got %v)", names,
	)
}

func TestMetricsProvider_HandlerServesPrometheusExposition(t *testing.T) {
	t.Parallel()

	p, err := pkgobs.NewMetricsProvider(
		pkgobs.WithMetricsEnabled(true),
		pkgobs.WithMetricsLog(zaptest.NewLogger(t)),
	)
	require.NoError(t, err)

	srv := httptest.NewServer(p.Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "go_goroutines")
}

func TestMetricsProvider_ServiceConstLabelsAttachedToCustomCounter(t *testing.T) {
	t.Parallel()

	p, err := pkgobs.NewMetricsProvider(
		pkgobs.WithMetricsEnabled(true),
		pkgobs.WithMetricsService(pkgobs.ServiceConfig{
			Name:        "api-gateway",
			Version:     "v0.1.0",
			Environment: "dev",
		}),
		pkgobs.WithMetricsLog(zaptest.NewLogger(t)),
	)
	require.NoError(t, err)

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "faas_test_counter_total",
		Help: "Test counter",
	})
	require.NoError(t, p.Registry().Register(counter))
	counter.Inc()

	families, err := p.Gatherer().Gather()
	require.NoError(t, err)

	var got *dto.MetricFamily
	for _, mf := range families {
		if mf.GetName() == "faas_test_counter_total" {
			got = mf
			break
		}
	}
	require.NotNil(t, got)
	require.Len(t, got.GetMetric(), 1)

	labels := map[string]string{}
	for _, lp := range got.GetMetric()[0].GetLabel() {
		labels[lp.GetName()] = lp.GetValue()
	}
	assert.Equal(t, "api-gateway", labels[pkgobs.LabelService])
	assert.Equal(t, "v0.1.0", labels[pkgobs.LabelVersion])
	assert.Equal(t, "dev", labels[pkgobs.LabelEnvironment])
}

func TestMetricsProvider_DisabledExposesEmptyButValidHandler(t *testing.T) {
	t.Parallel()

	p, err := pkgobs.NewMetricsProvider(
		pkgobs.WithMetricsEnabled(false),
		pkgobs.WithMetricsLog(zaptest.NewLogger(t)),
	)
	require.NoError(t, err)
	assert.False(t, p.Enabled())

	srv := httptest.NewServer(p.Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, p.Shutdown(context.Background()))
}
