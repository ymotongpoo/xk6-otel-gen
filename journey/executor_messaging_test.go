// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestExecute_Messaging_EmitsProducerConsumerWithLink(t *testing.T) {
	t.Parallel()

	spanExp := tracetest.NewInMemoryExporter()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	syn := synth.NewDefault(&reproFactory{spanExp: spanExp, logRec: &reproLogRecorder{}}, mp)
	engine := NewEngineWithSeed(newMessagingJourneySchema(), newMessagingJourneySchema().ApplyFaults(), syn, 42)
	plan := engine.impl.plans["place-order"]
	if plan == nil {
		t.Fatal("no place-order plan")
	}
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var producerSC trace.SpanContext
	var consumerSpan tracetest.SpanStub
	var producerTrace, consumerTrace trace.TraceID
	for _, span := range spanExp.GetSpans() {
		switch span.SpanKind {
		case trace.SpanKindProducer:
			producerSC = span.SpanContext
			producerTrace = span.SpanContext.TraceID()
		case trace.SpanKindConsumer:
			consumerSpan = span
			consumerTrace = span.SpanContext.TraceID()
		}
	}
	if !producerSC.IsValid() {
		t.Fatal("no PRODUCER span emitted")
	}
	if !consumerSpan.SpanContext.IsValid() {
		t.Fatal("no CONSUMER span emitted")
	}
	if producerTrace != consumerTrace {
		t.Fatalf("trace_id mismatch: producer=%s consumer=%s", producerTrace, consumerTrace)
	}
	if len(consumerSpan.Links) != 1 {
		t.Fatalf("consumer Links = %d, want 1", len(consumerSpan.Links))
	}
	if !consumerSpan.Links[0].SpanContext.Equal(producerSC) {
		t.Fatalf("consumer link = %+v, want producer %+v", consumerSpan.Links[0].SpanContext, producerSC)
	}
}

func TestExecute_MessagingProducer_PreservesSeededHTTPMetrics(t *testing.T) {
	t.Parallel()

	const seed = uint64(0xC0FFEE)
	httpSnap := executeHTTPMetricsSnapshot(t, newPlanTestSchema(), seed)
	if again := executeHTTPMetricsSnapshot(t, newPlanTestSchema(), seed); !metricsSnapshotsEqual(httpSnap, again) {
		t.Fatal("HTTP metrics snapshot not deterministic for same seed")
	}

	msgSnap := executeHTTPMetricsSnapshot(t, newMessagingHTTPPrefixSchema(), seed)
	if len(httpSnap) < 1 || len(msgSnap) < 1 {
		t.Fatalf("snapshots = %d and %d, want at least 1 HTTP hop each", len(httpSnap), len(msgSnap))
	}
	if httpSnap[0] != msgSnap[0] {
		t.Fatalf("first HTTP hop mismatch: http=%+v messaging=%+v", httpSnap[0], msgSnap[0])
	}
}

func TestMessagingPublishLatency_Deterministic(t *testing.T) {
	t.Parallel()

	edge := &topology.Edge{
		Latency: topology.LatencyDist{P50: 40 * time.Millisecond},
	}
	if got := messagingPublishLatency(edge); got != 10*time.Millisecond {
		t.Fatalf("messagingPublishLatency() = %s, want 10ms", got)
	}
	if got := messagingPublishLatency(nil); got != defaultEntryLatency {
		t.Fatalf("messagingPublishLatency(nil) = %s, want %s", got, defaultEntryLatency)
	}
}

type metricHopSnapshot struct {
	operation   string
	instanceIdx int
	latency     time.Duration
}

func executeHTTPMetricsSnapshot(t *testing.T, schema *topology.Schema, seed uint64) []metricHopSnapshot {
	t.Helper()

	mock := newMockSynth()
	engine := NewEngineWithSeed(schema, schema.ApplyFaults(), mock, seed)
	name := firstJourneyName(schema)
	plan, err := engine.BuildPlan(name)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var out []metricHopSnapshot
	for _, span := range mock.snapshotSpans() {
		if span.Input.Edge == nil || span.Input.Edge.Protocol != topology.ProtocolHTTP {
			continue
		}
		out = append(out, metricHopSnapshot{
			operation:   span.Input.Operation,
			instanceIdx: span.Input.InstanceIdx,
			latency:     span.Outcome.EndTime.Sub(span.Input.StartTime),
		})
	}
	return out
}

func metricsSnapshotsEqual(a, b []metricHopSnapshot) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newMessagingJourneySchema() *topology.Schema {
	checkoutSvc := &topology.Service{
		Name:       "checkout",
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: make(map[string]*topology.Operation),
	}
	kafkaSvc := &topology.Service{
		Name:       "kafka",
		Kind:       topology.KindQueue,
		Replicas:   2,
		Operations: make(map[string]*topology.Operation),
	}
	publish := &topology.Operation{Name: "publish_order", Service: checkoutSvc}
	consume := &topology.Operation{Name: "consume_order", Service: kafkaSvc}
	checkoutSvc.Operations[publish.Name] = publish
	kafkaSvc.Operations[consume.Name] = consume

	latency := topology.LatencyDist{Distribution: "fixed", P50: 40 * time.Millisecond, P95: 40 * time.Millisecond}
	edge := &topology.Edge{
		From:     publish,
		To:       consume,
		Protocol: topology.ProtocolMessaging,
		Latency:  latency,
	}
	publish.Calls = []*topology.CallNode{{Edge: edge}}

	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{
			checkoutSvc.Name: checkoutSvc,
			kafkaSvc.Name:    kafkaSvc,
		},
		Journeys: map[string]*topology.Journey{
			"place-order": {Name: "place-order", Steps: []*topology.Step{{Op: publish}}, Weight: 1},
		},
	}
}

func newMessagingHTTPPrefixSchema() *topology.Schema {
	schema := newPlanTestSchema()
	payments := schema.Services["payments"]
	kafkaSvc := &topology.Service{
		Name:       "kafka",
		Kind:       topology.KindQueue,
		Replicas:   2,
		Operations: make(map[string]*topology.Operation),
	}
	consume := &topology.Operation{Name: "consume_order", Service: kafkaSvc}
	kafkaSvc.Operations[consume.Name] = consume
	schema.Services[kafkaSvc.Name] = kafkaSvc

	charge := payments.Operations["POST /charge"]
	delete(schema.Services, "db")
	charge.Calls = []*topology.CallNode{{
		Edge: &topology.Edge{
			From:     charge,
			To:       consume,
			Protocol: topology.ProtocolMessaging,
			Latency:  topology.LatencyDist{Distribution: "fixed"},
		},
	}}
	return schema
}
