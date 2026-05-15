package pkgobs

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

type metricsProviderOptions struct {
	enabled    bool
	service    ServiceConfig
	registerer prometheus.Registerer
	gatherer   prometheus.Gatherer
	log        *zap.Logger
}

func defaultMetricsProviderOptions() []MetricsProviderOption {
	return []MetricsProviderOption{
		WithMetricsEnabled(true),
		WithMetricsLog(zap.NewNop()),
	}
}

func newMetricsProviderOptions(opts ...MetricsProviderOption) *metricsProviderOptions {
	options := &metricsProviderOptions{}

	toApply := append(defaultMetricsProviderOptions(), opts...)
	for _, opt := range toApply {
		opt(options)
	}

	return options
}

// MetricsProviderOption configures a [MetricsProvider].
type MetricsProviderOption func(*metricsProviderOptions)

// WithMetricsEnabled toggles metric collection. A disabled provider returns a
// dedicated registry that nobody scrapes, which keeps caller code free of nil
// checks while shedding the cost of registering Go/process collectors.
func WithMetricsEnabled(enabled bool) MetricsProviderOption {
	return func(opts *metricsProviderOptions) {
		opts.enabled = enabled
	}
}

// WithMetricsService overrides the service identity attached as Prometheus
// const labels on every metric.
func WithMetricsService(svc ServiceConfig) MetricsProviderOption {
	return func(opts *metricsProviderOptions) {
		opts.service = svc
	}
}

// WithMetricsRegistry injects an existing Prometheus registry. Tests use it to
// avoid polluting the default registry; production code typically lets the
// provider allocate a private one.
func WithMetricsRegistry(reg *prometheus.Registry) MetricsProviderOption {
	return func(opts *metricsProviderOptions) {
		opts.registerer = reg
		opts.gatherer = reg
	}
}

// WithMetricsLog injects a logger used for lifecycle events.
func WithMetricsLog(log *zap.Logger) MetricsProviderOption {
	return func(opts *metricsProviderOptions) {
		opts.log = log
	}
}
