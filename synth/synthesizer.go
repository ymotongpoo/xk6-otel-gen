package synth

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

const instrumentationName = "github.com/ymotongpoo/xk6-otel-gen/synth"

type defaultSynthesizer struct {
	tracer trace.Tracer
	meter  metric.Meter
	logger log.Logger

	httpClientDur  metric.Float64Histogram
	httpServerDur  metric.Float64Histogram
	httpActiveReq  metric.Int64UpDownCounter
	rpcClientDur   metric.Float64Histogram
	rpcServerDur   metric.Float64Histogram
	rpcActiveReq   metric.Int64UpDownCounter
	dbClientDur    metric.Float64Histogram
	msgProducerDur metric.Float64Histogram
	msgConsumerDur metric.Float64Histogram

	staticSetCache *staticSetCache
}

// NewDefault creates the default Synthesizer using injected OpenTelemetry
// providers and eagerly creates all U3 instruments.
func NewDefault(tp trace.TracerProvider, mp metric.MeterProvider, lp log.LoggerProvider) Synthesizer {
	if tp == nil {
		panic("synth: NewDefault: tp must not be nil")
	}
	if mp == nil {
		panic("synth: NewDefault: mp must not be nil")
	}
	if lp == nil {
		panic("synth: NewDefault: lp must not be nil")
	}

	meter := mp.Meter(instrumentationName)
	s := &defaultSynthesizer{
		tracer:         tp.Tracer(instrumentationName),
		meter:          meter,
		logger:         lp.Logger(instrumentationName),
		staticSetCache: &staticSetCache{},
	}

	s.httpClientDur = mustHistogram(meter, "http.client.request.duration", "s")
	s.httpServerDur = mustHistogram(meter, "http.server.request.duration", "s")
	s.httpActiveReq = mustUDC(meter, "http.server.active_requests", "{request}")
	s.rpcClientDur = mustHistogram(meter, "rpc.client.duration", "s")
	s.rpcServerDur = mustHistogram(meter, "rpc.server.duration", "s")
	s.rpcActiveReq = mustUDC(meter, "rpc.server.active_requests", "{request}")
	s.dbClientDur = mustHistogram(meter, "db.client.operation.duration", "s")
	s.msgProducerDur = mustHistogram(meter, "messaging.publish.duration", "s")
	s.msgConsumerDur = mustHistogram(meter, "messaging.receive.duration", "s")

	return s
}

func (s *defaultSynthesizer) BeginSpan(ctx context.Context, in SpanInput) (context.Context, FinishSpanFunc) {
	validateSpanInput(in)

	dir := inferDirection(in.Service, in.Edge)
	protocol := protocolFor(in.Edge)
	policy := policyFor(in.Service.Kind, protocol, dir)
	staticAttrs := s.staticAttrs(in.Service, in.Operation, in.Edge, policy)
	spanAttrs := make([]attribute.KeyValue, 0, staticAttrs.Len())
	spanAttrs = append(spanAttrs, staticAttrs.ToSlice()...)

	ctx, span := s.tracer.Start(ctx, spanName(in.Service, in.Operation),
		trace.WithTimestamp(in.StartTime),
		trace.WithSpanKind(policy.SpanKind),
		trace.WithAttributes(spanAttrs...),
	)
	s.maybeIncActive(ctx, in, policy, 1)

	var finished atomic.Bool
	return ctx, func(outcome Outcome) {
		if !finished.CompareAndSwap(false, true) {
			if raceEnabled {
				panic("synth: FinishSpanFunc called more than once")
			}
			return
		}
		if code := statusFor(policy, outcome); code == codes.Error {
			span.SetStatus(code, "")
		}
		span.SetAttributes(finishAttrs(policy, outcome)...)
		if outcome.Cascaded {
			span.SetAttributes(attribute.Bool("synth.cascaded", true))
		}
		span.End(trace.WithTimestamp(outcome.EndTime))
		s.maybeIncActive(ctx, in, policy, -1)
	}
}

