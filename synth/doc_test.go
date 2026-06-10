package synth_test

import (
	"context"
	"fmt"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

func ExampleNewDefault() {
	tp := sdktrace.NewTracerProvider()
	mp := sdkmetric.NewMeterProvider()
	lp := sdklog.NewLoggerProvider()

	syn := synth.NewDefault(tp, mp, lp)
	if syn != nil {
		fmt.Println("synthesizer ready")
	}

	// Output:
	// synthesizer ready
}

func ExampleBuildResource() {
	svc := &topology.Service{
		Name:      "checkout",
		Replicas:  2,
		Version:   "1.2.3",
		Language:  "go",
		Framework: "gin",
	}
	res := synth.BuildResource(svc, 0)
	attrs := map[string]string{}
	for _, kv := range res.Attributes() {
		attrs[string(kv.Key)] = kv.Value.AsString()
	}

	fmt.Println(attrs[string(semconv.ServiceNameKey)])
	fmt.Println(attrs[string(semconv.ServiceVersionKey)])
	fmt.Println(attrs[string(semconv.ProcessRuntimeNameKey)])
	fmt.Println(attrs["synth.service.framework"])

	// Output:
	// checkout
	// 1.2.3
	// go
	// gin
}

func ExampleSynthesizer_BeginSpan() {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	mp := sdkmetric.NewMeterProvider()
	lp := sdklog.NewLoggerProvider()
	syn := synth.NewDefault(tp, mp, lp)

	start := time.Unix(1_700_000_000, 0)
	ctx, finish := syn.BeginSpan(context.Background(), synth.SpanInput{
		Service:     &topology.Service{Name: "frontend", Kind: topology.KindApplication, Replicas: 1},
		Operation:   "GET /checkout",
		StartTime:   start,
		InstanceIdx: 0,
	})
	_ = ctx
	finish(synth.Outcome{Success: true, StatusCode: 200, EndTime: start.Add(20 * time.Millisecond)})

	fmt.Println(len(exporter.GetSpans()))

	// Output:
	// 1
}
