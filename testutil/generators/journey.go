package generators

import (
	"fmt"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

// JourneyOption mutates journey-generation parameters.
type JourneyOption func(*journeyOptions)

type journeyOptions struct {
	maxSteps  int
	fixedName string
}

func defaultJourneyOptions() journeyOptions {
	return journeyOptions{
		maxSteps: 3,
	}
}

func applyJourneyOptions(opts []JourneyOption) journeyOptions {
	o := defaultJourneyOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// MaxSteps caps the number of steps in a generated journey.
func MaxSteps(n int) JourneyOption {
	return func(o *journeyOptions) {
		o.maxSteps = clampInt(n, 1, n)
	}
}

// WithJourneyName fixes the generated journey name.
func WithJourneyName(name string) JourneyOption {
	return func(o *journeyOptions) {
		o.fixedName = name
	}
}

// ValidJourney returns a journey whose steps reference operations in schema.
func ValidJourney(schema *topology.Schema, opts ...JourneyOption) *rapid.Generator[*topology.Journey] {
	o := applyJourneyOptions(opts)
	return rapid.Custom(func(t *rapid.T) *topology.Journey {
		ops := allOperations(schema)
		if len(ops) == 0 {
			schema = ValidSchema(MaxServices(2), MaxOpsPerService(2)).Draw(t, "fallback_schema")
			ops = allOperations(schema)
		}

		stepCount := rapid.IntRange(1, min(o.maxSteps, len(ops))).Draw(t, "n_steps")
		steps := make([]*topology.Step, 0, stepCount)
		for i := 0; i < stepCount; i++ {
			steps = append(steps, ValidStep(schema).Draw(t, fmt.Sprintf("step_%d", i)))
		}

		name := o.fixedName
		if name == "" {
			name = fmt.Sprintf("journey-%d", rapid.IntRange(1, 10_000).Draw(t, "journey_id"))
		}

		return &topology.Journey{
			Name:   name,
			Steps:  steps,
			Weight: rapid.Float64Range(0.1, 10).Draw(t, "weight"),
		}
	})
}

// AnyJourney returns a journey that may violate reachability or weight invariants.
func AnyJourney(schema *topology.Schema, opts ...JourneyOption) *rapid.Generator[*topology.Journey] {
	return rapid.Custom(func(t *rapid.T) *topology.Journey {
		journey := ValidJourney(schema, opts...).Draw(t, "valid_journey")
		switch rapid.IntRange(0, 4).Draw(t, "journey_mutation") {
		case 0:
			journey.Steps = nil
		case 1:
			journey.Weight = rapid.Float64Range(-5, 0).Draw(t, "non_positive_weight")
		case 2:
			journey.Name = ""
		case 3:
			stale := &topology.Operation{Name: "stale-journey-op", Service: &topology.Service{Name: "stale"}}
			if len(journey.Steps) == 0 {
				journey.Steps = []*topology.Step{{Op: stale}}
			} else {
				journey.Steps[0].Op = stale
			}
		case 4:
			if len(journey.Steps) > 0 {
				journey.Steps[0].Op = nil
				journey.Steps[0].Parallel = nil
			}
		}
		return journey
	})
}

// StepOption mutates step-generation parameters.
type StepOption func(*stepOptions)

type stepOptions struct {
	preferParallel bool
}

func applyStepOptions(opts []StepOption) stepOptions {
	o := stepOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// PreferParallelStep biases ValidStep toward generating a parallel group.
func PreferParallelStep() StepOption {
	return func(o *stepOptions) {
		o.preferParallel = true
	}
}

// ValidStep returns a journey step with exactly one of Op or Parallel set.
func ValidStep(schema *topology.Schema, opts ...StepOption) *rapid.Generator[*topology.Step] {
	o := applyStepOptions(opts)
	return rapid.Custom(func(t *rapid.T) *topology.Step {
		ops := allOperations(schema)
		if len(ops) == 0 {
			schema = ValidSchema(MaxServices(2), MaxOpsPerService(2)).Draw(t, "fallback_schema")
			ops = allOperations(schema)
		}

		useParallel := o.preferParallel || rapid.Float64Range(0, 1).Draw(t, "parallel_roll") < 0.15
		if useParallel {
			childCount := rapid.IntRange(2, 3).Draw(t, "parallel_children")
			children := make([]*topology.Step, 0, childCount)
			for i := 0; i < childCount; i++ {
				children = append(children, &topology.Step{
					Op: rapid.SampledFrom(ops).Draw(t, fmt.Sprintf("parallel_op_%d", i)),
				})
			}
			return &topology.Step{Parallel: children}
		}
		return &topology.Step{
			Op: rapid.SampledFrom(ops).Draw(t, "op"),
		}
	})
}

// AnyStep returns a step that may violate the Op/Parallel variant rule.
func AnyStep(schema *topology.Schema, opts ...StepOption) *rapid.Generator[*topology.Step] {
	return rapid.Custom(func(t *rapid.T) *topology.Step {
		step := ValidStep(schema, opts...).Draw(t, "valid_step")
		switch rapid.IntRange(0, 3).Draw(t, "step_mutation") {
		case 0:
			step.Op = nil
			step.Parallel = nil
		case 1:
			if step.Op != nil {
				step.Parallel = []*topology.Step{{Op: step.Op}}
			} else if len(step.Parallel) > 0 {
				step.Op = step.Parallel[0].Op
			}
		case 2:
			step.Parallel = []*topology.Step{}
		case 3:
			step.Op = &topology.Operation{Name: "stale-step-op", Service: &topology.Service{Name: "stale"}}
		}
		return step
	})
}
