// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"context"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/journey"
	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"pgregory.net/rapid"
)

// PlanOption mutates journey Plan generation parameters.
type PlanOption func(*planOptions)

type planOptions struct{}

func applyPlanOptions(opts []PlanOption) planOptions {
	o := planOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// NodeOption mutates journey Node generation parameters.
type NodeOption func(*nodeOptions)

type nodeOptions struct{}

func applyNodeOptions(opts []NodeOption) nodeOptions {
	o := nodeOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// ValidPlan returns a generator producing structurally valid journey Plans.
func ValidPlan(opts ...PlanOption) *rapid.Generator[*journey.Plan] {
	return rapid.Custom(func(t *rapid.T) *journey.Plan {
		_ = applyPlanOptions(opts)
		schema := ValidSchema(MaxServices(5), MaxOpsPerService(3), MaxCallsPerOp(3), MaxFaults(0)).Draw(t, "schema")
		engine := journey.NewEngine(schema, schema.ApplyFaults(), noopSynth{})
		name := rapid.SampledFrom(engine.ListJourneys()).Draw(t, "journey_name")
		plan, err := engine.BuildPlan(name)
		if err != nil {
			t.Fatalf("BuildPlan(%q) error = %v", name, err)
		}
		return plan
	})
}

// AnyPlan returns a generator that may produce invalid journey Plans.
func AnyPlan(opts ...PlanOption) *rapid.Generator[*journey.Plan] {
	return rapid.Custom(func(t *rapid.T) *journey.Plan {
		plan := ValidPlan(opts...).Draw(t, "valid_plan")
		switch rapid.IntRange(0, 3).Draw(t, "plan_mutation") {
		case 0:
			plan.Root = nil
		case 1:
			plan.JourneyName = ""
		case 2:
			if plan.Root != nil {
				plan.Root.Parallel = append(plan.Root.Parallel, plan.Root)
			}
		case 3:
			return nil
		}
		return plan
	})
}

// ValidNode returns a generator producing structurally valid journey Nodes.
func ValidNode(opts ...NodeOption) *rapid.Generator[*journey.Node] {
	return rapid.Custom(func(t *rapid.T) *journey.Node {
		_ = applyNodeOptions(opts)
		return ValidPlan().Draw(t, "plan").Root
	})
}

// AnyNode returns a generator that may produce invalid journey Nodes.
func AnyNode(opts ...NodeOption) *rapid.Generator[*journey.Node] {
	return rapid.Custom(func(t *rapid.T) *journey.Node {
		_ = applyNodeOptions(opts)
		plan := AnyPlan().Draw(t, "plan")
		if plan == nil {
			return nil
		}
		return plan.Root
	})
}

// ValidEngineOutcome returns a generator producing journey Outcomes that
// satisfy the U2 outcome invariants.
func ValidEngineOutcome(opts ...OutcomeOption) *rapid.Generator[journey.Outcome] {
	return rapid.Custom(func(t *rapid.T) journey.Outcome {
		_ = opts
		latency := time.Duration(rapid.IntRange(0, 1000).Draw(t, "latency_ms")) * time.Millisecond
		status := rapid.SampledFrom([]int{0, 200, 500, 503}).Draw(t, "status")
		errType := rapid.SampledFrom(journey.AllowedErrorTypes).Draw(t, "error_type")
		switch rapid.IntRange(0, 4).Draw(t, "outcome_kind") {
		case 0:
			return journey.Outcome{Success: true, StatusCode: 200, Latency: latency}
		case 1:
			return journey.Outcome{Success: false, StatusCode: status, ErrorType: errType, Latency: latency}
		case 2:
			return journey.Outcome{Success: false, StatusCode: status, ErrorType: errType, Cascaded: true}
		case 3:
			return journey.Outcome{Success: true, StatusCode: 200, Latency: latency, PrimaryFailed: true, DefaultUsed: true}
		default:
			return journey.Outcome{Success: true, StatusCode: 200, Latency: latency, PrimaryFailed: true, SilentlySucceeded: true}
		}
	})
}

// AnyEngineOutcome returns a generator that may violate journey Outcome
// invariants for negative-path tests and shrinking.
func AnyEngineOutcome(opts ...OutcomeOption) *rapid.Generator[journey.Outcome] {
	return rapid.Custom(func(t *rapid.T) journey.Outcome {
		_ = opts
		return journey.Outcome{
			Success:           rapid.Bool().Draw(t, "success"),
			Latency:           time.Duration(rapid.IntRange(-1000, 5000).Draw(t, "latency_ms")) * time.Millisecond,
			StatusCode:        rapid.IntRange(0, 599).Draw(t, "status"),
			ErrorType:         rapid.StringMatching(`^[a-z_.0-9-]{0,32}$`).Draw(t, "error_type"),
			Cascaded:          rapid.Bool().Draw(t, "cascaded"),
			PrimaryFailed:     rapid.Bool().Draw(t, "primary_failed"),
			DefaultUsed:       rapid.Bool().Draw(t, "default_used"),
			SilentlySucceeded: rapid.Bool().Draw(t, "silently_succeeded"),
			FallbackAttempts:  nil,
			FallbackUsed:      nil,
		}
	})
}

type noopSynth struct{}

func (noopSynth) BeginSpan(ctx context.Context, _ synth.SpanInput) (context.Context, synth.FinishSpanFunc) {
	return ctx, func(synth.Outcome) {}
}

func (noopSynth) RecordMetric(context.Context, synth.MetricInput) {}

func (noopSynth) EmitLog(context.Context, synth.LogInput) {}