func (s *defaultSynthesizer) RecordMetric(ctx context.Context, in MetricInput) {
	validateMetricInput(in)

	dir := inferDirection(in.Service, in.Edge)
	protocol := protocolFor(in.Edge)
	policy := policyFor(in.Service.Kind, protocol, dir)
	histogram := s.histogramFor(policy)
	if histogram == nil {
		return
	}
	staticAttrs := s.staticAttrs(in.Service, in.Operation, in.Edge, policy)
	dynamicAttrs := dynamicOutcomeAttrs(policy, in.Outcome)
	histogram.Record(ctx, in.Latency.Seconds(),
		metric.WithAttributeSet(staticAttrs),
		metric.WithAttributes(dynamicAttrs...),
	)
}

func (s *defaultSynthesizer) EmitLog(ctx context.Context, in LogInput) {
	if in.Service == nil {
		panic("synth: EmitLog: Service must not be nil")
	}

	severity := in.Severity
	if severity == log.SeverityUndefined {
		severity = log.SeverityInfo
	}
	body := in.Body
	if body == "" {
		body = string(in.Service.Name) + " event"
	}

	record := log.Record{}
	now := time.Now()
	record.SetTimestamp(now)
	record.SetObservedTimestamp(now)
	record.SetSeverity(severity)
	record.SetBody(log.StringValue(body))
	for key, value := range in.Attributes {
		record.AddAttributes(log.KeyValue{Key: key, Value: toLogValue(value)})
	}
	record.AddAttributes(log.KeyValue{
		Key:   string(semconv.ServiceNameKey),
		Value: log.StringValue(string(in.Service.Name)),
	})

	s.logger.Emit(ctx, record)
}

func mustHistogram(meter metric.Meter, name, unit string) metric.Float64Histogram {
	histogram, err := meter.Float64Histogram(name, metric.WithUnit(unit))
	if err != nil {
		panic(fmt.Sprintf("synth: NewDefault: build %s: %v", name, err))
	}
	return histogram
}

func mustUDC(meter metric.Meter, name, unit string) metric.Int64UpDownCounter {
	counter, err := meter.Int64UpDownCounter(name, metric.WithUnit(unit))
	if err != nil {
		panic(fmt.Sprintf("synth: NewDefault: build %s: %v", name, err))
	}
	return counter
}

func validateSpanInput(in SpanInput) {
	if in.Service == nil {
		panic("synth: BeginSpan: Service must not be nil")
	}
	if in.Operation == "" {
		panic("synth: BeginSpan: Operation must not be empty")
	}
	if in.InstanceIdx < 0 || in.InstanceIdx >= in.Service.Replicas {
		panic(fmt.Sprintf("synth: BeginSpan: InstanceIdx %d out of range [0, %d)", in.InstanceIdx, in.Service.Replicas))
	}
}

func validateMetricInput(in MetricInput) {
	if in.Service == nil {
		panic("synth: RecordMetric: Service must not be nil")
	}
	if in.Operation == "" {
		panic("synth: RecordMetric: Operation must not be empty")
	}
	if in.InstanceIdx < 0 || in.InstanceIdx >= in.Service.Replicas {
		panic(fmt.Sprintf("synth: RecordMetric: InstanceIdx %d out of range [0, %d)", in.InstanceIdx, in.Service.Replicas))
	}
}

func (s *defaultSynthesizer) staticAttrs(svc *topology.Service, op string, edge *topology.Edge, policy attributePolicy) attribute.Set {
	key := cacheKeyFor(svc, op, edge, policy.Direction)
	if set, ok := s.staticSetCache.get(key); ok {
		return set
	}
	set := buildStaticSet(svc, op, edge, policy)
	s.staticSetCache.put(key, set)
	return set
}

