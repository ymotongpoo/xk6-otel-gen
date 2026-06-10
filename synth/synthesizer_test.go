package synth

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestNewDefault_NilProvider_Panics(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, _ := newTestProviders(t)
	tests := []struct {
		name string
		run  func()
	}{
		{name: "trace provider", run: func() { NewDefault(nil, mp, lp) }},
		{name: "meter provider", run: func() { NewDefault(tp, nil, lp) }},
		{name: "logger provider", run: func() { NewDefault(tp, mp, nil) }},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requirePanic(t, tt.run)
		})
	}
}

func TestNewDefault_BuildsAllInstruments(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	s, ok := syn.(*defaultSynthesizer)
	if !ok {
		t.Fatalf("NewDefault returned %T, want *defaultSynthesizer", syn)
	}

	if s.tracer == nil {
		t.Fatal("tracer is nil")
	}
	if s.meter == nil {
		t.Fatal("meter is nil")
	}
	if s.logger == nil {
		t.Fatal("logger is nil")
	}
	if s.httpClientDur == nil {
		t.Fatal("httpClientDur is nil")
	}
	if s.httpServerDur == nil {
		t.Fatal("httpServerDur is nil")
	}
	if s.httpActiveReq == nil {
		t.Fatal("httpActiveReq is nil")
	}
	if s.rpcClientDur == nil {
		t.Fatal("rpcClientDur is nil")
	}
	if s.rpcServerDur == nil {
		t.Fatal("rpcServerDur is nil")
	}
	if s.rpcActiveReq == nil {
		t.Fatal("rpcActiveReq is nil")
	}
	if s.dbClientDur == nil {
		t.Fatal("dbClientDur is nil")
	}
	if s.msgProducerDur == nil {
		t.Fatal("msgProducerDur is nil")
	}
	if s.msgConsumerDur == nil {
		t.Fatal("msgConsumerDur is nil")
	}
	if s.staticSetCache == nil {
		t.Fatal("staticSetCache is nil")
	}
}

func TestBeginSpan_Server_Success(t *testing.T) {
	t.Parallel()

	tp, mp, lp, spanExporter, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	start := time.Unix(1_700_000_000, 0)
	ctx, finish := syn.BeginSpan(context.Background(), SpanInput{
		Service:     makeSpanService("frontend", topology.KindApplication),
		Operation:   "GET /checkout",
		StartTime:   start,
		InstanceIdx: 0,
	})
	finish(Outcome{Success: true, StatusCode: 200, EndTime: start.Add(20 * time.Millisecond)})

	if ctx == context.Background() {
		t.Fatal("BeginSpan returned unchanged context")
	}
	span := requireSingleSpan(t, spanExporter.GetSpans())
	if span.Name != "frontend.GET /checkout" {
		t.Fatalf("span.Name = %q", span.Name)
	}
	if span.SpanKind != trace.SpanKindServer {
		t.Fatalf("SpanKind = %v, want server", span.SpanKind)
	}
	if span.Status.Code != codes.Unset {
		t.Fatalf("Status.Code = %v, want Unset", span.Status.Code)
	}
	attrs := resourceAttrs(span.Attributes)
	requireAttr(t, attrs, semconv.ServiceNameKey, "frontend")
	requireAttr(t, attrs, semconv.HTTPRequestMethodKey, "GET")
	requireAttr(t, attrs, semconv.HTTPRouteKey, "/checkout")
	requireAttr(t, attrs, semconv.HTTPResponseStatusCodeKey, "200")
}

func TestBeginSpan_Server_Failure_500(t *testing.T) {
	t.Parallel()

	tp, mp, lp, spanExporter, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	start := time.Unix(1_700_000_000, 0)
	_, finish := syn.BeginSpan(context.Background(), SpanInput{
		Service:     makeSpanService("frontend", topology.KindApplication),
		Operation:   "GET /checkout",
		StartTime:   start,
		InstanceIdx: 0,
	})
	finish(Outcome{Success: false, StatusCode: 500, ErrorType: "http.500", EndTime: start.Add(time.Millisecond)})

	span := requireSingleSpan(t, spanExporter.GetSpans())
	if span.Status.Code != codes.Error {
		t.Fatalf("Status.Code = %v, want Error", span.Status.Code)
	}
	attrs := resourceAttrs(span.Attributes)
	requireAttr(t, attrs, semconv.HTTPResponseStatusCodeKey, "500")
	requireAttr(t, attrs, semconv.ErrorTypeKey, "http.500")
}

