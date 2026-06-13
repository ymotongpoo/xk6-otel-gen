// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"testing"

	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"go.k6.io/k6/metrics"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

func TestNativeMetrics_RegisterAndEmitDeltas(t *testing.T) {
	t.Parallel()

	vu := newFakeVU(t, 1)
	instance := &ModuleInstance{vu: vu, nativeMetrics: newNativeMetrics(vu)}
	for _, name := range []string{
		metricTracesExported,
		metricTracesFailed,
		metricMetricsExported,
		metricMetricsFailed,
		metricLogsExported,
		metricLogsFailed,
		metricQueueDrops,
	} {
		if got := vu.InitEnv().Registry.Get(name); got == nil {
			t.Fatalf("registry missing %q", name)
		}
	}

	instance.emitExporterStatsSnapshot(exporter.Stats{
		TracesExported:  3,
		TracesFailed:    1,
		MetricsExported: 5,
		LogsExported:    7,
	})

	got := readMetricSamples(t, vu.(*fakeVU).samples)
	want := map[string]float64{
		metricTracesExported:  3,
		metricTracesFailed:    1,
		metricMetricsExported: 5,
		metricLogsExported:    7,
		metricQueueDrops:      0,
	}
	for name, value := range want {
		if got[name] != value {
			t.Fatalf("%s sample = %v, want %v (all samples %v)", name, got[name], value, got)
		}
	}
}

func TestNativeMetrics_WarnsOnExporterFailureDeltas(t *testing.T) {
	t.Parallel()

	logger, hook := logrustest.NewNullLogger()
	vu := newFakeVUWithLogger(t, 1, logger)
	instance := &ModuleInstance{vu: vu, logger: logger, nativeMetrics: newNativeMetrics(vu)}

	instance.emitExporterStatsSnapshot(exporter.Stats{
		TracesFailed:  2,
		MetricsFailed: 3,
		LogsFailed:    4,
	})

	entry := findEntryByLevelAndMessage(t, hook.AllEntries(), logrus.WarnLevel, "xk6-otel-gen: exporter failures observed")
	if entry.Data["traces_failed"] != int64(2) || entry.Data["metrics_failed"] != int64(3) || entry.Data["logs_failed"] != int64(4) || entry.Data["total_failed"] != int64(9) {
		t.Fatalf("warn fields = %#v, want failure deltas", entry.Data)
	}
}

func TestNativeMetrics_NilInstanceSafe(t *testing.T) {
	t.Parallel()

	var instance *ModuleInstance
	instance.emitExporterStatsSnapshot(exporter.Stats{TracesFailed: 1})
}

func readMetricSamples(t *testing.T, ch <-chan metrics.SampleContainer) map[string]float64 {
	t.Helper()

	select {
	case container := <-ch:
		out := map[string]float64{}
		for _, sample := range container.GetSamples() {
			out[sample.Metric.Name] += sample.Value
		}
		return out
	default:
		t.Fatal("no samples emitted")
		return nil
	}
}

func findEntryByLevelAndMessage(t *testing.T, entries []*logrus.Entry, level logrus.Level, message string) *logrus.Entry {
	t.Helper()
	for _, entry := range entries {
		if entry.Level == level && entry.Message == message {
			return entry
		}
	}
	t.Fatalf("missing %s log message %q in %d entries", level, message, len(entries))
	return nil
}
