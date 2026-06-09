package exporter

import (
	"context"
	"errors"
	"sync"
	"testing"

	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestPipelineStats_Snapshot_AtomicLoad(t *testing.T) {
	t.Parallel()

	stats := &pipelineStats{}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1_000; j++ {
				stats.tracesExported.Add(1)
				stats.tracesFailed.Add(1)
				stats.metricsExported.Add(1)
				stats.metricsFailed.Add(1)
				stats.logsExported.Add(1)
				stats.logsFailed.Add(1)
			}
		}()
	}

	var prev Stats
	for i := 0; i < 1_000; i++ {
		current := stats.snapshot()
		assertStatsNonDecreasing(t, prev, current)
		prev = current
	}
	wg.Wait()
	final := stats.snapshot()
	assertStatsNonDecreasing(t, prev, final)
	if final.TracesExported != 8_000 || final.TracesFailed != 8_000 || final.MetricsExported != 8_000 || final.MetricsFailed != 8_000 || final.LogsExported != 8_000 || final.LogsFailed != 8_000 {
		t.Fatalf("final snapshot = %#v, want all counters at 8000", final)
	}
}

func TestTracingExporter_Success(t *testing.T) {
	t.Parallel()

	stats := &pipelineStats{}
	inner := &fakeSpanExporter{}
	exp := &tracingExporter{inner: inner, stats: stats}
	spans := []sdktrace.ReadOnlySpan{nil, nil, nil}
	if err := exp.ExportSpans(context.Background(), spans); err != nil {
		t.Fatalf("ExportSpans() error = %v, want nil", err)
	}
	if got := stats.snapshot(); got.TracesExported != 3 || got.TracesFailed != 0 {
		t.Fatalf("stats = %#v, want 3 exported and 0 failed", got)
	}
	if inner.exports != 1 {
		t.Fatalf("inner exports = %d, want 1", inner.exports)
	}
}

func TestTracingExporter_Failure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("trace export failed")
	stats := &pipelineStats{}
	exp := &tracingExporter{inner: &fakeSpanExporter{err: sentinel}, stats: stats}
	if err := exp.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{nil}); !errors.Is(err, sentinel) {
		t.Fatalf("ExportSpans() error = %v, want %v", err, sentinel)
	}
	if got := stats.snapshot(); got.TracesExported != 0 || got.TracesFailed != 1 {
		t.Fatalf("stats = %#v, want 0 exported and 1 failed", got)
	}
}

func TestMetricExporter_Success(t *testing.T) {
	t.Parallel()

	stats := &pipelineStats{}
	inner := &fakeMetricExporter{}
	exp := &metricExporter{inner: inner, stats: stats}
	rm := resourceMetricsWith(metricdata.Gauge[int64]{DataPoints: make([]metricdata.DataPoint[int64], 2)})
	if err := exp.Export(context.Background(), rm); err != nil {
		t.Fatalf("Export() error = %v, want nil", err)
	}
	if got := stats.snapshot(); got.MetricsExported != 2 || got.MetricsFailed != 0 {
		t.Fatalf("stats = %#v, want 2 exported and 0 failed", got)
	}
	if inner.exports != 1 {
		t.Fatalf("inner exports = %d, want 1", inner.exports)
	}
	if got := exp.Temporality(sdkmetric.InstrumentKindCounter); got != metricdata.CumulativeTemporality {
		t.Fatalf("Temporality() = %v, want cumulative", got)
	}
	if got := exp.Aggregation(sdkmetric.InstrumentKindCounter); got == nil {
		t.Fatal("Aggregation() = nil, want default aggregation")
	}
}

func TestMetricExporter_Failure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("metric export failed")
	stats := &pipelineStats{}
	exp := &metricExporter{inner: &fakeMetricExporter{err: sentinel}, stats: stats}
	if err := exp.Export(context.Background(), resourceMetricsWith(metricdata.Sum[int64]{DataPoints: make([]metricdata.DataPoint[int64], 2)})); !errors.Is(err, sentinel) {
		t.Fatalf("Export() error = %v, want %v", err, sentinel)
	}
	if got := stats.snapshot(); got.MetricsExported != 0 || got.MetricsFailed != 1 {
		t.Fatalf("stats = %#v, want 0 exported and 1 failed", got)
	}
}

func TestLoggingExporter_Success(t *testing.T) {
	t.Parallel()

	stats := &pipelineStats{}
	inner := &fakeLogExporter{}
	exp := &loggingExporter{inner: inner, stats: stats}
	if err := exp.Export(context.Background(), make([]sdklog.Record, 4)); err != nil {
		t.Fatalf("Export() error = %v, want nil", err)
	}
	if got := stats.snapshot(); got.LogsExported != 4 || got.LogsFailed != 0 {
		t.Fatalf("stats = %#v, want 4 exported and 0 failed", got)
	}
	if inner.exports != 1 {
		t.Fatalf("inner exports = %d, want 1", inner.exports)
	}
}

