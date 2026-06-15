// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey_test

import (
	"context"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/journey"
	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"pgregory.net/rapid"
)

func TestExecute_MessagingSpanLink_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		p50 := time.Duration(rapid.IntRange(4, 400).Draw(rt, "p50_ms")) * time.Millisecond
		schema := messagingSchemaWithLatency(p50)
		spanExp := tracetest.NewInMemoryExporter()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))
		defer func() { _ = mp.Shutdown(context.Background()) }()

		syn := synth.NewDefault(&messagingReproFactory{spanExp: spanExp}, mp, nil)
		engine := journey.NewEngineWithSeed(schema, schema.ApplyFaults(), syn, rapid.Uint64().Draw(rt, "seed"))
		plan, err := engine.BuildPlan("place-order")
		if err != nil {
			rt.Fatalf("BuildPlan() error = %v", err)
		}
		if err := engine.Execute(context.Background(), plan); err != nil {
			rt.Fatalf("Execute() error = %v", err)
		}

		var producerSC trace.SpanContext
		var consumerSpan tracetest.SpanStub
		var producerTrace trace.TraceID
		for _, span := range spanExp.GetSpans() {
			switch span.SpanKind {
			case trace.SpanKindProducer:
				producerSC = span.SpanContext
				producerTrace = span.SpanContext.TraceID()
			case trace.SpanKindConsumer:
				consumerSpan = span
			}
		}
		if !producerSC.IsValid() || !consumerSpan.SpanContext.IsValid() {
			rt.Fatal("missing producer or consumer span")
		}
		if producerTrace != consumerSpan.SpanContext.TraceID() {
			rt.Fatalf("trace_id mismatch")
		}
		if len(consumerSpan.Links) != 1 || !consumerSpan.Links[0].SpanContext.Equal(producerSC) {
			rt.Fatalf("consumer missing producer span link")
		}
	})
}

type messagingReproFactory struct {
	spanExp *tracetest.InMemoryExporter
}

func (f *messagingReproFactory) TracerProviderForService(_ string, res *sdkresource.Resource) trace.TracerProvider {
	return sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithSyncer(f.spanExp))
}

func (f *messagingReproFactory) LoggerProviderForService(string, *sdkresource.Resource) log.LoggerProvider {
	return sdklog.NewLoggerProvider()
}

func messagingSchemaWithLatency(p50 time.Duration) *topology.Schema {
	checkoutSvc := &topology.Service{Name: "checkout", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	kafkaSvc := &topology.Service{Name: "kafka", Kind: topology.KindQueue, Replicas: 2, Operations: make(map[string]*topology.Operation)}
	publish := &topology.Operation{Name: "publish_order", Service: checkoutSvc}
	consume := &topology.Operation{Name: "consume_order", Service: kafkaSvc}
	checkoutSvc.Operations[publish.Name] = publish
	kafkaSvc.Operations[consume.Name] = consume
	edge := &topology.Edge{
		From:     publish,
		To:       consume,
		Protocol: topology.ProtocolMessaging,
		Latency:  topology.LatencyDist{Distribution: "fixed", P50: p50, P95: p50},
	}
	publish.Calls = []*topology.CallNode{{Edge: edge}}
	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{checkoutSvc.Name: checkoutSvc, kafkaSvc.Name: kafkaSvc},
		Journeys: map[string]*topology.Journey{"place-order": {Name: "place-order", Steps: []*topology.Step{{Op: publish}}, Weight: 1}},
	}
}
