// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"go.opentelemetry.io/otel/log"
	"pgregory.net/rapid"
)

// SemconvErrorTypes is the reusable sample set for synth.Outcome error.type values.
var SemconvErrorTypes = []string{
	"timeout",
	"connection_refused",
	"dns_failure",
	"http.500",
	"http.502",
	"http.503",
	"http.504",
	"grpc.unavailable",
	"grpc.deadline_exceeded",
	"grpc.unauthenticated",
	"db.connection_lost",
	"db.constraint_violation",
}

// SpanInputOption mutates ValidSpanInput and AnySpanInput generation parameters.
type SpanInputOption interface {
	applySpanInputOption(*spanInputOptions)
}

type spanInputOptions struct {
	service *topology.Service
}

func applySpanInputOptions(opts []SpanInputOption) spanInputOptions {
	o := spanInputOptions{}
	for _, opt := range opts {
		opt.applySpanInputOption(&o)
	}
	return o
}

// MetricInputOption mutates ValidMetricInput and AnyMetricInput generation parameters.
type MetricInputOption interface {
	applyMetricInputOption(*metricInputOptions)
}

type metricInputOptions struct {
	spanOpts []SpanInputOption
}

func applyMetricInputOptions(opts []MetricInputOption) metricInputOptions {
	o := metricInputOptions{}
	for _, opt := range opts {
		opt.applyMetricInputOption(&o)
	}
	return o
}

// LogInputOption mutates ValidLogInput and AnyLogInput generation parameters.
type LogInputOption interface {
	applyLogInputOption(*logInputOptions)
}

type logInputOptions struct {
	service *topology.Service
}

func applyLogInputOptions(opts []LogInputOption) logInputOptions {
	o := logInputOptions{}
	for _, opt := range opts {
		opt.applyLogInputOption(&o)
	}
	return o
}

// OutcomeOption mutates ValidOutcome and AnyOutcome generation parameters.
type OutcomeOption interface {
	applyOutcomeOption(*outcomeOptions)
}

type outcomeOptions struct{}

func applyOutcomeOptions(opts []OutcomeOption) outcomeOptions {
	o := outcomeOptions{}
	for _, opt := range opts {
		opt.applyOutcomeOption(&o)
	}
	return o
}

// ValidSpanInput returns a valid synth.SpanInput with a service, operation,
// start time, and in-range instance index.
func ValidSpanInput(opts ...SpanInputOption) *rapid.Generator[synth.SpanInput] {
	o := applySpanInputOptions(opts)
	return rapid.Custom(func(t *rapid.T) synth.SpanInput {
		svc := o.service
		if svc == nil {
			svc = ValidService().Draw(t, "service")
		}
		op := chooseOperation(t, svc, "operation")
		var edge *topology.Edge
		if rapid.Float64Range(0, 1).Draw(t, "edge_roll") >= 0.1 {
			edge = validSynthEdge(t, svc, op)
		}
		return synth.SpanInput{
			Service:     svc,
			Edge:        edge,
			Operation:   op.Name,
			StartTime:   validSynthTime(t, "start_time"),
			InstanceIdx: rapid.IntRange(0, svc.Replicas-1).Draw(t, "instance_idx"),
		}
	})
}

// AnySpanInput returns a synth.SpanInput that may violate service, operation,
// or instance-index invariants.
func AnySpanInput(opts ...SpanInputOption) *rapid.Generator[synth.SpanInput] {
	return rapid.Custom(func(t *rapid.T) synth.SpanInput {
		in := ValidSpanInput(opts...).Draw(t, "valid_span")
		switch rapid.IntRange(0, 3).Draw(t, "span_mutation") {
		case 0:
			in.Service = nil
		case 1:
			in.Operation = ""
		case 2:
			in.InstanceIdx = in.Service.Replicas
		case 3:
			in.InstanceIdx = -1
		}
		return in
	})
}

// ValidMetricInput returns a valid synth.MetricInput suitable for RecordMetric.
func ValidMetricInput(opts ...MetricInputOption) *rapid.Generator[synth.MetricInput] {
	o := applyMetricInputOptions(opts)
	return rapid.Custom(func(t *rapid.T) synth.MetricInput {
		span := ValidSpanInput(o.spanOpts...).Draw(t, "span")
		latency := ValidLatencyPair().Draw(t, "latency").P50
		return synth.MetricInput{
			Service:     span.Service,
			Edge:        span.Edge,
			Operation:   span.Operation,
			Latency:     latency,
			Outcome:     ValidOutcome().Draw(t, "outcome"),
			InstanceIdx: span.InstanceIdx,
		}
	})
}

