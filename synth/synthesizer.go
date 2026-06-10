package synth

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
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
	panic("synth: BeginSpan: not implemented")
}

func (s *defaultSynthesizer) RecordMetric(ctx context.Context, in MetricInput) {
	panic("synth: RecordMetric: not implemented")
}

func (s *defaultSynthesizer) EmitLog(ctx context.Context, in LogInput) {
	panic("synth: EmitLog: not implemented")
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
