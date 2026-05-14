// Package pkgobs provides a single initialization point for the
// observability stack: an OpenTelemetry tracer provider with an OTLP gRPC
// exporter, a Prometheus metrics registry, and a helper that enriches zap
// loggers with trace_id/span_id taken from the active span.
//
// The package is intentionally low-level: it constructs and returns the
// providers and lets the caller wire them into HTTP/gRPC servers and
// application configs. See ADR 0004 for the rationale.
package pkgobs

import "time"

// Sampler names accepted by [TracingConfig.Sampler]. They map onto the
// OpenTelemetry samplers documented in
// https://opentelemetry.io/docs/concepts/sampling/.
const (
	SamplerAlwaysOn               = "always_on"
	SamplerAlwaysOff              = "always_off"
	SamplerTraceIDRatio           = "traceidratio"
	SamplerParentBasedAlwaysOn    = "parentbased_always_on"
	SamplerParentBasedAlwaysOff   = "parentbased_always_off"
	SamplerParentBasedTraceIDRate = "parentbased_traceidratio"
)

// Default values for the observability YAML block. Exported so other packages
// can reuse them when constructing test configs.
const (
	DefaultTracingEndpoint        = "localhost:4317"
	DefaultTracingInsecure        = true
	DefaultTracingTimeout         = 10 * time.Second
	DefaultTracingSampler         = SamplerParentBasedAlwaysOn
	DefaultTracingSamplerRatio    = 1.0
	DefaultTracingShutdownTimeout = 5 * time.Second
)

// Config is the shared `observability:` block embedded into every service
// config. Each sub-block is consumed by a dedicated provider constructor.
type Config struct {
	Service ServiceConfig `yaml:"service"`
	Tracing TracingConfig `yaml:"tracing"`
	Metrics MetricsConfig `yaml:"metrics"`
}

// ServiceConfig identifies the running process. The values are attached as
// OpenTelemetry resource attributes and as Prometheus const labels so a single
// scrape/trace backend can host multiple services.
type ServiceConfig struct {
	Name        string `yaml:"name" env:"OBSERVABILITY_SERVICE_NAME"`
	Version     string `yaml:"version" env:"OBSERVABILITY_SERVICE_VERSION"`
	Environment string `yaml:"environment" env:"OBSERVABILITY_SERVICE_ENVIRONMENT"`
}

// TracingConfig configures the OTLP/gRPC trace exporter and the sampler.
type TracingConfig struct {
	Enabled         bool          `yaml:"enabled" env:"OBSERVABILITY_TRACING_ENABLED" env-default:"true"`
	Endpoint        string        `yaml:"endpoint" env:"OBSERVABILITY_TRACING_ENDPOINT" env-default:"localhost:4317"`
	Insecure        bool          `yaml:"insecure" env:"OBSERVABILITY_TRACING_INSECURE" env-default:"true"`
	Timeout         time.Duration `yaml:"timeout" env:"OBSERVABILITY_TRACING_TIMEOUT" env-default:"10s"`
	Sampler         string        `yaml:"sampler" env:"OBSERVABILITY_TRACING_SAMPLER" env-default:"parentbased_always_on"`
	SamplerRatio    float64       `yaml:"sampler_ratio" env:"OBSERVABILITY_TRACING_SAMPLER_RATIO" env-default:"1.0"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"OBSERVABILITY_TRACING_SHUTDOWN_TIMEOUT" env-default:"5s"`
}

// MetricsConfig configures the Prometheus registry. Scrape endpoint setup
// lives in the consuming service (typically pkg/http admin listener).
type MetricsConfig struct {
	Enabled bool `yaml:"enabled" env:"OBSERVABILITY_METRICS_ENABLED" env-default:"true"`
}

// TracingOptions converts the YAML block into a slice of [TracerProvider]
// options. The service block is replayed so resource attributes stay in sync.
func (c Config) TracingOptions() []TracerProviderOption {
	return []TracerProviderOption{
		WithTracingEnabled(c.Tracing.Enabled),
		WithTracingEndpoint(c.Tracing.Endpoint),
		WithTracingInsecure(c.Tracing.Insecure),
		WithTracingTimeout(c.Tracing.Timeout),
		WithTracingSampler(c.Tracing.Sampler),
		WithTracingSamplerRatio(c.Tracing.SamplerRatio),
		WithTracingShutdownTimeout(c.Tracing.ShutdownTimeout),
		WithTracingService(c.Service),
	}
}

// MetricsOptions converts the YAML block into a slice of [MetricsProvider]
// options. The service block is replayed so const labels stay in sync.
func (c Config) MetricsOptions() []MetricsProviderOption {
	return []MetricsProviderOption{
		WithMetricsEnabled(c.Metrics.Enabled),
		WithMetricsService(c.Service),
	}
}
