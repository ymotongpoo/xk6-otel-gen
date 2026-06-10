package synth

import (
	"context"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func TestBuildResource_Idempotent_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		svc := generators.ValidService().Draw(rt, "svc")
		idx := rapid.IntRange(0, svc.Replicas-1).Draw(rt, "idx")

		first := BuildResource(svc, idx)
		second := BuildResource(svc, idx)
		if !first.Equal(second) {
			rt.Fatalf("BuildResource not idempotent for svc=%q idx=%d", svc.Name, idx)
		}
	})
}

func TestSpanAttributes_AllowedKeysOnly_Property(t *testing.T) {
	t.Parallel()

	tp, mp, lp, spanExporter, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	start := time.Unix(1_700_000_000, 0)

	rapid.Check(t, func(rt *rapid.T) {
		spanExporter.Reset()
		in := localSpanInput(rt, start)
		_, finish := syn.BeginSpan(context.Background(), in)
		finish(Outcome{
			Success:    false,
			StatusCode: statusCodeForProtocol(protocolFor(in.Edge)),
			ErrorType:  "timeout",
			EndTime:    start.Add(time.Millisecond),
		})

		spans := spanExporter.GetSpans()
		if len(spans) != 1 {
			rt.Fatalf("spans = %d, want 1", len(spans))
		}
		span := spans[0]
		for _, kv := range span.Attributes {
			if _, ok := allowedAttrKeys[string(kv.Key)]; !ok {
				rt.Fatalf("attribute key %q not in allowedAttrKeys", kv.Key)
			}
		}
	})
}

func localSpanInput(t *rapid.T, start time.Time) SpanInput {
	kind := rapid.SampledFrom([]topology.ServiceKind{
		topology.KindApplication,
		topology.KindDatabase,
		topology.KindExternalAPI,
		topology.KindCache,
		topology.KindQueue,
	}).Draw(t, "service_kind")
	protocol := rapid.SampledFrom([]topology.Protocol{
		topology.ProtocolHTTP,
		topology.ProtocolGRPC,
		topology.ProtocolMessaging,
	}).Draw(t, "protocol")
	dir := rapid.SampledFrom([]direction{
		dirClient,
		dirServer,
		dirProducer,
		dirConsumer,
		dirInternal,
	}).Draw(t, "direction")
	op := rapid.SampledFrom([]string{
		"GET /api/users",
		"POST /checkout",
		"Checkout/Get",
		"SELECT users",
		"orders",
	}).Draw(t, "operation")

	svc := makeSpanService("svc", kind)
	edge := localEdgeFor(svc, protocol, dir)
	return SpanInput{
		Service:     svc,
		Edge:        edge,
		Operation:   op,
		StartTime:   start,
		InstanceIdx: 0,
	}
}

func localEdgeFor(svc *topology.Service, protocol topology.Protocol, dir direction) *topology.Edge {
	if dir == dirInternal {
		other := makeSpanService("other", topology.KindApplication)
		target := makeSpanService("target", topology.KindExternalAPI)
		return &topology.Edge{
			From:     &topology.Operation{Name: "GET /other", Service: other},
			To:       &topology.Operation{Name: "GET /target", Service: target},
			Protocol: protocol,
		}
	}
	otherKind := topology.KindApplication
	if protocol == topology.ProtocolMessaging {
		otherKind = topology.KindQueue
	}
	other := makeSpanService("target", otherKind)
	if dir == dirClient || dir == dirProducer {
		return &topology.Edge{
			From:     &topology.Operation{Name: "GET /source", Service: svc},
			To:       &topology.Operation{Name: "GET /target", Service: other},
			Protocol: protocol,
		}
	}
	return &topology.Edge{
		From:     &topology.Operation{Name: "GET /source", Service: other},
		To:       &topology.Operation{Name: "GET /target", Service: svc},
		Protocol: protocol,
	}
}

func statusCodeForProtocol(protocol topology.Protocol) int {
	if protocol == topology.ProtocolGRPC {
		return 14
	}
	return 500
}
