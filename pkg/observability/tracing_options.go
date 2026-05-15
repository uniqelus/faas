package pkgobs

import (
	"time"

	"go.uber.org/zap"
)

type tracerProviderOptions struct {
	enabled         bool
	endpoint        string
	insecure        bool
	timeout         time.Duration
	sampler         string
	samplerRatio    float64
	shutdownTimeout time.Duration
	service         ServiceConfig
	log             *zap.Logger
}

func defaultTracerProviderOptions() []TracerProviderOption {
	return []TracerProviderOption{
		WithTracingEnabled(true),
		WithTracingEndpoint(DefaultTracingEndpoint),
		WithTracingInsecure(DefaultTracingInsecure),
		WithTracingTimeout(DefaultTracingTimeout),
		WithTracingSampler(DefaultTracingSampler),
		WithTracingSamplerRatio(DefaultTracingSamplerRatio),
		WithTracingShutdownTimeout(DefaultTracingShutdownTimeout),
		WithTracingLog(zap.NewNop()),
	}
}

func newTracerProviderOptions(opts ...TracerProviderOption) *tracerProviderOptions {
	options := &tracerProviderOptions{}

	toApply := append(defaultTracerProviderOptions(), opts...)
	for _, opt := range toApply {
		opt(options)
	}

	return options
}

// TracerProviderOption configures a [TracerProvider].
type TracerProviderOption func(*tracerProviderOptions)

// WithTracingEnabled toggles span export. When disabled, [NewTracerProvider]
// returns a provider whose tracers are usable but produce no exported spans,
// which keeps caller code free of nil checks.
func WithTracingEnabled(enabled bool) TracerProviderOption {
	return func(opts *tracerProviderOptions) {
		opts.enabled = enabled
	}
}

// WithTracingEndpoint sets the OTLP/gRPC collector endpoint, e.g. "otel-collector:4317".
func WithTracingEndpoint(endpoint string) TracerProviderOption {
	return func(opts *tracerProviderOptions) {
		opts.endpoint = endpoint
	}
}

// WithTracingInsecure disables TLS on the OTLP gRPC connection. The default is
// true because the MVP runs the collector inside the same Kubernetes cluster.
func WithTracingInsecure(insecure bool) TracerProviderOption {
	return func(opts *tracerProviderOptions) {
		opts.insecure = insecure
	}
}

// WithTracingTimeout bounds the OTLP exporter dial and export RPC durations.
func WithTracingTimeout(d time.Duration) TracerProviderOption {
	return func(opts *tracerProviderOptions) {
		opts.timeout = d
	}
}

// WithTracingSampler selects the SDK sampler. Accepted values are the
// SamplerXxx constants exported by this package.
func WithTracingSampler(name string) TracerProviderOption {
	return func(opts *tracerProviderOptions) {
		opts.sampler = name
	}
}

// WithTracingSamplerRatio sets the ratio used by TraceIDRatioBased samplers.
// Values outside [0, 1] are clamped by the SDK.
func WithTracingSamplerRatio(ratio float64) TracerProviderOption {
	return func(opts *tracerProviderOptions) {
		opts.samplerRatio = ratio
	}
}

// WithTracingShutdownTimeout caps how long [TracerProvider.Shutdown] waits
// when flushing pending spans on application exit.
func WithTracingShutdownTimeout(d time.Duration) TracerProviderOption {
	return func(opts *tracerProviderOptions) {
		opts.shutdownTimeout = d
	}
}

// WithTracingService overrides the resource attributes attached to every span.
func WithTracingService(svc ServiceConfig) TracerProviderOption {
	return func(opts *tracerProviderOptions) {
		opts.service = svc
	}
}

// WithTracingLog injects a logger that the provider uses for lifecycle events.
func WithTracingLog(log *zap.Logger) TracerProviderOption {
	return func(opts *tracerProviderOptions) {
		opts.log = log
	}
}
