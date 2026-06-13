// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"

	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Pipeline owns the trace, metric, and log providers for one OTLP destination.
type Pipeline struct {
	cfg   Config
	tp    *sdktrace.TracerProvider
	mp    *sdkmetric.MeterProvider
	lp    *sdklog.LoggerProvider
	res   *sdkresource.Resource
	stats *pipelineStats

	metricExp sdkmetric.Exporter
	traceExp  sdktrace.SpanExporter
	logExp    sdklog.Exporter

	// Per-virtual-service trace and log providers, each carrying that service's
	// own service.name resource attribute while sharing this Pipeline's single
	// OTLP exporter. Built lazily and cached by service key so all VUs share one
	// provider (hence one batch processor) per synthetic service instance.
	svcMu      sync.Mutex
	svcTracers map[string]*sdktrace.TracerProvider
	svcLoggers map[string]*sdklog.LoggerProvider

	shutdownOnce sync.Once
	shutdownErr  error
}

type traceBuilder func(context.Context, Config, *tls.Config, *pipelineStats) (sdktrace.SpanExporter, error)
type metricBuilder func(context.Context, Config, *tls.Config, *pipelineStats) (sdkmetric.Exporter, error)
type logBuilder func(context.Context, Config, *tls.Config, *pipelineStats) (sdklog.Exporter, error)

// New builds a Pipeline with one shared resource and OTLP exporters for all signals.
func New(cfg Config) (*Pipeline, error) {
	return newWithExporterBuilders(cfg, buildTraceExporter, buildMetricExporter, buildLogExporter)
}

func newWithExporterBuilders(cfg Config, buildTrace traceBuilder, buildMetric metricBuilder, buildLog logBuilder) (*Pipeline, error) {
	cfg = cfg.fillDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, &PipelineError{Stage: "validate", Inner: err}
	}

	ctx := context.Background()
	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, &PipelineError{Stage: "resource", Inner: err}
	}
	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		return nil, &PipelineError{Stage: "tls", Inner: err}
	}

	stats := &pipelineStats{}
	traceExp, err := buildTrace(ctx, cfg, tlsConfig, stats)
	if err != nil {
		return nil, &PipelineError{Stage: "trace_exporter", Inner: err}
	}
	metricExp, err := buildMetric(ctx, cfg, tlsConfig, stats)
	if err != nil {
		_ = traceExp.Shutdown(context.Background())
		return nil, &PipelineError{Stage: "metric_exporter", Inner: err}
	}
	logExp, err := buildLog(ctx, cfg, tlsConfig, stats)
	if err != nil {
		_ = traceExp.Shutdown(context.Background())
		_ = metricExp.Shutdown(context.Background())
		return nil, &PipelineError{Stage: "log_exporter", Inner: err}
	}

	return newPipelineFromExporters(cfg, res, stats, traceExp, metricExp, logExp), nil
}

// onceShutdownSpanExporter guards Shutdown with sync.Once so the span exporter
// shared by the default and per-service TracerProviders is closed exactly once.
type onceShutdownSpanExporter struct {
	sdktrace.SpanExporter
	once sync.Once
	err  error
}

func (e *onceShutdownSpanExporter) Shutdown(ctx context.Context) error {
	e.once.Do(func() { e.err = e.SpanExporter.Shutdown(ctx) })
	return e.err
}

// onceShutdownLogExporter is the log-exporter counterpart of
// onceShutdownSpanExporter.
type onceShutdownLogExporter struct {
	sdklog.Exporter
	once sync.Once
	err  error
}

func (e *onceShutdownLogExporter) Shutdown(ctx context.Context) error {
	e.once.Do(func() { e.err = e.Exporter.Shutdown(ctx) })
	return e.err
}

func newPipelineFromExporters(cfg Config, res *sdkresource.Resource, stats *pipelineStats, traceExp sdktrace.SpanExporter, metricExp sdkmetric.Exporter, logExp sdklog.Exporter) *Pipeline {
	// Wrap the span and log exporters so the default and per-service providers
	// can each own a batch processor over the SAME exporter without closing it
	// more than once on Shutdown.
	traceExp = &onceShutdownSpanExporter{SpanExporter: traceExp}
	logExp = &onceShutdownLogExporter{Exporter: logExp}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(samplerForConfig(cfg)),
		sdktrace.WithBatcher(traceExp,
			sdktrace.WithMaxQueueSize(cfg.MaxQueueSize),
			sdktrace.WithMaxExportBatchSize(cfg.BatchSize),
			sdktrace.WithBatchTimeout(cfg.BatchTimeout),
		),
	)
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp,
			sdkmetric.WithInterval(cfg.BatchTimeout),
		)),
	)
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp,
			sdklog.WithMaxQueueSize(cfg.MaxQueueSize),
			sdklog.WithExportMaxBatchSize(cfg.BatchSize),
			sdklog.WithExportInterval(cfg.BatchTimeout),
		)),
	)
	return &Pipeline{
		cfg:        cfg,
		tp:         tp,
		mp:         mp,
		lp:         lp,
		res:        res,
		stats:      stats,
		metricExp:  metricExp,
		traceExp:   traceExp,
		logExp:     logExp,
		svcTracers: make(map[string]*sdktrace.TracerProvider),
		svcLoggers: make(map[string]*sdklog.LoggerProvider),
	}
}

func samplerForConfig(cfg Config) sdktrace.Sampler {
	switch cfg.Sampler {
	case "always_off":
		return sdktrace.NeverSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(cfg.SamplerArg)
	default:
		return sdktrace.AlwaysSample()
	}
}

