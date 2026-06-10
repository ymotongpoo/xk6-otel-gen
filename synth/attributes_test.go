package synth

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestPolicyFor_AllCombinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		kind      topology.ServiceKind
		protocol  topology.Protocol
		dir       direction
		wantKind  trace.SpanKind
		wantAttrs string
		wantMet   string
		wantDir   direction
	}{
		{name: "application http client", kind: topology.KindApplication, protocol: topology.ProtocolHTTP, dir: dirClient, wantKind: trace.SpanKindClient, wantAttrs: "http", wantMet: "http", wantDir: dirClient},
		{name: "application http server", kind: topology.KindApplication, protocol: topology.ProtocolHTTP, dir: dirServer, wantKind: trace.SpanKindServer, wantAttrs: "http", wantMet: "http", wantDir: dirServer},
		{name: "application rpc client", kind: topology.KindApplication, protocol: topology.ProtocolGRPC, dir: dirClient, wantKind: trace.SpanKindClient, wantAttrs: "rpc", wantMet: "rpc", wantDir: dirClient},
		{name: "application rpc server", kind: topology.KindApplication, protocol: topology.ProtocolGRPC, dir: dirServer, wantKind: trace.SpanKindServer, wantAttrs: "rpc", wantMet: "rpc", wantDir: dirServer},
		{name: "application internal", kind: topology.KindApplication, protocol: topology.ProtocolHTTP, dir: dirUnset, wantKind: trace.SpanKindInternal, wantDir: dirInternal},
		{name: "database", kind: topology.KindDatabase, protocol: topology.ProtocolHTTP, dir: dirClient, wantKind: trace.SpanKindClient, wantAttrs: "db", wantMet: "db", wantDir: dirClient},
		{name: "cache", kind: topology.KindCache, protocol: topology.ProtocolHTTP, dir: dirClient, wantKind: trace.SpanKindClient, wantAttrs: "db", wantMet: "db", wantDir: dirClient},
		{name: "queue producer", kind: topology.KindQueue, protocol: topology.ProtocolMessaging, dir: dirProducer, wantKind: trace.SpanKindProducer, wantAttrs: "messaging", wantMet: "messaging", wantDir: dirProducer},
		{name: "queue consumer", kind: topology.KindQueue, protocol: topology.ProtocolMessaging, dir: dirConsumer, wantKind: trace.SpanKindConsumer, wantAttrs: "messaging", wantMet: "messaging", wantDir: dirConsumer},
		{name: "external api", kind: topology.KindExternalAPI, protocol: topology.ProtocolHTTP, dir: dirClient, wantKind: trace.SpanKindClient, wantAttrs: "http", wantMet: "http", wantDir: dirClient},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := policyFor(tt.kind, tt.protocol, tt.dir)
			if got.SpanKind != tt.wantKind {
				t.Fatalf("SpanKind = %v, want %v", got.SpanKind, tt.wantKind)
			}
			if got.AttributeNamespace != tt.wantAttrs {
				t.Fatalf("AttributeNamespace = %q, want %q", got.AttributeNamespace, tt.wantAttrs)
			}
			if got.MetricNamespace != tt.wantMet {
				t.Fatalf("MetricNamespace = %q, want %q", got.MetricNamespace, tt.wantMet)
			}
			if got.Direction != tt.wantDir {
				t.Fatalf("Direction = %v, want %v", got.Direction, tt.wantDir)
			}
		})
	}
}

func TestBuildStaticSet_HTTP_Server(t *testing.T) {
	t.Parallel()

	svc := &topology.Service{Name: "frontend", Kind: topology.KindApplication}
	set := buildStaticSet(svc, "GET /api/users", nil, policyFor(svc.Kind, topology.ProtocolHTTP, dirServer))
	attrs := resourceAttrs(set.ToSlice())

	requireAttr(t, attrs, semconv.ServiceNameKey, "frontend")
	requireAttr(t, attrs, semconv.HTTPRequestMethodKey, "GET")
	requireAttr(t, attrs, semconv.HTTPRouteKey, "/api/users")
}

func TestBuildStaticSet_HTTP_Client(t *testing.T) {
	t.Parallel()

	svc, edge := makeAttrEdge(topology.KindApplication, topology.KindExternalAPI, topology.ProtocolHTTP)
	set := buildStaticSet(svc, "POST /payments", edge, policyFor(svc.Kind, edge.Protocol, dirClient))
	attrs := resourceAttrs(set.ToSlice())

	requireAttr(t, attrs, semconv.HTTPRequestMethodKey, "POST")
	requireAttr(t, attrs, semconv.URLPathKey, "/payments")
	requireAttr(t, attrs, semconv.ServerAddressKey, "target")
	requireAttr(t, attrs, attribute.Key("peer.service"), "target")
}

