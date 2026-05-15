package pkgobs_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc"

	"github.com/uniqelus/faas/pkg/observability"
)

// fakeOTLPServer is a tiny in-process OTLP gRPC trace receiver. It records the
// resource spans pushed by the exporter so tests can assert that
// [pkgobs.TracerProvider.Shutdown] flushes pending spans.
type fakeOTLPServer struct {
	coltracepb.UnimplementedTraceServiceServer

	mu    sync.Mutex
	spans []*tracepb.Span

	srv  *grpc.Server
	addr string
}

func (f *fakeOTLPServer) Export(_ context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rs := range req.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			f.spans = append(f.spans, ss.GetSpans()...)
		}
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

func (f *fakeOTLPServer) Spans() []*tracepb.Span {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*tracepb.Span, len(f.spans))
	copy(out, f.spans)
	return out
}

func startFakeOTLPServer(t *testing.T) *fakeOTLPServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	f := &fakeOTLPServer{srv: srv, addr: listener.Addr().String()}
	coltracepb.RegisterTraceServiceServer(srv, f)

	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(srv.GracefulStop)
	return f
}

func TestTracerProvider_DisabledIsNoop(t *testing.T) {
	t.Parallel()

	tp, err := pkgobs.NewTracerProvider(context.Background(),
		pkgobs.WithTracingEnabled(false),
		pkgobs.WithTracingLog(zaptest.NewLogger(t)),
	)
	require.NoError(t, err)
	assert.False(t, tp.Enabled())
	assert.NotNil(t, tp.Tracer("test"))
	assert.NotNil(t, tp.Propagator())
	require.NoError(t, tp.Shutdown(context.Background()))
}

// TestTracerProvider_ShutdownFlushesPendingSpans covers two acceptance
// criteria at once: (1) OTLP endpoint is taken from config, (2) all providers
// graceful-shutdown-flush pending spans. The fake collector observes the
// exported batch only because Shutdown drains the BatchSpanProcessor.
func TestTracerProvider_ShutdownFlushesPendingSpans(t *testing.T) {
	t.Parallel()

	fake := startFakeOTLPServer(t)
	ctx := context.Background()

	tp, err := pkgobs.NewTracerProvider(ctx,
		pkgobs.WithTracingEnabled(true),
		pkgobs.WithTracingEndpoint(fake.addr),
		pkgobs.WithTracingInsecure(true),
		pkgobs.WithTracingTimeout(2*time.Second),
		pkgobs.WithTracingSampler(pkgobs.SamplerAlwaysOn),
		pkgobs.WithTracingShutdownTimeout(3*time.Second),
		pkgobs.WithTracingService(pkgobs.ServiceConfig{Name: "api-gateway"}),
		pkgobs.WithTracingLog(zaptest.NewLogger(t)),
	)
	require.NoError(t, err)
	require.True(t, tp.Enabled())

	tracer := tp.Tracer("test")
	_, span := tracer.Start(ctx, "op")
	traceID := span.SpanContext().TraceID().String()
	span.End()

	require.NoError(t, tp.Shutdown(ctx))

	require.Eventually(t, func() bool {
		return len(fake.Spans()) > 0
	}, 3*time.Second, 25*time.Millisecond, "collector never received spans")

	spans := fake.Spans()
	require.NotEmpty(t, spans)
	gotTraceID := traceIDHex(spans[0].GetTraceId())
	assert.Equal(t, traceID, gotTraceID)
}

func TestTracerProvider_ShutdownIsIdempotent(t *testing.T) {
	t.Parallel()

	fake := startFakeOTLPServer(t)
	tp, err := pkgobs.NewTracerProvider(context.Background(),
		pkgobs.WithTracingEnabled(true),
		pkgobs.WithTracingEndpoint(fake.addr),
		pkgobs.WithTracingInsecure(true),
		pkgobs.WithTracingLog(zaptest.NewLogger(t)),
	)
	require.NoError(t, err)

	require.NoError(t, tp.Shutdown(context.Background()))
	// A second Shutdown on a stopped provider must not panic. The SDK reports
	// an error for double Shutdown, which we tolerate as long as the call
	// returns instead of blocking.
	_ = tp.Shutdown(context.Background())
}

