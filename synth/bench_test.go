// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func BenchmarkBuildResource(b *testing.B) {
	svc := &topology.Service{
		Name:      "checkout",
		Kind:      topology.KindApplication,
		Replicas:  1,
		Version:   "1.2.3",
		Language:  "go",
		Framework: "gin",
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BuildResource(svc, 0)
	}
}

func BenchmarkBeginSpan_HTTP_Server(b *testing.B) {
	syn, spanExporter := newBenchSynthesizer(b)
	start := time.Unix(1_700_000_000, 0)
	in := SpanInput{
		Service:     makeSpanService("frontend", topology.KindApplication),
		Operation:   "GET /checkout",
		StartTime:   start,
		InstanceIdx: 0,
	}
	outcome := Outcome{Success: true, StatusCode: 200, EndTime: start.Add(10 * time.Millisecond)}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, finish := syn.BeginSpan(context.Background(), in)
		finish(outcome)
		if i%1024 == 0 {
			spanExporter.Reset()
		}
	}
}

func BenchmarkRecordMetric_HTTP_Server(b *testing.B) {
	syn, _ := newBenchSynthesizer(b)
	in := MetricInput{
		Service:     makeSpanService("frontend", topology.KindApplication),
		Operation:   "GET /checkout",
		Latency:     10 * time.Millisecond,
		Outcome:     Outcome{Success: true, StatusCode: 200},
		InstanceIdx: 0,
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		syn.RecordMetric(context.Background(), in)
	}
}

func BenchmarkEmitLog(b *testing.B) {
	syn, _ := newBenchSynthesizer(b)
	in := LogInput{
		Service:  makeSpanService("frontend", topology.KindApplication),
		Severity: log.SeverityInfo,
		Body:     "frontend event",
		Attributes: map[string]any{
			"route":   "/checkout",
			"attempt": 1,
		},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		syn.EmitLog(context.Background(), in)
	}
}

func newBenchSynthesizer(b *testing.B) (Synthesizer, *tracetest.InMemoryExporter) {
	b.Helper()

	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	lp := sdklog.NewLoggerProvider()
	b.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
		_ = lp.Shutdown(context.Background())
	})
	return NewDefault(singleProviderFactory{tp: tp, lp: lp}, mp), spanExporter
}
