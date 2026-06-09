package exporter

import (
	"context"
	"errors"
	"fmt"
	"testing"

	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"pgregory.net/rapid"
)

func TestStats_Monotonic_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		fixture := newStatsMonotonicFixture()
		nOps := rapid.IntRange(1, 20).Draw(t, "n_ops")
		var prev Stats
		for i := 0; i < nOps; i++ {
			fixture.simulateExport(t, i)
			current := fixture.pipeline.Stats()
			assertStatsNonDecreasing(t, prev, current)
			prev = current
		}
	})
}

type statsMonotonicFixture struct {
	pipeline *Pipeline
	trace    *tracingExporter
	metric   *metricExporter
	log      *loggingExporter

	traceInner  *fakeSpanExporter
	metricInner *fakeMetricExporter
	logInner    *fakeLogExporter
}

func newStatsMonotonicFixture() *statsMonotonicFixture {
	stats := &pipelineStats{}
	traceInner := &fakeSpanExporter{}
	metricInner := &fakeMetricExporter{}
	logInner := &fakeLogExporter{}
	return &statsMonotonicFixture{
		pipeline:    &Pipeline{stats: stats},
		trace:       &tracingExporter{inner: traceInner, stats: stats},
		metric:      &metricExporter{inner: metricInner, stats: stats},
		log:         &loggingExporter{inner: logInner, stats: stats},
		traceInner:  traceInner,
		metricInner: metricInner,
		logInner:    logInner,
	}
}

func (f *statsMonotonicFixture) simulateExport(t *rapid.T, index int) {
	fail := rapid.Bool().Draw(t, fmt.Sprintf("op_%d_fail", index))
	batchSize := rapid.IntRange(0, 10).Draw(t, fmt.Sprintf("op_%d_batch_size", index))
	switch rapid.IntRange(0, 2).Draw(t, fmt.Sprintf("op_%d_signal", index)) {
	case 0:
		f.traceInner.err = maybeErr(fail)
		_ = f.trace.ExportSpans(context.Background(), make([]sdktrace.ReadOnlySpan, batchSize))
	case 1:
		f.metricInner.err = maybeErr(fail)
		_ = f.metric.Export(context.Background(), resourceMetricsWith(metricdata.Sum[int64]{
			DataPoints: make([]metricdata.DataPoint[int64], batchSize),
		}))
	default:
		f.logInner.err = maybeErr(fail)
		_ = f.log.Export(context.Background(), make([]sdklog.Record, batchSize))
	}
}

func maybeErr(fail bool) error {
	if fail {
		return errors.New("simulated export failure")
	}
	return nil
}