// TestTracerProvider_SamplerFromConfig exercises every sampler name accepted
// by Config.Tracing.Sampler and the rejection of unknown values. It satisfies
// the "Default sampler настраивается из конфига (AlwaysOn / ParentBased)"
// acceptance criterion.
func TestTracerProvider_SamplerFromConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		sampler string
		ratio   float64
		wantErr bool
	}{
		{"always_on", pkgobs.SamplerAlwaysOn, 0, false},
		{"always_off", pkgobs.SamplerAlwaysOff, 0, false},
		{"traceidratio", pkgobs.SamplerTraceIDRatio, 0.1, false},
		{"parentbased_always_on", pkgobs.SamplerParentBasedAlwaysOn, 0, false},
		{"parentbased_always_off", pkgobs.SamplerParentBasedAlwaysOff, 0, false},
		{"parentbased_traceidratio", pkgobs.SamplerParentBasedTraceIDRate, 0.25, false},
		{"empty defaults to always_on", "", 0, false},
		{"unknown", "broken", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := startFakeOTLPServer(t)
			tp, err := pkgobs.NewTracerProvider(context.Background(),
				pkgobs.WithTracingEnabled(true),
				pkgobs.WithTracingEndpoint(fake.addr),
				pkgobs.WithTracingInsecure(true),
				pkgobs.WithTracingSampler(tc.sampler),
				pkgobs.WithTracingSamplerRatio(tc.ratio),
				pkgobs.WithTracingLog(zaptest.NewLogger(t)),
			)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NoError(t, tp.Shutdown(context.Background()))
		})
	}
}

func TestTracerProvider_AlwaysOffDropsSpans(t *testing.T) {
	t.Parallel()

	fake := startFakeOTLPServer(t)
	ctx := context.Background()

	tp, err := pkgobs.NewTracerProvider(ctx,
		pkgobs.WithTracingEnabled(true),
		pkgobs.WithTracingEndpoint(fake.addr),
		pkgobs.WithTracingInsecure(true),
		pkgobs.WithTracingSampler(pkgobs.SamplerAlwaysOff),
		pkgobs.WithTracingLog(zaptest.NewLogger(t)),
	)
	require.NoError(t, err)

	_, span := tp.Tracer("test").Start(ctx, "op")
	span.End()

	require.NoError(t, tp.Shutdown(ctx))
	assert.Empty(t, fake.Spans(), "always_off sampler must not export spans")
}

func TestConfig_TracingAndMetricsOptions(t *testing.T) {
	t.Parallel()

	cfg := pkgobs.Config{
		Service: pkgobs.ServiceConfig{
			Name:        "api-gateway",
			Version:     "v0.1.0",
			Environment: "dev",
		},
		Tracing: pkgobs.TracingConfig{
			Enabled:         true,
			Endpoint:        "collector:4317",
			Insecure:        true,
			Timeout:         5 * time.Second,
			Sampler:         pkgobs.SamplerParentBasedTraceIDRate,
			SamplerRatio:    0.1,
			ShutdownTimeout: 2 * time.Second,
		},
		Metrics: pkgobs.MetricsConfig{Enabled: true},
	}

	assert.NotEmpty(t, cfg.TracingOptions())
	assert.NotEmpty(t, cfg.MetricsOptions())

	mp, err := pkgobs.NewMetricsProvider(cfg.MetricsOptions()...)
	require.NoError(t, err)
	assert.True(t, mp.Enabled())
}

// traceIDHex renders a raw 16-byte trace ID as the 32-character lowercase hex
// string used by W3C TraceContext. The OTLP proto payload uses raw bytes,
// while OTel SDK consumers see hex-encoded strings.
func traceIDHex(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, hex[c>>4], hex[c&0xf])
	}
	return string(out)
}
