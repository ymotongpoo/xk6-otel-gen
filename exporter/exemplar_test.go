// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"bytes"
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"pgregory.net/rapid"
)

func TestExemplar_TraceBasedFilter_AttachesTraceContext(t *testing.T) {
	t.Parallel()

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithExemplarFilter(exemplar.TraceBasedFilter),
	)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "work")
	spanCtx := span.SpanContext()
	hist, err := mp.Meter("test").Float64Histogram("latency", metric.WithUnit("s"))
	if err != nil {
		t.Fatalf("Float64Histogram() error = %v", err)
	}
	hist.Record(ctx, 0.042, metric.WithAttributes(attribute.String("outcome", "success")))
	span.End()

	ex := requireHistogramExemplar(t, reader)
	traceID := spanCtx.TraceID()
	spanID := spanCtx.SpanID()
	if !bytes.Equal(ex.TraceID, traceID[:]) {
		t.Fatalf("exemplar TraceID = %x, want %x", ex.TraceID, traceID)
	}
	if !bytes.Equal(ex.SpanID, spanID[:]) {
		t.Fatalf("exemplar SpanID = %x, want %x", ex.SpanID, spanID)
	}
}

func TestExemplar_AlwaysOffSampler_NoExemplars(t *testing.T) {
	t.Parallel()

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithExemplarFilter(exemplar.TraceBasedFilter),
	)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "unsampled")
	hist, err := mp.Meter("test").Float64Histogram("latency", metric.WithUnit("s"))
	if err != nil {
		t.Fatalf("Float64Histogram() error = %v", err)
	}
	hist.Record(ctx, 0.042)
	span.End()

	if count := histogramExemplarCount(t, reader); count != 0 {
		t.Fatalf("exemplar count = %d, want 0 when sampler is always_off", count)
	}
}

func TestExemplar_TraceBasedFilter_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		value := rapid.Float64Range(0, 10).Draw(rt, "value")
		tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
		defer func() { _ = tp.Shutdown(context.Background()) }()
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(reader),
			sdkmetric.WithExemplarFilter(exemplar.TraceBasedFilter),
		)
		defer func() { _ = mp.Shutdown(context.Background()) }()

		ctx, span := tp.Tracer("test").Start(context.Background(), "work")
		spanCtx := span.SpanContext()
		hist, err := mp.Meter("test").Float64Histogram("latency", metric.WithUnit("s"))
		if err != nil {
			rt.Fatalf("Float64Histogram() error = %v", err)
		}
		hist.Record(ctx, value)
		span.End()

		ex := requireHistogramExemplarFromReader(reader)
		traceID := spanCtx.TraceID()
		spanID := spanCtx.SpanID()
		if !bytes.Equal(ex.TraceID, traceID[:]) || !bytes.Equal(ex.SpanID, spanID[:]) {
			rt.Fatalf("exemplar context = (%x,%x), want (%x,%x)", ex.TraceID, ex.SpanID, traceID, spanID)
		}
	})
}

func requireHistogramExemplar(t testing.TB, reader *sdkmetric.ManualReader) metricdata.Exemplar[float64] {
	return requireHistogramExemplarFromReader(reader)
}

func requireHistogramExemplarFromReader(reader *sdkmetric.ManualReader) metricdata.Exemplar[float64] {
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		panic(err)
	}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok {
				continue
			}
			for _, point := range histogram.DataPoints {
				if len(point.Exemplars) == 0 {
					continue
				}
				return point.Exemplars[0]
			}
		}
	}
	panic("no histogram exemplar found")
}

func histogramExemplarCount(t testing.TB, reader *sdkmetric.ManualReader) int {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	count := 0
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok {
				continue
			}
			for _, point := range histogram.DataPoints {
				count += len(point.Exemplars)
			}
		}
	}
	return count
}