func TestFinishFn_CascadedAttribute(t *testing.T) {
	t.Parallel()

	tp, mp, lp, spanExporter, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	start := time.Unix(1_700_000_000, 0)
	_, finish := syn.BeginSpan(context.Background(), SpanInput{
		Service:     makeSpanService("frontend", topology.KindApplication),
		Operation:   "GET /checkout",
		StartTime:   start,
		InstanceIdx: 0,
	})
	finish(Outcome{
		Success:    false,
		StatusCode: 503,
		ErrorType:  "connection_refused",
		EndTime:    start.Add(time.Millisecond),
		Cascaded:   true,
	})

	span := requireSingleSpan(t, spanExporter.GetSpans())
	if !hasBoolAttr(span.Attributes, attribute.Key("synth.cascaded"), true) {
		t.Fatalf("span attributes = %v, want synth.cascaded=true", span.Attributes)
	}
}

func TestBeginSpan_4xx_NoErrorStatus(t *testing.T) {
	t.Parallel()

	tp, mp, lp, spanExporter, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	start := time.Unix(1_700_000_000, 0)
	_, finish := syn.BeginSpan(context.Background(), SpanInput{
		Service:     makeSpanService("frontend", topology.KindApplication),
		Operation:   "GET /missing",
		StartTime:   start,
		InstanceIdx: 0,
	})
	finish(Outcome{Success: false, StatusCode: 404, ErrorType: "http.404", EndTime: start.Add(time.Millisecond)})

	span := requireSingleSpan(t, spanExporter.GetSpans())
	if span.Status.Code != codes.Unset {
		t.Fatalf("Status.Code = %v, want Unset", span.Status.Code)
	}
}

func TestBeginSpan_Client_HTTP(t *testing.T) {
	t.Parallel()

	tp, mp, lp, spanExporter, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	source, edge := makeSpanEdge(topology.KindApplication, topology.KindExternalAPI, topology.ProtocolHTTP)
	start := time.Unix(1_700_000_000, 0)
	_, finish := syn.BeginSpan(context.Background(), SpanInput{
		Service:     source,
		Edge:        edge,
		Operation:   "POST /payments",
		StartTime:   start,
		InstanceIdx: 0,
	})
	finish(Outcome{Success: true, StatusCode: 200, EndTime: start.Add(time.Millisecond)})

	span := requireSingleSpan(t, spanExporter.GetSpans())
	if span.SpanKind != trace.SpanKindClient {
		t.Fatalf("SpanKind = %v, want client", span.SpanKind)
	}
	attrs := resourceAttrs(span.Attributes)
	requireAttr(t, attrs, semconv.ServerAddressKey, "target")
	requireAttr(t, attrs, semconv.URLPathKey, "/payments")
}

func TestBeginSpan_InvalidInput_Panics(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	tests := []struct {
		name string
		in   SpanInput
	}{
		{name: "nil service", in: SpanInput{Operation: "GET /", InstanceIdx: 0}},
		{name: "empty operation", in: SpanInput{Service: makeSpanService("frontend", topology.KindApplication), InstanceIdx: 0}},
		{name: "out of range", in: SpanInput{Service: makeSpanService("frontend", topology.KindApplication), Operation: "GET /", InstanceIdx: 1}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requirePanic(t, func() {
				syn.BeginSpan(context.Background(), tt.in)
			})
		})
	}
}

func TestFinishSpanFunc_DoubleCall_NoOp(t *testing.T) {
	t.Parallel()
	if raceEnabled {
		t.Skip("race builds panic on duplicate finish")
	}

	tp, mp, lp, spanExporter, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	start := time.Unix(1_700_000_000, 0)
	_, finish := syn.BeginSpan(context.Background(), SpanInput{
		Service:     makeSpanService("frontend", topology.KindApplication),
		Operation:   "GET /",
		StartTime:   start,
		InstanceIdx: 0,
	})
	outcome := Outcome{Success: true, StatusCode: 200, EndTime: start.Add(time.Millisecond)}
	finish(outcome)
	finish(outcome)

	if got := len(spanExporter.GetSpans()); got != 1 {
		t.Fatalf("ended spans = %d, want 1", got)
	}
}