func TestBuildStaticSet_RPC_ServerClient(t *testing.T) {
	t.Parallel()

	svc, edge := makeAttrEdge(topology.KindApplication, topology.KindApplication, topology.ProtocolGRPC)
	server := setAttrs(buildStaticSet(svc, "Checkout/Get", edge, policyFor(svc.Kind, edge.Protocol, dirServer)))
	client := setAttrs(buildStaticSet(svc, "Checkout/Get", edge, policyFor(svc.Kind, edge.Protocol, dirClient)))

	requireAttr(t, server, semconv.RPCSystemKey, "grpc")
	requireAttr(t, server, semconv.RPCServiceKey, "source")
	requireAttr(t, server, semconv.RPCMethodKey, "Checkout/Get")
	requireAttr(t, client, semconv.RPCServiceKey, "target")
}

func TestBuildStaticSet_DB(t *testing.T) {
	t.Parallel()

	svc := &topology.Service{Name: "postgres", Kind: topology.KindDatabase, Framework: "postgresql"}
	set := buildStaticSet(svc, "SELECT users", nil, policyFor(svc.Kind, topology.ProtocolHTTP, dirClient))
	attrs := resourceAttrs(set.ToSlice())

	requireAttr(t, attrs, semconv.DBSystemKey, "postgresql")
	requireAttr(t, attrs, semconv.DBOperationNameKey, "SELECT users")
}

func TestBuildStaticSet_MessagingProducerConsumer(t *testing.T) {
	t.Parallel()

	svc, edge := makeAttrEdge(topology.KindQueue, topology.KindQueue, topology.ProtocolMessaging)
	producer := setAttrs(buildStaticSet(svc, "orders", edge, policyFor(svc.Kind, edge.Protocol, dirProducer)))
	consumer := setAttrs(buildStaticSet(svc, "orders", edge, policyFor(svc.Kind, edge.Protocol, dirConsumer)))

	requireAttr(t, producer, semconv.MessagingSystemKey, "kafka")
	requireAttr(t, producer, semconv.MessagingOperationNameKey, "publish")
	requireAttr(t, producer, semconv.MessagingDestinationNameKey, "target")
	requireAttr(t, consumer, semconv.MessagingOperationNameKey, "receive")
}

func TestDynamicOutcomeAttrs_HTTPStatus(t *testing.T) {
	t.Parallel()

	policy := policyFor(topology.KindApplication, topology.ProtocolHTTP, dirServer)
	attrs := resourceAttrs(dynamicOutcomeAttrs(policy, Outcome{Success: true, StatusCode: 204}))

	requireAttr(t, attrs, semconv.HTTPResponseStatusCodeKey, "204")
}

func TestDynamicOutcomeAttrs_RPCFailure(t *testing.T) {
	t.Parallel()

	policy := policyFor(topology.KindApplication, topology.ProtocolGRPC, dirClient)
	attrs := resourceAttrs(dynamicOutcomeAttrs(policy, Outcome{Success: false, StatusCode: 14, ErrorType: "grpc.unavailable"}))

	requireAttr(t, attrs, semconv.RPCGRPCStatusCodeKey, "14")
	requireAttr(t, attrs, semconv.ErrorTypeKey, "grpc.unavailable")
}

func TestDynamicOutcomeAttrs_NoErrorTypeOnSuccess(t *testing.T) {
	t.Parallel()

	policy := policyFor(topology.KindApplication, topology.ProtocolHTTP, dirServer)
	attrs := resourceAttrs(dynamicOutcomeAttrs(policy, Outcome{Success: true, StatusCode: 200, ErrorType: "ignored"}))

	if _, ok := attrs[semconv.ErrorTypeKey]; ok {
		t.Fatal("error.type present for success outcome")
	}
}

func TestStaticSetCache_GetPut(t *testing.T) {
	t.Parallel()

	cache := &staticSetCache{}
	key := cacheKey{svcName: "frontend", op: "GET /", dir: dirServer}
	if _, ok := cache.get(key); ok {
		t.Fatal("cache hit before put")
	}

	set := attribute.NewSet(semconv.HTTPRequestMethodKey.String("GET"))
	cache.put(key, set)
	got, ok := cache.get(key)
	if !ok {
		t.Fatal("cache miss after put")
	}
	if !got.Equals(&set) {
		t.Fatalf("cache set = %v, want %v", got.ToSlice(), set.ToSlice())
	}
}

func makeAttrEdge(sourceKind, targetKind topology.ServiceKind, protocol topology.Protocol) (*topology.Service, *topology.Edge) {
	source := &topology.Service{Name: "source", Kind: sourceKind, Replicas: 1}
	target := &topology.Service{Name: "target", Kind: targetKind, Replicas: 1}
	from := &topology.Operation{Name: "GET /source", Service: source}
	to := &topology.Operation{Name: "GET /target", Service: target}
	return source, &topology.Edge{From: from, To: to, Protocol: protocol}
}

func setAttrs(set attribute.Set) map[attribute.Key]string {
	return resourceAttrs(set.ToSlice())
}
