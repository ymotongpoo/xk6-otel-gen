// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"testing"

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
