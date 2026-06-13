// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

// ProviderFactory supplies per-virtual-service trace and log providers that
// share one OTLP connection. Each synthetic service gets its own provider whose
// resource carries that service's service.name, so the service shows up under
// its own name in Tempo/Grafana rather than OTLPResourceNoServiceName.
// Implemented by exporter.Pipeline; the key uniquely identifies a service
// instance and providers are cached by it.
type ProviderFactory interface {
	TracerProviderForService(key string, res *sdkresource.Resource) trace.TracerProvider
	LoggerProviderForService(key string, res *sdkresource.Resource) log.LoggerProvider
}

// Synthesizer is the interface used by the journey engine to emit synthetic
// OpenTelemetry spans, metrics, and logs with Semantic Conventions v1.27.0
// attributes.
type Synthesizer interface {
	// BeginSpan starts a span for a journey node and returns the child context
	// plus a finish function to call once the operation outcome is known.
	BeginSpan(ctx context.Context, in SpanInput) (context.Context, FinishSpanFunc)

	// RecordMetric records request latency and outcome metrics for one journey
	// operation using Semantic Conventions v1.27.0 attributes.
	RecordMetric(ctx context.Context, in MetricInput)

	// EmitLog emits one structured log record tied to the current journey span
	// context.
	EmitLog(ctx context.Context, in LogInput)
}

// SpanInput describes a journey node span to synthesize from topology and
// journey-engine state.
type SpanInput struct {
	Service     *topology.Service
	Edge        *topology.Edge
	Operation   string
	StartTime   time.Time
	InstanceIdx int
}

// MetricInput describes one synthetic metric data point to record after a
// journey operation completes.
type MetricInput struct {
	Service     *topology.Service
	Edge        *topology.Edge
	Operation   string
	Latency     time.Duration
	Outcome     Outcome
	InstanceIdx int
}

// LogInput describes one synthetic log record to emit for a journey operation.
type LogInput struct {
	Service  *topology.Service
	Severity log.Severity
	Body     string
	// InstanceIdx selects which replica of Service emitted the record; it
	// determines the per-instance logger (and thus the service.instance.id
	// resource attribute) so logs carry the same service identity as the span.
	InstanceIdx int
	// Timestamp is the simulated event time for the record. It must align with
	// the corresponding span's timeline (typically the span's end time) so that
	// Grafana's time-windowed trace->logs correlation finds the record. When
	// zero, EmitLog falls back to time.Now().
	Timestamp  time.Time
	Attributes map[string]any
}

// Outcome describes the result of one journey operation and supplies dynamic
// Semantic Conventions v1.27.0 attributes such as status code and error.type.
type Outcome struct {
	Success    bool
	StatusCode int
	ErrorType  string
	EndTime    time.Time
	// Cascaded is set by the caller, typically the Journey Engine, to indicate
	// that this Outcome represents a child step forced to skip execution by an
	// upstream failure. The Synthesizer emits synth.cascaded=true when this is
	// true.
	Cascaded bool
}

// FinishSpanFunc finishes a span after the journey engine knows the operation
// outcome.
type FinishSpanFunc func(outcome Outcome)
