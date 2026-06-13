// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6output

import (
	"context"
	"strconv"
	"testing"
	"time"

	"go.k6.io/k6/metrics"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func BenchmarkAddMetricSamples(b *testing.B) {
	o := &Output{
		params: defaultParams(),
		queue:  make(chan metrics.SampleContainer, 1024),
		logger: func(string, ...any) {},
	}
	o.ctx, o.cancelFn = context.WithCancel(context.Background())
	b.Cleanup(o.cancelFn)
	sample := benchmarkK6Sample(b, "iterations", metrics.Counter, 1, nil)
	containers := []metrics.SampleContainer{sample}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o.AddMetricSamples(containers)
	}
}

func BenchmarkFlushLoop(b *testing.B) {
	o := newBenchmarkEmitter(b)
	sample := benchmarkK6Sample(b, "iterations", metrics.Counter, 1, map[string]string{"method": "GET", "status": "200"})
	o.emitSample(sample)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o.emitContainer(sample)
	}
}

func BenchmarkTagSetCache_Hit(b *testing.B) {
	cache := &tagSetCache{}
	tags := map[string]string{"method": "GET", "status": "200", "name": "/api"}
	cache.get(tags)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.get(tags)
	}
}

func BenchmarkTagSetCache_Miss(b *testing.B) {
	cache := &tagSetCache{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.get(map[string]string{"name": "/api/" + strconv.Itoa(i)})
	}
}

func BenchmarkInstrumentLookup(b *testing.B) {
	o := newBenchmarkEmitter(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = o.instruments.counters.Load("iterations")
	}
}

func newBenchmarkEmitter(b *testing.B) *Output {
	b.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	o := &Output{
		params:        defaultParams(),
		meterProvider: mp,
		setCache:      &tagSetCache{},
		logger:        func(string, ...any) {},
	}
	if err := o.buildKnownInstruments(); err != nil {
		b.Fatalf("buildKnownInstruments() error = %v", err)
	}
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mp.Shutdown(ctx)
	})
	return o
}

func benchmarkK6Sample(b *testing.B, name string, typ metrics.MetricType, value float64, tags map[string]string) metrics.Sample {
	b.Helper()

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