func TestFinishSpanFunc_DoubleCall_RacePanic(t *testing.T) {
	t.Parallel()
	if !raceEnabled {
		t.Skip("duplicate finish only panics in race builds")
	}

	tp, mp, lp, _, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	start := time.Unix(1_700_000_000, 0)
	_, finish := syn.BeginSpan(context.Background(), SpanInput{
		Service:     makeSpanService("frontend", topology.KindApplication),
		Operation:   "GET /",
		StartTime:   start,
		InstanceIdx: 0,
	})
	outcome := Outcome{Success: true, StatusCode: 200, EndTime: start.Add(time.Millisecond)}
	finish(outcome)
	requirePanic(t, func() {
		finish(outcome)
	})
}

func TestActiveRequests_BalancedAfterFinish(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, reader, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	start := time.Unix(1_700_000_000, 0)
	var finishes []FinishSpanFunc
	for i := 0; i < 5; i++ {
		_, finish := syn.BeginSpan(context.Background(), SpanInput{
			Service:     makeSpanService("frontend", topology.KindApplication),
			Operation:   "GET /checkout",
			StartTime:   start.Add(time.Duration(i) * time.Millisecond),
			InstanceIdx: 0,
		})
		finishes = append(finishes, finish)
	}
	for i, finish := range finishes {
		finish(Outcome{Success: true, StatusCode: 200, EndTime: start.Add(time.Duration(i+10) * time.Millisecond)})
	}

	if got := int64MetricValue(t, reader, "http.server.active_requests"); got != 0 {
		t.Fatalf("active_requests = %d, want 0", got)
	}
}

func TestRecordMetric_HTTP_Server(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, reader, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	syn.RecordMetric(context.Background(), MetricInput{
		Service:     makeSpanService("frontend", topology.KindApplication),
		Operation:   "GET /checkout",
		Latency:     25 * time.Millisecond,
		Outcome:     Outcome{Success: true, StatusCode: 200},
		InstanceIdx: 0,
	})

	point := histogramPoint(t, reader, "http.server.request.duration")
	if point.Count != 1 {
		t.Fatalf("Count = %d, want 1", point.Count)
	}
	attrs := resourceAttrs(point.Attributes.ToSlice())
	requireAttr(t, attrs, semconv.HTTPRequestMethodKey, "GET")
	requireAttr(t, attrs, semconv.HTTPRouteKey, "/checkout")
	requireAttr(t, attrs, semconv.HTTPResponseStatusCodeKey, "200")
}

func TestRecordMetric_RPC_Client(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, reader, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	source, edge := makeSpanEdge(topology.KindApplication, topology.KindApplication, topology.ProtocolGRPC)
	syn.RecordMetric(context.Background(), MetricInput{
		Service:     source,
		Edge:        edge,
		Operation:   "Checkout/Get",
		Latency:     10 * time.Millisecond,
		Outcome:     Outcome{Success: false, StatusCode: 14, ErrorType: "grpc.unavailable"},
		InstanceIdx: 0,
	})

	point := histogramPoint(t, reader, "rpc.client.duration")
	attrs := resourceAttrs(point.Attributes.ToSlice())
	requireAttr(t, attrs, semconv.RPCSystemKey, "grpc")
	requireAttr(t, attrs, semconv.RPCServiceKey, "target")
	requireAttr(t, attrs, semconv.RPCGRPCStatusCodeKey, "14")
	requireAttr(t, attrs, semconv.ErrorTypeKey, "grpc.unavailable")
}

func TestRecordMetric_DB_Client(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, reader, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	syn.RecordMetric(context.Background(), MetricInput{
		Service:     &topology.Service{Name: "postgres", Kind: topology.KindDatabase, Replicas: 1, Framework: "postgresql"},
		Operation:   "SELECT users",
		Latency:     3 * time.Millisecond,
		Outcome:     Outcome{Success: true},
		InstanceIdx: 0,
	})

	point := histogramPoint(t, reader, "db.client.operation.duration")
	attrs := resourceAttrs(point.Attributes.ToSlice())
	requireAttr(t, attrs, semconv.DBSystemKey, "postgresql")
	requireAttr(t, attrs, semconv.DBOperationNameKey, "SELECT users")
}

