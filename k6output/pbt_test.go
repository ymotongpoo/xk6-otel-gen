// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6output

import (
	"context"
	"math"
	"testing"
	"time"

	"go.k6.io/k6/metrics"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"pgregory.net/rapid"
)

func TestOutput_Robustness_AllStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "Start_AddSamples_Stop",
			run: func(t *testing.T) {
				o := newTestOutput(t, "endpoint=localhost:4317,insecure=true,timeout=1ms,batchTimeout=1h")
				o.logger = func(string, ...any) {}
				if err := o.Start(); err != nil {
					t.Fatalf("Start() error = %v, want nil", err)
				}
				o.AddMetricSamples([]metrics.SampleContainer{testK6Sample(t, "iterations", metrics.Counter, 1, nil)})
				if err := o.Stop(); err != nil {
					t.Fatalf("Stop() error = %v, want nil", err)
				}
			},
		},
		{
			name: "AddSamples_BeforeStart",
			run: func(t *testing.T) {
				o := newTestOutput(t, "")
				o.AddMetricSamples([]metrics.SampleContainer{testK6Sample(t, "iterations", metrics.Counter, 1, nil)})
			},
		},
		{
			name: "Stop_BeforeStart",
			run: func(t *testing.T) {
				o := newTestOutput(t, "")
				if err := o.Stop(); err != nil {
					t.Fatalf("Stop() error = %v, want nil", err)
				}
			},
		},
		{
			name: "Stop_AfterStop_NoOp",
			run: func(t *testing.T) {
				o := newTestOutput(t, "")
				if err := o.Stop(); err != nil {
					t.Fatalf("Stop() first error = %v, want nil", err)
				}
				if err := o.Stop(); err != nil {
					t.Fatalf("Stop() second error = %v, want nil", err)
				}
			},
		},
		{
			name: "AddSamples_AfterStop",
			run: func(t *testing.T) {
				o := newTestOutput(t, "")
				if err := o.Stop(); err != nil {
					t.Fatalf("Stop() error = %v, want nil", err)
				}
				o.AddMetricSamples([]metrics.SampleContainer{testK6Sample(t, "iterations", metrics.Counter, 1, nil)})
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertNoPanic(t, func() { tt.run(t) })
		})
	}
}

func TestCounter_Monotonic_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		values := rapid.SliceOfN(rapid.Float64Range(0.001, 1000), 1, 50).Draw(t, "counter_values")
		o, reader := newManualEmitter(t)
		want := 0.0
		for _, value := range values {
			want += value
			o.emitSample(testK6SampleRapid(t, "iterations", metrics.Counter, value, nil))
		}

		rm := collectManualMetrics(t, reader)
		got, ok := sumMetricValue(rm, "k6.iterations.total")
		if !ok {
			t.Fatalf("metric k6.iterations.total not found in %#v", rm.ScopeMetrics)
		}
		if math.Abs(got-want) > math.Max(1e-9, want*1e-12) {
			t.Fatalf("counter sum = %v, want %v", got, want)
		}
	})
}

func TestTag_Attribute_RoundTrip_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		tags := validK6Tags(t)
		o, reader := newManualEmitter(t)
		o.emitSample(testK6SampleRapid(t, "iterations", metrics.Counter, 1, tags))

		rm := collectManualMetrics(t, reader)
		point, ok := firstSumDataPoint(rm, "k6.iterations.total")
		if !ok {
			t.Fatalf("metric k6.iterations.total not found in %#v", rm.ScopeMetrics)
		}
		for key, want := range tags {
			got, ok := point.Attributes.Value(attribute.Key("k6.tag." + key))
			if !ok {
				t.Fatalf("attribute k6.tag.%s missing from %v", key, point.Attributes)
			}
			if got.AsString() != want {
				t.Fatalf("attribute k6.tag.%s = %q, want %q", key, got.AsString(), want)
			}
		}
	})
}

func assertNoPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	f()
}

func newManualEmitter(t *rapid.T) (*Output, *sdkmetric.ManualReader) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	o := &Output{
		params:        defaultParams(),
		meterProvider: mp,
		setCache:      &tagSetCache{},
		logger:        func(string, ...any) {},
	}
	if err := o.buildKnownInstruments(); err != nil {
		t.Fatalf("buildKnownInstruments() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mp.Shutdown(ctx)
	})
	return o, reader
}

func collectManualMetrics(t *rapid.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("ManualReader.Collect() error = %v", err)
	}
	return rm
}

func sumMetricValue(rm metricdata.ResourceMetrics, name string) (float64, bool) {
	sum := 0.0
	found := false
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			data, ok := metric.Data.(metricdata.Sum[float64])
			if !ok {
				continue
			}
			for _, point := range data.DataPoints {
				sum += point.Value
				found = true
			}
		}
	}
	return sum, found
}

func firstSumDataPoint(rm metricdata.ResourceMetrics, name string) (metricdata.DataPoint[float64], bool) {
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			data, ok := metric.Data.(metricdata.Sum[float64])
			if !ok || len(data.DataPoints) == 0 {
				continue
			}
			return data.DataPoints[0], true
		}
	}
	return metricdata.DataPoint[float64]{}, false
}

func validK6Tags(t *rapid.T) map[string]string {
	count := rapid.IntRange(0, 5).Draw(t, "tag_count")
	keys := rapid.SliceOfNDistinct(
		rapid.StringMatching(`^[A-Za-z][A-Za-z0-9_-]{0,10}$`),
		count,
		count,
		func(key string) string { return key },
	).Draw(t, "tag_keys")
	tags := make(map[string]string, len(keys))
	for _, key := range keys {
		tags[key] = rapid.StringMatching(`^[A-Za-z0-9_.:/ -]{0,20}$`).Draw(t, "tag_value_"+key)
	}
	return tags
}

func testK6Sample(t *testing.T, name string, typ metrics.MetricType, value float64, tags map[string]string) metrics.Sample {
	t.Helper()

	registry := metrics.NewRegistry()
	metric := registry.MustNewMetric(name, typ)
	return metrics.Sample{
		TimeSeries: metrics.TimeSeries{
			Metric: metric,
			Tags:   registry.RootTagSet().WithTagsFromMap(tags),
		},
		Time:  time.Now(),
		Value: value,
	}
}

func testK6SampleRapid(t *rapid.T, name string, typ metrics.MetricType, value float64, tags map[string]string) metrics.Sample {
	registry := metrics.NewRegistry()
	metric, err := registry.NewMetric(name, typ)
	if err != nil {
		t.Fatalf("NewMetric(%q) error = %v", name, err)
	}
	return metrics.Sample{
		TimeSeries: metrics.TimeSeries{
			Metric: metric,
			Tags:   registry.RootTagSet().WithTagsFromMap(tags),
		},
		Time:  time.Now(),
		Value: value,
	}
}
