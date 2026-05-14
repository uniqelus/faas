package pkgobs

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// MetricsProvider owns the Prometheus registry and the common collectors
// (process + Go runtime) shared by every service. The provider exposes a ready
// to use HTTP handler that can be plugged into the pkg/http admin listener via
// [pkghttp.WithMetricsHandler]. Service identity (name/version/environment) is
// reflected as Prometheus const labels so a single Prometheus can host
// multiple FaaS services.
type MetricsProvider struct {
	log        *zap.Logger
	registry   prometheus.Registerer
	gatherer   prometheus.Gatherer
	handler    http.Handler
	constLabel prometheus.Labels
	enabled    bool
}

// NewMetricsProvider builds a metrics provider from the given options. The
// returned provider always exposes a non-nil registry and handler so callers
// can wire it unconditionally; when metrics are disabled the handler responds
// with an empty exposition.
func NewMetricsProvider(opts ...MetricsProviderOption) (*MetricsProvider, error) {
	options := newMetricsProviderOptions(opts...)

	componentLog := options.log.With(zap.String("component", "observability.metrics"))

	if !options.enabled {
		componentLog.Info("metrics disabled, exposing empty registry")
		emptyReg := prometheus.NewRegistry()
		return &MetricsProvider{
			log:      componentLog,
			registry: emptyReg,
			gatherer: emptyReg,
			handler:  promhttp.HandlerFor(emptyReg, promhttp.HandlerOpts{}),
			enabled:  false,
		}, nil
	}

	var (
		registerer = options.registerer
		gatherer   = options.gatherer
	)
	if registerer == nil || gatherer == nil {
		reg := prometheus.NewRegistry()
		registerer = reg
		gatherer = reg
	}

	constLabels := buildConstLabels(options.service)
	if len(constLabels) > 0 {
		registerer = prometheus.WrapRegistererWith(constLabels, registerer)
	}

	if err := registerer.Register(collectors.NewGoCollector()); err != nil {
		return nil, fmt.Errorf("register go collector: %w", err)
	}
	if err := registerer.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		return nil, fmt.Errorf("register process collector: %w", err)
	}

	componentLog.Info("metrics initialized",
		zap.String("service_name", options.service.Name),
		zap.String("service_version", options.service.Version),
		zap.String("service_environment", options.service.Environment),
	)

	return &MetricsProvider{
		log:        componentLog,
		registry:   registerer,
		gatherer:   gatherer,
		handler:    promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}),
		constLabel: constLabels,
		enabled:    true,
	}, nil
}

// Registry returns the registerer that callers should register their own
// collectors against. Wrapping with const labels has already been applied.
func (p *MetricsProvider) Registry() prometheus.Registerer {
	return p.registry
}

// Gatherer returns the raw Prometheus gatherer. Useful for tests that want to
// inspect samples without scraping over HTTP.
func (p *MetricsProvider) Gatherer() prometheus.Gatherer {
	return p.gatherer
}

// Handler returns an [http.Handler] suitable for the /metrics endpoint of the
// admin listener.
func (p *MetricsProvider) Handler() http.Handler {
	return p.handler
}

// Enabled reports whether metric collection is active.
func (p *MetricsProvider) Enabled() bool {
	return p.enabled
}

// Shutdown is provided for symmetry with [TracerProvider.Shutdown]. The
// Prometheus client is pull-based so there are no buffers to flush; the call
// merely emits a log line. Accepts a context so callers can use the same
// shutdown wiring.
func (p *MetricsProvider) Shutdown(_ context.Context) error {
	p.log.Info("metrics provider stopped")
	return nil
}

// Const label keys attached to every metric registered through
// [MetricsProvider.Registry]. Names are prefixed with `service_` to avoid
// colliding with labels emitted by the built-in Prometheus collectors (the Go
// collector, for instance, attaches its own `version` const label to go_info).
const (
	LabelService     = "service_name"
	LabelVersion     = "service_version"
	LabelEnvironment = "service_environment"
)

func buildConstLabels(svc ServiceConfig) prometheus.Labels {
	labels := prometheus.Labels{}
	if svc.Name != "" {
		labels[LabelService] = svc.Name
	}
	if svc.Version != "" {
		labels[LabelVersion] = svc.Version
	}
	if svc.Environment != "" {
		labels[LabelEnvironment] = svc.Environment
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}