func TestLoggingExporter_Failure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("log export failed")
	stats := &pipelineStats{}
	exp := &loggingExporter{inner: &fakeLogExporter{err: sentinel}, stats: stats}
	if err := exp.Export(context.Background(), make([]sdklog.Record, 2)); !errors.Is(err, sentinel) {
		t.Fatalf("Export() error = %v, want %v", err, sentinel)
	}
	if got := stats.snapshot(); got.LogsExported != 0 || got.LogsFailed != 1 {
		t.Fatalf("stats = %#v, want 0 exported and 1 failed", got)
	}
}

func TestCountMetricDataPoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data metricdata.Aggregation
		want int
	}{
		{name: "nil resource metrics", data: nil, want: 0},
		{name: "gauge int64", data: metricdata.Gauge[int64]{DataPoints: make([]metricdata.DataPoint[int64], 2)}, want: 2},
		{name: "gauge float64", data: metricdata.Gauge[float64]{DataPoints: make([]metricdata.DataPoint[float64], 3)}, want: 3},
		{name: "sum int64", data: metricdata.Sum[int64]{DataPoints: make([]metricdata.DataPoint[int64], 4)}, want: 4},
		{name: "sum float64", data: metricdata.Sum[float64]{DataPoints: make([]metricdata.DataPoint[float64], 5)}, want: 5},
		{name: "histogram int64", data: metricdata.Histogram[int64]{DataPoints: make([]metricdata.HistogramDataPoint[int64], 6)}, want: 6},
		{name: "histogram float64", data: metricdata.Histogram[float64]{DataPoints: make([]metricdata.HistogramDataPoint[float64], 7)}, want: 7},
		{name: "exponential histogram int64", data: metricdata.ExponentialHistogram[int64]{DataPoints: make([]metricdata.ExponentialHistogramDataPoint[int64], 8)}, want: 8},
		{name: "exponential histogram float64", data: metricdata.ExponentialHistogram[float64]{DataPoints: make([]metricdata.ExponentialHistogramDataPoint[float64], 9)}, want: 9},
		{name: "summary", data: metricdata.Summary{DataPoints: make([]metricdata.SummaryDataPoint, 10)}, want: 10},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var rm *metricdata.ResourceMetrics
			if tt.data != nil {
				rm = resourceMetricsWith(tt.data)
			}
			if got := countMetricDataPoints(rm); got != tt.want {
				t.Fatalf("countMetricDataPoints() = %d, want %d", got, tt.want)
			}
		})
	}
}

type fakeSpanExporter struct {
	err      error
	exports  int
	shutdown int
}

func (f *fakeSpanExporter) ExportSpans(_ context.Context, _ []sdktrace.ReadOnlySpan) error {
	f.exports++
	return f.err
}

func (f *fakeSpanExporter) Shutdown(context.Context) error {
	f.shutdown++
	return nil
}

type fakeMetricExporter struct {
	err      error
	exports  int
	shutdown int
}

func (f *fakeMetricExporter) Temporality(sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (f *fakeMetricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

func (f *fakeMetricExporter) Export(_ context.Context, _ *metricdata.ResourceMetrics) error {
	f.exports++
	return f.err
}

func (f *fakeMetricExporter) ForceFlush(context.Context) error {
	return nil
}

func (f *fakeMetricExporter) Shutdown(context.Context) error {
	f.shutdown++
	return nil
}

type fakeLogExporter struct {
	err      error
	exports  int
	shutdown int
}

func (f *fakeLogExporter) Export(_ context.Context, _ []sdklog.Record) error {
	f.exports++
	return f.err
}

func (f *fakeLogExporter) ForceFlush(context.Context) error {
	return nil
}

func (f *fakeLogExporter) Shutdown(context.Context) error {
	f.shutdown++
	return nil
}

func resourceMetricsWith(data metricdata.Aggregation) *metricdata.ResourceMetrics {
	return &metricdata.ResourceMetrics{
		ScopeMetrics: []metricdata.ScopeMetrics{
			{
				Metrics: []metricdata.Metrics{
					{Data: data},
				},
			},
		},
	}
}

// failHelper is the minimal subset of *testing.T / *rapid.T needed by
// assertStatsNonDecreasing. Using a local interface avoids coupling to
// the full testing.TB surface (which gained methods like Attr in Go 1.25
// that rapid.T does not yet implement).
type failHelper interface {
	Helper()
	Fatalf(format string, args ...any)
}

func assertStatsNonDecreasing(t failHelper, prev, current Stats) {
	t.Helper()
	if current.TracesExported < prev.TracesExported ||
		current.TracesFailed < prev.TracesFailed ||
		current.MetricsExported < prev.MetricsExported ||
		current.MetricsFailed < prev.MetricsFailed ||
		current.LogsExported < prev.LogsExported ||
		current.LogsFailed < prev.LogsFailed {
		t.Fatalf("stats decreased: prev=%#v current=%#v", prev, current)
	}
}
