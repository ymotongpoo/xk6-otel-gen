package synth

import (
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

type direction uint8

const (
	dirClient direction = iota
	dirServer
	dirProducer
	dirConsumer
	dirInternal
	dirUnset
)

type attributePolicy struct {
	SpanKind           trace.SpanKind
	AttributeNamespace string
	MetricNamespace    string
	Direction          direction
}

func policyFor(svcKind topology.ServiceKind, protocol topology.Protocol, dir direction) attributePolicy {
	switch svcKind {
	case topology.KindDatabase, topology.KindCache:
		return attributePolicy{
			SpanKind:           trace.SpanKindClient,
			AttributeNamespace: "db",
			MetricNamespace:    "db",
			Direction:          dirClient,
		}
	case topology.KindQueue:
		return messagingPolicy(dir)
	case topology.KindExternalAPI:
		return attributePolicy{
			SpanKind:           trace.SpanKindClient,
			AttributeNamespace: "http",
			MetricNamespace:    "http",
			Direction:          dirClient,
		}
	case topology.KindApplication:
		switch protocol {
		case topology.ProtocolHTTP:
			if dir == dirUnset || dir == dirInternal {
				return internalPolicy()
			}
			return requestPolicy("http", dir)
		case topology.ProtocolGRPC:
			if dir == dirUnset || dir == dirInternal {
				return internalPolicy()
			}
			return requestPolicy("rpc", dir)
		case topology.ProtocolMessaging:
			return messagingPolicy(dir)
		default:
			return internalPolicy()
		}
	default:
		return internalPolicy()
	}
}

type cacheKey struct {
	svcName string
	op      string
	edgeID  string
	dir     direction
}

type staticSetCache struct {
	sets sync.Map
}

func (c *staticSetCache) get(k cacheKey) (attribute.Set, bool) {
	value, ok := c.sets.Load(k)
	if !ok {
		return attribute.Set{}, false
	}
	return value.(attribute.Set), true
}

func (c *staticSetCache) put(k cacheKey, set attribute.Set) {
	c.sets.Store(k, set)
}

func buildStaticSet(svc *topology.Service, op string, edge *topology.Edge, policy attributePolicy) attribute.Set {
	kvs := []attribute.KeyValue{semconv.ServiceName(string(svc.Name))}
	switch policy.AttributeNamespace {
	case "http":
		kvs = append(kvs, httpStaticAttrs(svc, op, edge, policy.Direction)...)
	case "rpc":
		kvs = append(kvs, rpcStaticAttrs(svc, op, edge, policy.Direction)...)
	case "db":
		kvs = append(kvs, dbStaticAttrs(svc, op, edge)...)
	case "messaging":
		kvs = append(kvs, messagingStaticAttrs(svc, op, edge, policy.Direction)...)
	}
	return attribute.NewSet(kvs...)
}

func httpStaticAttrs(svc *topology.Service, op string, edge *topology.Edge, dir direction) []attribute.KeyValue {
	method, route := parseHTTPOp(op)
	kvs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(method),
	}
	switch dir {
	case dirServer:
		kvs = append(kvs, semconv.HTTPRouteKey.String(route))
	case dirClient:
		kvs = append(kvs, semconv.URLPathKey.String(route))
		if target := edgeTargetServiceName(edge); target != "" {
			kvs = append(kvs, semconv.ServerAddressKey.String(target))
			if svc.Kind == topology.KindExternalAPI || edgeTargetServiceKind(edge) == topology.KindExternalAPI {
				kvs = append(kvs, attribute.String("peer.service", target))
			}
		}
	}
	return kvs
}

func rpcStaticAttrs(svc *topology.Service, op string, edge *topology.Edge, dir direction) []attribute.KeyValue {
	serviceName := string(svc.Name)
	if dir == dirClient {
		if target := edgeTargetServiceName(edge); target != "" {
			serviceName = target
		}
	}
	return []attribute.KeyValue{
		semconv.RPCSystemKey.String("grpc"),
		semconv.RPCServiceKey.String(serviceName),
		semconv.RPCMethodKey.String(op),
	}
}

func dbStaticAttrs(svc *topology.Service, op string, _ *topology.Edge) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.DBSystemKey.String(dbSystem(svc)),
		semconv.DBOperationNameKey.String(op),
	}
}

func messagingStaticAttrs(svc *topology.Service, op string, edge *topology.Edge, dir direction) []attribute.KeyValue {
	operation := "publish"
	if dir == dirConsumer {
		operation = "receive"
	}
	destination := edgeTargetServiceName(edge)
	if destination == "" {
		destination = op
	}
	return []attribute.KeyValue{
		semconv.MessagingSystemKey.String(messagingSystem(svc)),
		semconv.MessagingOperationNameKey.String(operation),
		semconv.MessagingDestinationNameKey.String(destination),
	}
}

