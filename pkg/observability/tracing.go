package pkgobs

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

// TracerProvider owns the SDK tracer provider, the OTLP exporter, and the
// lifecycle for both. When tracing is disabled, the provider remains usable
// but produces a noop tracer so that downstream code can call
// [TracerProvider.Tracer] without nil checks.
type TracerProvider struct {
	log             *zap.Logger
	provider        trace.TracerProvider
	sdkProvider     *sdktrace.TracerProvider
	exporter        sdktrace.SpanExporter
	propagator      propagation.TextMapPropagator
	shutdownTimeout time.Duration
	enabled         bool
}

// NewTracerProvider builds a tracer provider from the given options. When
// tracing is enabled it dials the OTLP/gRPC collector, registers the SDK
// provider as the OpenTelemetry global, and installs a W3C TraceContext +
// Baggage propagator. When tracing is disabled it returns a wrapper around the
// OpenTelemetry noop provider; [TracerProvider.Shutdown] is then a no-op.
//
// The supplied context bounds the exporter handshake; on success the
// provider's own internal context handles span batching.
func NewTracerProvider(ctx context.Context, opts ...TracerProviderOption) (*TracerProvider, error) {
	options := newTracerProviderOptions(opts...)

	componentLog := options.log.With(zap.String("component", "observability.tracing"))
	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	if !options.enabled {
		componentLog.Info("tracing disabled, installing noop provider")
		noopProvider := noop.NewTracerProvider()
		otel.SetTracerProvider(noopProvider)
		otel.SetTextMapPropagator(propagator)
		return &TracerProvider{
			log:        componentLog,
			provider:   noopProvider,
			propagator: propagator,
			enabled:    false,
		}, nil
	}

	exporter, err := newOTLPExporter(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := newResource(ctx, options.service)
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, fmt.Errorf("build resource: %w", err)
	}

	sampler, err := buildSampler(options.sampler, options.samplerRatio)
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, err
	}

	sdkProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(sdkProvider)
	otel.SetTextMapPropagator(propagator)

	componentLog.Info("tracing initialized",
		zap.String("endpoint", options.endpoint),
		zap.String("sampler", options.sampler),
		zap.Float64("sampler_ratio", options.samplerRatio),
	)

	return &TracerProvider{
		log:             componentLog,
		provider:        sdkProvider,
		sdkProvider:     sdkProvider,
		exporter:        exporter,
		propagator:      propagator,
		shutdownTimeout: options.shutdownTimeout,
		enabled:         true,
	}, nil
}

// Tracer returns a named tracer that respects the configured sampler.
func (p *TracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return p.provider.Tracer(name, opts...)
}

// Provider returns the underlying [trace.TracerProvider] for callers that need
// to pass it to OpenTelemetry-aware libraries (otelgrpc, otelhttp, etc.).
func (p *TracerProvider) Provider() trace.TracerProvider {
	return p.provider
}

// Propagator returns the configured W3C TraceContext + Baggage propagator.
func (p *TracerProvider) Propagator() propagation.TextMapPropagator {
	return p.propagator
}

// Enabled reports whether the provider is exporting spans.
func (p *TracerProvider) Enabled() bool {
	return p.enabled
}

// Shutdown flushes any pending spans and tears down the exporter. It is safe
// to call multiple times and on a disabled provider.
func (p *TracerProvider) Shutdown(ctx context.Context) error {
	if !p.enabled || p.sdkProvider == nil {
		return nil
	}

	if p.shutdownTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.shutdownTimeout)
		defer cancel()
	}

	p.log.Info("flushing tracer provider")
	if err := p.sdkProvider.ForceFlush(ctx); err != nil {
		p.log.Warn("force flush failed", zap.Error(err))
	}
	if err := p.sdkProvider.Shutdown(ctx); err != nil {
		p.log.Error("tracer provider shutdown failed", zap.Error(err))
		return fmt.Errorf("shutdown tracer provider: %w", err)
	}
	p.log.Info("tracer provider stopped")
	return nil
}

func newOTLPExporter(ctx context.Context, options *tracerProviderOptions) (sdktrace.SpanExporter, error) {
	clientOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(options.endpoint),
		otlptracegrpc.WithTimeout(options.timeout),
	}
	if options.insecure {
		clientOpts = append(clientOpts, otlptracegrpc.WithInsecure())
	}
	client := otlptracegrpc.NewClient(clientOpts...)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, err
	}
	return exporter, nil
}

func newResource(ctx context.Context, svc ServiceConfig) (*resource.Resource, error) {
	base, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, err
	}

	kvs := make([]attribute.KeyValue, 0, 3)
	if svc.Name != "" {
		kvs = append(kvs, semconv.ServiceName(svc.Name))
	}
	if svc.Version != "" {
		kvs = append(kvs, semconv.ServiceVersion(svc.Version))
	}
	if svc.Environment != "" {
		kvs = append(kvs, semconv.DeploymentEnvironment(svc.Environment))
	}
	if len(kvs) == 0 {
		return base, nil
	}

	merged, err := resource.Merge(base, resource.NewSchemaless(kvs...))
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func buildSampler(name string, ratio float64) (sdktrace.Sampler, error) {
	switch name {
	case SamplerAlwaysOn, "":
		return sdktrace.AlwaysSample(), nil
	case SamplerAlwaysOff:
		return sdktrace.NeverSample(), nil
	case SamplerTraceIDRatio:
		return sdktrace.TraceIDRatioBased(ratio), nil
	case SamplerParentBasedAlwaysOn:
		return sdktrace.ParentBased(sdktrace.AlwaysSample()), nil
	case SamplerParentBasedAlwaysOff:
		return sdktrace.ParentBased(sdktrace.NeverSample()), nil
	case SamplerParentBasedTraceIDRate:
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio)), nil
	default:
		return nil, fmt.Errorf("unknown sampler %q", name)
	}
}
