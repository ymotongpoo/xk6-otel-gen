// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"time"

	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/metrics"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

const (
	metricTracesExported  = "otel_gen_traces_exported"
	metricTracesFailed    = "otel_gen_traces_failed"
	metricMetricsExported = "otel_gen_metrics_exported"
	metricMetricsFailed   = "otel_gen_metrics_failed"
	metricLogsExported    = "otel_gen_logs_exported"
	metricLogsFailed      = "otel_gen_logs_failed"
	metricQueueDrops      = "otel_gen_queue_drops"
)

type nativeMetrics struct {
	counters   map[string]*metrics.Metric
	queueDrops *metrics.Metric
	rootTags   *metrics.TagSet
}

func newNativeMetrics(vu modules.VU) *nativeMetrics {
	if vu == nil || vu.InitEnv() == nil || vu.InitEnv().Registry == nil {
		return nil
	}
	registry := vu.InitEnv().Registry
	names := []string{
		metricTracesExported,
		metricTracesFailed,
		metricMetricsExported,
		metricMetricsFailed,
		metricLogsExported,
		metricLogsFailed,
	}
	nm := &nativeMetrics{
		counters: make(map[string]*metrics.Metric, len(names)),
		rootTags: registry.RootTagSet(),
	}
	for _, name := range names {
		metric, err := registry.NewMetric(name, metrics.Counter)
		if err != nil {
			return nil
		}
		nm.counters[name] = metric
	}
	queueDrops, err := registry.NewMetric(metricQueueDrops, metrics.Gauge)
	if err != nil {
		return nil
	}
	nm.queueDrops = queueDrops
	return nm
}

func (i *ModuleInstance) emitExporterStats() {
	if i == nil || i.nativeMetrics == nil {
		return
	}
	pipeline, err := i.getOrBuildPipeline()
	if err != nil {
		return
	}
	i.emitExporterStatsSnapshot(pipeline.Stats())
}

func (i *ModuleInstance) emitExporterStatsSnapshot(current exporter.Stats) {
	if i == nil || i.nativeMetrics == nil || i.vu == nil || i.vu.State() == nil || i.vu.State().Samples == nil {
		i.lastStats = current
		return
	}

	previous := i.lastStats
	i.lastStats = current
	deltas := map[string]int64{
		metricTracesExported:  current.TracesExported - previous.TracesExported,
		metricTracesFailed:    current.TracesFailed - previous.TracesFailed,
		metricMetricsExported: current.MetricsExported - previous.MetricsExported,
		metricMetricsFailed:   current.MetricsFailed - previous.MetricsFailed,
		metricLogsExported:    current.LogsExported - previous.LogsExported,
		metricLogsFailed:      current.LogsFailed - previous.LogsFailed,
	}

	tags := i.nativeMetrics.rootTags
	if stateTags := i.vu.State().Tags; stateTags != nil {
		tags = stateTags.GetCurrentValues().Tags
	}
	now := time.Now()
	samples := make(metrics.Samples, 0, len(deltas)+1)
	for name, delta := range deltas {
		if delta <= 0 {
			continue
		}
		samples = append(samples, metrics.Sample{
			TimeSeries: metrics.TimeSeries{Metric: i.nativeMetrics.counters[name], Tags: tags},
			Time:       now,
			Value:      float64(delta),
		})
	}
	if i.nativeMetrics.queueDrops != nil {
		samples = append(samples, metrics.Sample{
			TimeSeries: metrics.TimeSeries{Metric: i.nativeMetrics.queueDrops, Tags: tags},
			Time:       now,
			Value:      0,
		})
	}
	if len(samples) == 0 {
		return
	}
	metrics.PushIfNotDone(i.vu.Context(), i.vu.State().Samples, samples)
}