func dynamicOutcomeAttrs(policy attributePolicy, outcome Outcome) []attribute.KeyValue {
	kvs := make([]attribute.KeyValue, 0, 2)
	if outcome.StatusCode != 0 {
		switch policy.AttributeNamespace {
		case "http":
			kvs = append(kvs, semconv.HTTPResponseStatusCodeKey.Int(outcome.StatusCode))
		case "rpc":
			kvs = append(kvs, semconv.RPCGRPCStatusCodeKey.Int(outcome.StatusCode))
		}
	}
	if !outcome.Success && outcome.ErrorType != "" {
		kvs = append(kvs, semconv.ErrorTypeKey.String(outcome.ErrorType))
	}
	return kvs
}

var allowedAttrKeys = map[string]struct{}{
	string(semconv.ServiceNameKey):              {},
	string(semconv.HTTPRequestMethodKey):        {},
	string(semconv.HTTPResponseStatusCodeKey):   {},
	string(semconv.HTTPRouteKey):                {},
	string(semconv.ServerAddressKey):            {},
	string(semconv.ServerPortKey):               {},
	string(semconv.URLPathKey):                  {},
	string(semconv.RPCSystemKey):                {},
	string(semconv.RPCServiceKey):               {},
	string(semconv.RPCMethodKey):                {},
	string(semconv.RPCGRPCStatusCodeKey):        {},
	string(semconv.DBSystemKey):                 {},
	string(semconv.DBOperationNameKey):          {},
	string(semconv.MessagingSystemKey):          {},
	string(semconv.MessagingOperationNameKey):   {},
	string(semconv.MessagingDestinationNameKey): {},
	string(semconv.ErrorTypeKey):                {},
	"peer.service":                              {},
	"outcome":                                   {},
	"synth.service.framework":                   {},
}

func cacheKeyFor(svc *topology.Service, op string, edge *topology.Edge, dir direction) cacheKey {
	return cacheKey{
		svcName: string(svc.Name),
		op:      op,
		edgeID:  edgeID(edge),
		dir:     dir,
	}
}

func parseHTTPOp(op string) (method, route string) {
	fields := strings.Fields(op)
	if len(fields) >= 2 && isHTTPMethod(fields[0]) {
		return fields[0], fields[1]
	}
	trimmed := strings.TrimSpace(op)
	if trimmed == "" {
		return "GET", "/"
	}
	if strings.HasPrefix(trimmed, "/") {
		return "GET", trimmed
	}
	return "GET", "/" + strings.ReplaceAll(trimmed, " ", "-")
}

func requestPolicy(namespace string, dir direction) attributePolicy {
	if dir != dirClient && dir != dirServer {
		dir = dirInternal
	}
	return attributePolicy{
		SpanKind:           spanKindFor(dir),
		AttributeNamespace: namespace,
		MetricNamespace:    namespace,
		Direction:          dir,
	}
}

func messagingPolicy(dir direction) attributePolicy {
	if dir != dirProducer && dir != dirConsumer {
		dir = dirProducer
	}
	return attributePolicy{
		SpanKind:           spanKindFor(dir),
		AttributeNamespace: "messaging",
		MetricNamespace:    "messaging",
		Direction:          dir,
	}
}

func internalPolicy() attributePolicy {
	return attributePolicy{
		SpanKind:  trace.SpanKindInternal,
		Direction: dirInternal,
	}
}

func spanKindFor(dir direction) trace.SpanKind {
	switch dir {
	case dirClient:
		return trace.SpanKindClient
	case dirServer:
		return trace.SpanKindServer
	case dirProducer:
		return trace.SpanKindProducer
	case dirConsumer:
		return trace.SpanKindConsumer
	default:
		return trace.SpanKindInternal
	}
}

func edgeID(edge *topology.Edge) string {
	if edge == nil {
		return ""
	}
	from, to := "", ""
	if edge.From != nil {
		from = operationID(edge.From)
	}
	if edge.To != nil {
		to = operationID(edge.To)
	}
	return from + "->" + to + "/" + edge.Protocol.String()
}

func operationID(op *topology.Operation) string {
	if op == nil {
		return ""
	}
	service := ""
	if op.Service != nil {
		service = string(op.Service.Name)
	}
	return service + "." + op.Name
}

func edgeTargetServiceName(edge *topology.Edge) string {
	if edge == nil || edge.To == nil || edge.To.Service == nil {
		return ""
	}
	return string(edge.To.Service.Name)
}

func edgeTargetServiceKind(edge *topology.Edge) topology.ServiceKind {
	if edge == nil || edge.To == nil || edge.To.Service == nil {
		return topology.KindApplication
	}
	return edge.To.Service.Kind
}

func dbSystem(svc *topology.Service) string {
	if svc.Framework != "" {
		return svc.Framework
	}
	if svc.Kind == topology.KindCache {
		return "redis"
	}
	return "postgresql"
}

func messagingSystem(svc *topology.Service) string {
	if svc.Framework != "" {
		return svc.Framework
	}
	return "kafka"
}

func isHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}