func TestRecordMetric_Messaging_Producer(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, reader, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	source, edge := makeSpanEdge(topology.KindQueue, topology.KindQueue, topology.ProtocolMessaging)
	syn.RecordMetric(context.Background(), MetricInput{
		Service:     source,
		Edge:        edge,
		Operation:   "orders",
		Latency:     12 * time.Millisecond,
		Outcome:     Outcome{Success: true},
		InstanceIdx: 0,
	})

	point := histogramPoint(t, reader, "messaging.publish.duration")
	attrs := resourceAttrs(point.Attributes.ToSlice())
	requireAttr(t, attrs, semconv.MessagingSystemKey, "kafka")
	requireAttr(t, attrs, semconv.MessagingOperationNameKey, "publish")
	requireAttr(t, attrs, semconv.MessagingDestinationNameKey, "target")
}

func TestRecordMetric_ZeroLatency_StillRecords(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, reader, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	syn.RecordMetric(context.Background(), MetricInput{
		Service:     makeSpanService("frontend", topology.KindApplication),
		Operation:   "GET /health",
		Latency:     0,
		Outcome:     Outcome{Success: true, StatusCode: 200},
		InstanceIdx: 0,
	})

	point := histogramPoint(t, reader, "http.server.request.duration")
	if point.Count != 1 {
		t.Fatalf("Count = %d, want 1", point.Count)
	}
	if point.Sum != 0 {
		t.Fatalf("Sum = %f, want 0", point.Sum)
	}
}

func TestRecordMetric_StaticSetCached(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	s := syn.(*defaultSynthesizer)
	in := MetricInput{
		Service:     makeSpanService("frontend", topology.KindApplication),
		Operation:   "GET /cached",
		Latency:     time.Millisecond,
		Outcome:     Outcome{Success: true, StatusCode: 200},
		InstanceIdx: 0,
	}
	policy := policyFor(in.Service.Kind, protocolFor(in.Edge), inferDirection(in.Service, in.Edge))
	key := cacheKeyFor(in.Service, in.Operation, in.Edge, policy.Direction)
	if _, ok := s.staticSetCache.get(key); ok {
		t.Fatal("cache hit before RecordMetric")
	}

	s.RecordMetric(context.Background(), in)
	first, ok := s.staticSetCache.get(key)
	if !ok {
		t.Fatal("cache miss after first RecordMetric")
	}
	s.RecordMetric(context.Background(), in)
	second, ok := s.staticSetCache.get(key)
	if !ok {
		t.Fatal("cache miss after second RecordMetric")
	}
	if !first.Equals(&second) {
		t.Fatalf("cached sets differ: %v vs %v", first.ToSlice(), second.ToSlice())
	}
}

func TestEmitLog_Success(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, recorder := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	syn.EmitLog(context.Background(), LogInput{
		Service:  makeSpanService("frontend", topology.KindApplication),
		Severity: otellog.SeverityInfo,
		Body:     "frontend handled request",
	})

	record := requireSingleLog(t, recorder)
	if record.Severity() != otellog.SeverityInfo {
		t.Fatalf("Severity = %v, want Info", record.Severity())
	}
	if got := record.Body().AsString(); got != "frontend handled request" {
		t.Fatalf("Body = %q", got)
	}
	if record.Timestamp().IsZero() {
		t.Fatal("Timestamp is zero")
	}
	if record.ObservedTimestamp().IsZero() {
		t.Fatal("ObservedTimestamp is zero")
	}
}

func TestEmitLog_NilService_Panics(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	requirePanic(t, func() {
		syn.EmitLog(context.Background(), LogInput{Body: "missing service"})
	})
}

func TestEmitLog_EmptyBody_DefaultFallback(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, recorder := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	syn.EmitLog(context.Background(), LogInput{Service: makeSpanService("frontend", topology.KindApplication)})

	record := requireSingleLog(t, recorder)
	if got := record.Body().AsString(); got != "frontend event" {
		t.Fatalf("Body = %q, want frontend event", got)
	}
	if record.Severity() != otellog.SeverityInfo {
		t.Fatalf("Severity = %v, want Info", record.Severity())
	}
}