// TracerProvider returns the Pipeline's shared trace provider.
func (p *Pipeline) TracerProvider() trace.TracerProvider {
	return p.tp
}

// MeterProvider returns the Pipeline's shared metric provider.
func (p *Pipeline) MeterProvider() metric.MeterProvider {
	return p.mp
}

// MetricExporter returns the underlying OTLP metric exporter used by this
// Pipeline. Intended for k6output to construct an additional MeterProvider
// with a different Resource (e.g., xk6-otel-gen-runner) while sharing the
// same OTLP connection.
//
// The returned exporter is owned by the Pipeline; callers must NOT call
// Shutdown on it directly. Use Pipeline.Shutdown for unified lifecycle.
func (p *Pipeline) MetricExporter() sdkmetric.Exporter {
	return p.metricExp
}

// LoggerProvider returns the Pipeline's shared log provider.
func (p *Pipeline) LoggerProvider() log.LoggerProvider {
	return p.lp
}

// TracerProviderForService returns a TracerProvider whose resource is res
// (carrying the synthetic service's service.name), sharing this Pipeline's span
// exporter, sampler, and batching config. Providers are cached by key so every
// VU shares one provider — and therefore one batch processor — per service
// instance. This is how each virtual service in the topology shows up in
// Tempo/Grafana under its own service.name instead of OTLPResourceNoServiceName.
func (p *Pipeline) TracerProviderForService(key string, res *sdkresource.Resource) trace.TracerProvider {
	p.svcMu.Lock()
	defer p.svcMu.Unlock()
	if tp, ok := p.svcTracers[key]; ok {
		return tp
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(samplerForConfig(p.cfg)),
		sdktrace.WithBatcher(p.traceExp,
			sdktrace.WithMaxQueueSize(p.cfg.MaxQueueSize),
			sdktrace.WithMaxExportBatchSize(p.cfg.BatchSize),
			sdktrace.WithBatchTimeout(p.cfg.BatchTimeout),
		),
	)
	p.svcTracers[key] = tp
	return tp
}

// LoggerProviderForService returns a LoggerProvider whose resource is res,
// sharing this Pipeline's log exporter and batching config, cached by key.
// See TracerProviderForService.
func (p *Pipeline) LoggerProviderForService(key string, res *sdkresource.Resource) log.LoggerProvider {
	p.svcMu.Lock()
	defer p.svcMu.Unlock()
	if lp, ok := p.svcLoggers[key]; ok {
		return lp
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(p.logExp,
			sdklog.WithMaxQueueSize(p.cfg.MaxQueueSize),
			sdklog.WithExportMaxBatchSize(p.cfg.BatchSize),
			sdklog.WithExportInterval(p.cfg.BatchTimeout),
		)),
	)
	p.svcLoggers[key] = lp
	return lp
}

// svcProviderSnapshot returns the currently cached per-service providers so
// flush/shutdown can act on them without holding svcMu during network I/O.
func (p *Pipeline) svcProviderSnapshot() ([]*sdktrace.TracerProvider, []*sdklog.LoggerProvider) {
	p.svcMu.Lock()
	defer p.svcMu.Unlock()
	tps := make([]*sdktrace.TracerProvider, 0, len(p.svcTracers))
	for _, tp := range p.svcTracers {
		tps = append(tps, tp)
	}
	lps := make([]*sdklog.LoggerProvider, 0, len(p.svcLoggers))
	for _, lp := range p.svcLoggers {
		lps = append(lps, lp)
	}
	return tps, lps
}

// ForceFlush synchronously exports any spans, metrics, and log records still
// queued in the batch processors WITHOUT closing the underlying exporters.
//
// Unlike Shutdown, ForceFlush leaves the Pipeline usable, so it is safe to call
// from a k6 teardown() to guarantee root spans (which End last and therefore
// enter the batch queue last) are delivered before the process exits — even
// when no otel-gen output is configured to trigger Shutdown.
func (p *Pipeline) ForceFlush(ctx context.Context) error {
	tps, lps := p.svcProviderSnapshot()
	errs := []error{
		p.tp.ForceFlush(ctx),
		p.mp.ForceFlush(ctx),
		p.lp.ForceFlush(ctx),
	}
	for _, tp := range tps {
		errs = append(errs, tp.ForceFlush(ctx))
	}
	for _, lp := range lps {
		errs = append(errs, lp.ForceFlush(ctx))
	}
	return errors.Join(errs...)
}

// Shutdown flushes and closes all providers once, returning the first result thereafter.
func (p *Pipeline) Shutdown(ctx context.Context) error {
	p.shutdownOnce.Do(func() {
		tps, lps := p.svcProviderSnapshot()
		errs := []error{
			p.tp.Shutdown(ctx),
			p.mp.Shutdown(ctx),
			p.lp.Shutdown(ctx),
		}
		// Per-service providers share the underlying exporters with the default
		// providers above; shutting a provider down flushes its queue but does
		// not close the shared exporter twice, so this is safe.
		for _, tp := range tps {
			errs = append(errs, tp.Shutdown(ctx))
		}
		for _, lp := range lps {
			errs = append(errs, lp.Shutdown(ctx))
		}
		p.shutdownErr = errors.Join(errs...)
	})
	return p.shutdownErr
}

// Stats returns an atomic snapshot of Pipeline export counters.
func (p *Pipeline) Stats() Stats {
	return p.stats.snapshot()
}
