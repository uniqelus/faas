package pkgobs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/uniqelus/faas/pkg/observability"
)

// TestWithSpanContext_AddsTraceAndSpanID covers the primary acceptance
// criterion of the issue: when a span is active, every log entry produced
// through the enriched logger carries trace_id and span_id matching the span.
func TestWithSpanContext_AddsTraceAndSpanID(t *testing.T) {
	t.Parallel()

	core, recorded := observer.New(zapcore.DebugLevel)
	log := zap.New(core)

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	enriched := pkgobs.WithSpanContext(ctx, log)
	enriched.Info("hello")

	require.Equal(t, 1, recorded.Len())
	fields := recorded.All()[0].ContextMap()

	traceID, ok := fields[pkgobs.LogFieldTraceID]
	require.True(t, ok, "expected %s field, got %#v", pkgobs.LogFieldTraceID, fields)
	assert.Equal(t, span.SpanContext().TraceID().String(), traceID)

	spanID, ok := fields[pkgobs.LogFieldSpanID]
	require.True(t, ok, "expected %s field, got %#v", pkgobs.LogFieldSpanID, fields)
	assert.Equal(t, span.SpanContext().SpanID().String(), spanID)
}

func TestWithSpanContext_NoSpanIsPassThrough(t *testing.T) {
	t.Parallel()

	core, recorded := observer.New(zapcore.DebugLevel)
	log := zap.New(core)

	enriched := pkgobs.WithSpanContext(context.Background(), log)
	enriched.Info("hello")

	require.Equal(t, 1, recorded.Len())
	fields := recorded.All()[0].ContextMap()
	assert.NotContains(t, fields, pkgobs.LogFieldTraceID)
	assert.NotContains(t, fields, pkgobs.LogFieldSpanID)
}

func TestWithSpanContext_NilLogger(t *testing.T) {
	t.Parallel()

	assert.Nil(t, pkgobs.WithSpanContext(context.Background(), nil))
	assert.Nil(t, pkgobs.WithSpanContextSugared(context.Background(), nil))
}

func TestSpanContextCore_InjectsTraceFieldsFromContextField(t *testing.T) {
	t.Parallel()

	core, recorded := observer.New(zapcore.DebugLevel)
	log := zap.New(pkgobs.SpanContextCore(core))

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	log.Info("hello", pkgobs.ContextField(ctx), zap.String("k", "v"))

	require.Equal(t, 1, recorded.Len())
	fields := recorded.All()[0].ContextMap()

	assert.Contains(t, fields, pkgobs.LogFieldTraceID)
	assert.Contains(t, fields, pkgobs.LogFieldSpanID)
	assert.Equal(t, "v", fields["k"])
	assert.NotContains(t, fields, "__otel_ctx", "internal marker must not leak")
}

func TestSpanContextCore_WithFieldPropagatesAcrossLoggers(t *testing.T) {
	t.Parallel()

	core, recorded := observer.New(zapcore.DebugLevel)
	root := zap.New(pkgobs.SpanContextCore(core))

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	child := root.With(pkgobs.ContextField(ctx))
	child.Info("hello")

	require.Equal(t, 1, recorded.Len())
	fields := recorded.All()[0].ContextMap()
	assert.Contains(t, fields, pkgobs.LogFieldTraceID)
}

func TestWithSpanContextSugared_AddsTraceFields(t *testing.T) {
	t.Parallel()

	core, recorded := observer.New(zapcore.DebugLevel)
	log := zap.New(core).Sugar()

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	enriched := pkgobs.WithSpanContextSugared(ctx, log)
	enriched.Infow("hello")

	require.Equal(t, 1, recorded.Len())
	fields := recorded.All()[0].ContextMap()
	assert.Contains(t, fields, pkgobs.LogFieldTraceID)
	assert.Contains(t, fields, pkgobs.LogFieldSpanID)
}