// AnyMetricInput returns a synth.MetricInput that may violate required input
// invariants.
func AnyMetricInput(opts ...MetricInputOption) *rapid.Generator[synth.MetricInput] {
	return rapid.Custom(func(t *rapid.T) synth.MetricInput {
		in := ValidMetricInput(opts...).Draw(t, "valid_metric")
		switch rapid.IntRange(0, 3).Draw(t, "metric_mutation") {
		case 0:
			in.Service = nil
		case 1:
			in.Operation = ""
		case 2:
			in.InstanceIdx = in.Service.Replicas
		case 3:
			in.Latency = -time.Millisecond
		}
		return in
	})
}

// ValidLogInput returns a valid synth.LogInput with a non-nil service.
func ValidLogInput(opts ...LogInputOption) *rapid.Generator[synth.LogInput] {
	o := applyLogInputOptions(opts)
	return rapid.Custom(func(t *rapid.T) synth.LogInput {
		svc := o.service
		if svc == nil {
			svc = ValidService().Draw(t, "service")
		}
		return synth.LogInput{
			Service: svc,
			Severity: rapid.SampledFrom([]log.Severity{
				log.SeverityUndefined,
				log.SeverityInfo,
				log.SeverityWarn,
				log.SeverityError,
			}).Draw(t, "severity"),
			Body: rapid.SampledFrom([]string{
				"",
				string(svc.Name) + " event",
				string(svc.Name) + " handled request",
			}).Draw(t, "body"),
			Attributes: map[string]any{
				"operation": chooseOperation(t, svc, "log_operation").Name,
				"attempt":   rapid.IntRange(0, 5).Draw(t, "attempt"),
			},
		}
	})
}

// AnyLogInput returns a synth.LogInput that may have a nil service.
func AnyLogInput(opts ...LogInputOption) *rapid.Generator[synth.LogInput] {
	return rapid.Custom(func(t *rapid.T) synth.LogInput {
		in := ValidLogInput(opts...).Draw(t, "valid_log")
		switch rapid.IntRange(0, 2).Draw(t, "log_mutation") {
		case 0:
			in.Service = nil
		case 1:
			in.Attributes = map[string]any{"unsupported": struct{ A int }{A: 1}}
		case 2:
			in.Body = ""
		}
		return in
	})
}

// ValidOutcome returns a synth.Outcome with consistent success/error fields.
func ValidOutcome(opts ...OutcomeOption) *rapid.Generator[synth.Outcome] {
	_ = applyOutcomeOptions(opts)
	return rapid.Custom(func(t *rapid.T) synth.Outcome {
		success := rapid.Bool().Draw(t, "success")
		end := validSynthTime(t, "end_time")
		if success {
			return synth.Outcome{
				Success:    true,
				StatusCode: rapid.SampledFrom([]int{0, 200, 204, 404}).Draw(t, "success_status"),
				EndTime:    end,
			}
		}
		return synth.Outcome{
			Success:    false,
			StatusCode: rapid.SampledFrom([]int{0, 500, 503, 14}).Draw(t, "failure_status"),
			ErrorType:  ValidErrorType().Draw(t, "error_type"),
			EndTime:    end,
		}
	})
}

// AnyOutcome returns a synth.Outcome that may violate failure/error invariants.
func AnyOutcome(opts ...OutcomeOption) *rapid.Generator[synth.Outcome] {
	return rapid.Custom(func(t *rapid.T) synth.Outcome {
		out := ValidOutcome(opts...).Draw(t, "valid_outcome")
		switch rapid.IntRange(0, 2).Draw(t, "outcome_mutation") {
		case 0:
			out.Success = false
			out.ErrorType = ""
		case 1:
			out.StatusCode = -1
		case 2:
			out.Success = true
			out.ErrorType = ValidErrorType().Draw(t, "unexpected_error_type")
		}
		return out
	})
}

// ValidErrorType returns a semconv-compatible error.type sample.
func ValidErrorType() *rapid.Generator[string] {
	return rapid.SampledFrom(SemconvErrorTypes)
}

func validSynthEdge(t *rapid.T, svc *topology.Service, from *topology.Operation) *topology.Edge {
	target := ValidService().Draw(t, "target_service")
	to := chooseOperation(t, target, "target_operation")
	protocol := ValidProtocol().Draw(t, "edge_protocol")
	if svc.Kind == topology.KindQueue {
		protocol = topology.ProtocolMessaging
	}
	return ValidEdge(from, to, WithProtocol(protocol), WithoutRecovery()).Draw(t, "edge")
}

func chooseOperation(t *rapid.T, svc *topology.Service, label string) *topology.Operation {
	name := rapid.SampledFrom(mapKeys(svc.Operations)).Draw(t, label)
	return svc.Operations[name]
}

func validSynthTime(t *rapid.T, label string) time.Time {
	offsetMillis := rapid.IntRange(0, 3_600_000).Draw(t, label+"_offset_ms")
	return time.Unix(1_700_000_000, 0).Add(time.Duration(offsetMillis) * time.Millisecond)
}