func inferDirection(svc *topology.Service, edge *topology.Edge) direction {
	if edge == nil {
		return dirServer
	}
	if edge.From != nil && edge.From.Service == svc {
		if edge.Protocol == topology.ProtocolMessaging {
			return dirProducer
		}
		return dirClient
	}
	if edge.To != nil && edge.To.Service == svc {
		if edge.Protocol == topology.ProtocolMessaging {
			return dirConsumer
		}
		return dirServer
	}
	if svc.Kind == topology.KindQueue {
		return dirProducer
	}
	return dirInternal
}

func protocolFor(edge *topology.Edge) topology.Protocol {
	if edge == nil {
		return topology.ProtocolHTTP
	}
	return edge.Protocol
}

func (s *defaultSynthesizer) maybeIncActive(ctx context.Context, in SpanInput, policy attributePolicy, delta int64) {
	udc := s.activeUDC(policy)
	if udc == nil {
		return
	}
	if policy.SpanKind != trace.SpanKindServer {
		return
	}
	staticAttrs := s.staticAttrs(in.Service, in.Operation, in.Edge, policy)
	udc.Add(ctx, delta, metric.WithAttributeSet(staticAttrs))
}

func (s *defaultSynthesizer) activeUDC(policy attributePolicy) metric.Int64UpDownCounter {
	switch policy.MetricNamespace {
	case "http":
		return s.httpActiveReq
	case "rpc":
		return s.rpcActiveReq
	default:
		return nil
	}
}

func (s *defaultSynthesizer) histogramFor(policy attributePolicy) metric.Float64Histogram {
	switch policy.MetricNamespace {
	case "http":
		if policy.Direction == dirClient {
			return s.httpClientDur
		}
		if policy.Direction == dirServer {
			return s.httpServerDur
		}
	case "rpc":
		if policy.Direction == dirClient {
			return s.rpcClientDur
		}
		if policy.Direction == dirServer {
			return s.rpcServerDur
		}
	case "db":
		return s.dbClientDur
	case "messaging":
		if policy.Direction == dirProducer {
			return s.msgProducerDur
		}
		if policy.Direction == dirConsumer {
			return s.msgConsumerDur
		}
	}
	return nil
}

func statusFor(policy attributePolicy, outcome Outcome) codes.Code {
	switch policy.AttributeNamespace {
	case "http":
		if outcome.StatusCode >= 500 && outcome.StatusCode <= 599 {
			return codes.Error
		}
		if outcome.StatusCode >= 400 && outcome.StatusCode <= 499 {
			return codes.Unset
		}
	case "rpc":
		if outcome.StatusCode != 0 {
			return codes.Error
		}
	}
	if !outcome.Success {
		return codes.Error
	}
	return codes.Unset
}

func finishAttrs(policy attributePolicy, outcome Outcome) []attribute.KeyValue {
	return dynamicOutcomeAttrs(policy, outcome)
}

func spanName(svc *topology.Service, op string) string {
	return string(svc.Name) + "." + op
}

func toLogValue(value any) log.Value {
	switch v := value.(type) {
	case nil:
		return log.StringValue("")
	case log.Value:
		return v
	case string:
		return log.StringValue(v)
	case bool:
		return log.BoolValue(v)
	case int:
		return log.IntValue(v)
	case int8:
		return log.Int64Value(int64(v))
	case int16:
		return log.Int64Value(int64(v))
	case int32:
		return log.Int64Value(int64(v))
	case int64:
		return log.Int64Value(v)
	case uint:
		return log.Int64Value(int64(v))
	case uint8:
		return log.Int64Value(int64(v))
	case uint16:
		return log.Int64Value(int64(v))
	case uint32:
		return log.Int64Value(int64(v))
	case uint64:
		if v <= uint64(^uint(0)>>1) {
			return log.Int64Value(int64(v))
		}
		return log.StringValue(fmt.Sprint(v))
	case float32:
		return log.Float64Value(float64(v))
	case float64:
		return log.Float64Value(v)
	case []byte:
		return log.BytesValue(v)
	default:
		return log.StringValue(fmt.Sprint(v))
	}
}