func TestEmitLog_AttributesPropagated(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, recorder := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	syn.EmitLog(context.Background(), LogInput{
		Service: makeSpanService("frontend", topology.KindApplication),
		Body:    "with attrs",
		Attributes: map[string]any{
			"attempt": 2,
			"cache":   true,
			"route":   "/checkout",
		},
	})

	attrs := logAttrs(requireSingleLog(t, recorder))
	if attrs["attempt"] != "2" {
		t.Fatalf("attempt = %q, want 2", attrs["attempt"])
	}
	if attrs["cache"] != "true" {
		t.Fatalf("cache = %q, want true", attrs["cache"])
	}
	if attrs["route"] != "/checkout" {
		t.Fatalf("route = %q, want /checkout", attrs["route"])
	}
}

func TestEmitLog_ServiceNameAuto(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, recorder := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	syn.EmitLog(context.Background(), LogInput{
		Service: makeSpanService("frontend", topology.KindApplication),
		Body:    "service attr",
	})

	attrs := logAttrs(requireSingleLog(t, recorder))
	if attrs[string(semconv.ServiceNameKey)] != "frontend" {
		t.Fatalf("service.name = %q, want frontend", attrs[string(semconv.ServiceNameKey)])
	}
}

func requireSingleSpan(t *testing.T, spans tracetest.SpanStubs) tracetest.SpanStub {
	t.Helper()

	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	return spans[0]
}

func int64MetricValue(t *testing.T, reader metricReader, name string) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			sum, ok := metric.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("%s data = %T, want metricdata.Sum[int64]", name, metric.Data)
			}
			var total int64
			for _, point := range sum.DataPoints {
				total += point.Value
			}
			return total
		}
	}
	t.Fatalf("metric %q not found", name)
	return 0
}

func histogramPoint(t *testing.T, reader metricReader, name string) metricdata.HistogramDataPoint[float64] {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("%s data = %T, want metricdata.Histogram[float64]", name, metric.Data)
			}
			if len(histogram.DataPoints) != 1 {
				t.Fatalf("%s datapoints = %d, want 1", name, len(histogram.DataPoints))
			}
			return histogram.DataPoints[0]
		}
	}
	t.Fatalf("histogram %q not found", name)
	return metricdata.HistogramDataPoint[float64]{}
}

func requireSingleLog(t *testing.T, recorder *logRecorder) sdklog.Record {
	t.Helper()

	records := recorder.Records()
	if len(records) != 1 {
		t.Fatalf("logs = %d, want 1", len(records))
	}
	return records[0]
}

func logAttrs(record sdklog.Record) map[string]string {
	attrs := map[string]string{}
	record.WalkAttributes(func(kv otellog.KeyValue) bool {
		attrs[kv.Key] = logValueString(kv.Value)
		return true
	})
	return attrs
}

func logValueString(value otellog.Value) string {
	switch value.Kind() {
	case otellog.KindString:
		return value.AsString()
	case otellog.KindBool:
		if value.AsBool() {
			return "true"
		}
		return "false"
	case otellog.KindInt64:
		return value.String()
	case otellog.KindFloat64:
		return value.String()
	default:
		return value.String()
	}
}

func hasBoolAttr(kvs []attribute.KeyValue, key attribute.Key, want bool) bool {
	for _, kv := range kvs {
		if kv.Key == key && kv.Value.AsBool() == want {
			return true
		}
	}
	return false
}

type metricReader interface {
	Collect(context.Context, *metricdata.ResourceMetrics) error
}

func makeSpanService(name string, kind topology.ServiceKind) *topology.Service {
	return &topology.Service{Name: topology.ServiceID(name), Kind: kind, Replicas: 1}
}

func makeSpanEdge(sourceKind, targetKind topology.ServiceKind, protocol topology.Protocol) (*topology.Service, *topology.Edge) {
	source := makeSpanService("source", sourceKind)
	target := makeSpanService("target", targetKind)
	from := &topology.Operation{Name: "POST /payments", Service: source}
	to := &topology.Operation{Name: "POST /payments", Service: target}
	return source, &topology.Edge{From: from, To: to, Protocol: protocol}
}
