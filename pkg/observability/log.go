package pkgobs

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Field keys used by the zap integration. They are exported so tests, log
// queries, and Grafana derived-fields configuration can rely on stable names.
const (
	LogFieldTraceID = "trace_id"
	LogFieldSpanID  = "span_id"
)

// WithSpanContext returns a logger that automatically tags every record with
// `trace_id` and `span_id` taken from the span attached to ctx. When the
// context does not carry a valid span context the original logger is returned
// unchanged, so the helper is safe to call from code paths that may execute
// outside of an instrumented request.
//
// Usage at every handler entry point (HTTP/gRPC):
//
//	func (h *Handler) Invoke(ctx context.Context, req *Request) (*Resp, error) {
//	    log := pkgobservability.WithSpanContext(ctx, h.log)
//	    log.Info("invoking function", zap.String("fn", req.Name))
//	    ...
//	}
//
// This is the "zap hook" referenced by ADR 0004: the link between an active
// OpenTelemetry span and every log record produced inside that span.
func WithSpanContext(ctx context.Context, log *zap.Logger) *zap.Logger {
	if log == nil {
		return nil
	}
	fields := spanContextFields(ctx)
	if len(fields) == 0 {
		return log
	}
	return log.With(fields...)
}

// WithSpanContextSugared mirrors [WithSpanContext] for SugaredLogger users.
func WithSpanContextSugared(ctx context.Context, log *zap.SugaredLogger) *zap.SugaredLogger {
	if log == nil {
		return nil
	}
	fields := spanContextFields(ctx)
	if len(fields) == 0 {
		return log
	}
	args := make([]any, 0, len(fields))
	for _, f := range fields {
		args = append(args, f)
	}
	return log.With(args...)
}

// SpanContextCore wraps a [zapcore.Core] so that records produced by the
// returned logger include trace_id/span_id whenever the field carrying the
// active context is supplied. Use it together with [ContextField] when the
// per-request log enrichment via [WithSpanContext] is inconvenient:
//
//	root := zap.New(pkgobservability.SpanContextCore(core))
//	root.Info("invoking", pkgobservability.ContextField(ctx), zap.String("fn", name))
//
// The core leaves regular fields untouched.
func SpanContextCore(inner zapcore.Core) zapcore.Core {
	return &spanContextCore{Core: inner}
}

// ContextField returns a zap field that carries ctx through to a
// [SpanContextCore]. The field renders nothing on cores that do not understand
// it, so it is safe to use with arbitrary zap configurations.
func ContextField(ctx context.Context) zap.Field {
	return zap.Field{
		Key:       contextFieldKey,
		Type:      zapcore.SkipType,
		Interface: contextCarrier{ctx: ctx},
	}
}

const contextFieldKey = "__otel_ctx"

type contextCarrier struct {
	ctx context.Context
}

type spanContextCore struct {
	zapcore.Core
}

func (c *spanContextCore) With(fields []zapcore.Field) zapcore.Core {
	enriched, rest := expandContextFields(fields)
	return &spanContextCore{Core: c.Core.With(append(rest, enriched...))}
}

func (c *spanContextCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *spanContextCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	enriched, rest := expandContextFields(fields)
	return c.Core.Write(ent, append(rest, enriched...))
}

func expandContextFields(fields []zapcore.Field) (extra, rest []zapcore.Field) {
	rest = fields
	for i, f := range fields {
		if f.Key != contextFieldKey {
			continue
		}
		carrier, ok := f.Interface.(contextCarrier)
		if !ok {
			continue
		}
		rest = make([]zapcore.Field, 0, len(fields)-1)
		rest = append(rest, fields[:i]...)
		rest = append(rest, fields[i+1:]...)
		extra = spanContextFields(carrier.ctx)
		return extra, rest
	}
	return nil, rest
}

func spanContextFields(ctx context.Context) []zapcore.Field {
	if ctx == nil {
		return nil
	}
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return nil
	}
	fields := make([]zapcore.Field, 0, 2)
	if sc.HasTraceID() {
		fields = append(fields, zap.String(LogFieldTraceID, sc.TraceID().String()))
	}
	if sc.HasSpanID() {
		fields = append(fields, zap.String(LogFieldSpanID, sc.SpanID().String()))
	}
	return fields
}
