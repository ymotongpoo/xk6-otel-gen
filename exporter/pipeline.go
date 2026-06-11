// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"context"
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
	tp    *sdktrace.TracerProvider
	mp    *sdkmetric.MeterProvider
	lp    *sdklog.LoggerProvider
	res   *sdkresource.Resource
	stats *pipelineStats

	metricExp sdkmetric.Exporter

	shutdownOnce sync.Once
	shutdownErr  error
}

type traceBuilder func(context.Context, Config, *pipelineStats) (sdktrace.SpanExporter, error)
type metricBuilder func(context.Context, Config, *pipelineStats) (sdkmetric.Exporter, error)
type logBuilder func(context.Context, Config, *pipelineStats) (sdklog.Exporter, error)

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

	stats := &pipelineStats{}
	traceExp, err := buildTrace(ctx, cfg, stats)
	if err != nil {
		return nil, &PipelineError{Stage: "trace_exporter", Inner: err}
	}
	metricExp, err := buildMetric(ctx, cfg, stats)
	if err != nil {
		_ = traceExp.Shutdown(context.Background())
		return nil, &PipelineError{Stage: "metric_exporter", Inner: err}
	}
	logExp, err := buildLog(ctx, cfg, stats)
	if err != nil {
		_ = traceExp.Shutdown(context.Background())
		_ = metricExp.Shutdown(context.Background())
		return nil, &PipelineError{Stage: "log_exporter", Inner: err}
	}

	return newPipelineFromExporters(cfg, res, stats, traceExp, metricExp, logExp), nil
}

func newPipelineFromExporters(cfg Config, res *sdkresource.Resource, stats *pipelineStats, traceExp sdktrace.SpanExporter, metricExp sdkmetric.Exporter, logExp sdklog.Exporter) *Pipeline {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
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
		tp:        tp,
		mp:        mp,
		lp:        lp,
		res:       res,
		stats:     stats,
		metricExp: metricExp,
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

// Shutdown flushes and closes all providers once, returning the first result thereafter.
func (p *Pipeline) Shutdown(ctx context.Context) error {
	p.shutdownOnce.Do(func() {
		p.shutdownErr = errors.Join(
			p.tp.Shutdown(ctx),
			p.mp.Shutdown(ctx),
			p.lp.Shutdown(ctx),
		)
	})
	return p.shutdownErr
}

// Stats returns an atomic snapshot of Pipeline export counters.
func (p *Pipeline) Stats() Stats {
	return p.stats.snapshot()
}
