package exporter

import (
	"context"
	"sync/atomic"

	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Stats is a point-in-time snapshot of Pipeline export counters.
type Stats struct {
	TracesExported  int64
	TracesFailed    int64
	MetricsExported int64
	MetricsFailed   int64
	LogsExported    int64
	LogsFailed      int64
}

// pipelineStats stores per-signal counters updated by exporter wrappers.
type pipelineStats struct {
	tracesExported  atomic.Int64
	tracesFailed    atomic.Int64
	metricsExported atomic.Int64
	metricsFailed   atomic.Int64
	logsExported    atomic.Int64
	logsFailed      atomic.Int64
}

func (s *pipelineStats) snapshot() Stats {
	return Stats{
		TracesExported:  s.tracesExported.Load(),
		TracesFailed:    s.tracesFailed.Load(),
		MetricsExported: s.metricsExported.Load(),
		MetricsFailed:   s.metricsFailed.Load(),
		LogsExported:    s.logsExported.Load(),
		LogsFailed:      s.logsFailed.Load(),
	}
}

// tracingExporter wraps a SpanExporter and records trace export counters.
type tracingExporter struct {
	inner sdktrace.SpanExporter
	stats *pipelineStats
}

func (e *tracingExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if err := e.inner.ExportSpans(ctx, spans); err != nil {
		e.stats.tracesFailed.Add(1)
		return err
	}
	e.stats.tracesExported.Add(int64(len(spans)))
	return nil
}

func (e *tracingExporter) Shutdown(ctx context.Context) error {
	return e.inner.Shutdown(ctx)
}

// metricExporter wraps a metric Exporter and records metric export counters.
type metricExporter struct {
	inner sdkmetric.Exporter
	stats *pipelineStats
}

func (e *metricExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return e.inner.Temporality(kind)
}

func (e *metricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return e.inner.Aggregation(kind)
}

func (e *metricExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	count := countMetricDataPoints(rm)
	if err := e.inner.Export(ctx, rm); err != nil {
		e.stats.metricsFailed.Add(1)
		return err
	}
	e.stats.metricsExported.Add(int64(count))
	return nil
}

func (e *metricExporter) ForceFlush(ctx context.Context) error {
	return e.inner.ForceFlush(ctx)
}

func (e *metricExporter) Shutdown(ctx context.Context) error {
	return e.inner.Shutdown(ctx)
}

// loggingExporter wraps a log Exporter and records log export counters.
type loggingExporter struct {
	inner sdklog.Exporter
	stats *pipelineStats
}

func (e *loggingExporter) Export(ctx context.Context, records []sdklog.Record) error {
	if err := e.inner.Export(ctx, records); err != nil {
		e.stats.logsFailed.Add(1)
		return err
	}
	e.stats.logsExported.Add(int64(len(records)))
	return nil
}

func (e *loggingExporter) ForceFlush(ctx context.Context) error {
	return e.inner.ForceFlush(ctx)
}

func (e *loggingExporter) Shutdown(ctx context.Context) error {
	return e.inner.Shutdown(ctx)
}

func countMetricDataPoints(rm *metricdata.ResourceMetrics) int {
	if rm == nil {
		return 0
	}
	var count int
	for _, scopeMetrics := range rm.ScopeMetrics {
		for _, metric := range scopeMetrics.Metrics {
			switch data := metric.Data.(type) {
			case metricdata.Gauge[int64]:
				count += len(data.DataPoints)
			case metricdata.Gauge[float64]:
				count += len(data.DataPoints)
			case metricdata.Sum[int64]:
				count += len(data.DataPoints)
			case metricdata.Sum[float64]:
				count += len(data.DataPoints)
			case metricdata.Histogram[int64]:
				count += len(data.DataPoints)
			case metricdata.Histogram[float64]:
				count += len(data.DataPoints)
			case metricdata.ExponentialHistogram[int64]:
				count += len(data.DataPoints)
			case metricdata.ExponentialHistogram[float64]:
				count += len(data.DataPoints)
			case metricdata.Summary:
				count += len(data.DataPoints)
			}
		}
	}
	return count
}
