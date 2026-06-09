package generators

import (
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

// FaultOption mutates fault-spec generation parameters.
type FaultOption func(*faultOptions)

type faultOptions struct {
	fixedKind *topology.FaultKind
}

func applyFaultOptions(opts []FaultOption) faultOptions {
	o := faultOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithFaultKind fixes the generated fault kind.
func WithFaultKind(kind topology.FaultKind) FaultOption {
	return func(o *faultOptions) {
		o.fixedKind = &kind
	}
}

// FaultTargetOption mutates fault-target generation parameters.
type FaultTargetOption func(*faultTargetOptions)

type faultTargetOptions struct {
	fixedKind *topology.TargetKind
}

func applyFaultTargetOptions(opts []FaultTargetOption) faultTargetOptions {
	o := faultTargetOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithTargetKind fixes the generated fault target kind.
func WithTargetKind(kind topology.TargetKind) FaultTargetOption {
	return func(o *faultTargetOptions) {
		o.fixedKind = &kind
	}
}

// FaultOverlayOption mutates fault-overlay generation parameters.
type FaultOverlayOption func(*faultOverlayOptions)

type faultOverlayOptions struct {
	maxFaults int
}

func defaultFaultOverlayOptions() faultOverlayOptions {
	return faultOverlayOptions{
		maxFaults: 3,
	}
}

func applyFaultOverlayOptions(opts []FaultOverlayOption) faultOverlayOptions {
	o := defaultFaultOverlayOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// OverlayMaxFaults caps faults when ValidFaultOverlay synthesizes a schema.
func OverlayMaxFaults(n int) FaultOverlayOption {
	return func(o *faultOverlayOptions) {
		o.maxFaults = clampInt(n, 0, n)
	}
}

// ValidFaultTarget returns a fault target referencing entities in schema.
func ValidFaultTarget(schema *topology.Schema, opts ...FaultTargetOption) *rapid.Generator[topology.FaultTarget] {
	o := applyFaultTargetOptions(opts)
	return rapid.Custom(func(t *rapid.T) topology.FaultTarget {
		if schema == nil || len(allOperations(schema)) == 0 {
			schema = ValidSchema(MaxServices(2), MaxOpsPerService(2), MaxFaults(1)).Draw(t, "fallback_schema")
		}

		ops := allOperations(schema)
		services := uniqueServices(ops)
		edges := collectEdgesFromOperations(ops)

		kind := topology.TargetKind(rapid.IntRange(0, 2).Draw(t, "target_kind"))
		if o.fixedKind != nil {
			kind = *o.fixedKind
		}
		if len(edges) == 0 && kind == topology.TargetEdge {
			kind = topology.TargetOperation
		}

		target := topology.FaultTarget{Kind: kind}
		switch kind {
		case topology.TargetNode:
			target.Service = rapid.SampledFrom(services).Draw(t, "service")
		case topology.TargetOperation:
			target.Operation = rapid.SampledFrom(ops).Draw(t, "operation")
		default:
			target.Edge = rapid.SampledFrom(edges).Draw(t, "edge")
		}
		return target
	})
}

// AnyFaultTarget returns a fault target that may reference stale entities.
func AnyFaultTarget(schema *topology.Schema, opts ...FaultTargetOption) *rapid.Generator[topology.FaultTarget] {
	return rapid.Custom(func(t *rapid.T) topology.FaultTarget {
		target := ValidFaultTarget(schema, opts...).Draw(t, "valid_target")
		staleSvc := &topology.Service{Name: "stale"}
		staleOp := &topology.Operation{Name: "stale-op", Service: staleSvc}
		staleEdge := &topology.Edge{From: staleOp, To: staleOp, Protocol: topology.ProtocolHTTP}

		switch rapid.IntRange(0, 4).Draw(t, "target_mutation") {
		case 0:
			target.Service = staleSvc
			target.Kind = topology.TargetNode
			target.Operation = nil
			target.Edge = nil
		case 1:
			target.Operation = staleOp
			target.Kind = topology.TargetOperation
			target.Service = nil
			target.Edge = nil
		case 2:
			target.Edge = staleEdge
			target.Kind = topology.TargetEdge
			target.Service = nil
			target.Operation = nil
		case 3:
			target.Kind = topology.TargetKind(rapid.IntRange(10, 20).Draw(t, "invalid_kind"))
		case 4:
			target.Service = staleSvc
			target.Operation = staleOp
		}
		return target
	})
}

// ValidFaultSpec returns a fault specification with a valid target and severity.
func ValidFaultSpec(schema *topology.Schema, opts ...FaultOption) *rapid.Generator[topology.FaultSpec] {
	o := applyFaultOptions(opts)
	return rapid.Custom(func(t *rapid.T) topology.FaultSpec {
		kind := rapid.SampledFrom([]topology.FaultKind{
			topology.FaultLatencyInflation,
			topology.FaultErrorRateOverride,
			topology.FaultDisconnect,
			topology.FaultCrash,
		}).Draw(t, "fault_kind")
		if o.fixedKind != nil {
			kind = *o.fixedKind
		}

		multiplier := rapid.Float64Range(1, 10).Draw(t, "multiplier")
		if kind != topology.FaultLatencyInflation {
			multiplier = rapid.Float64Range(0, 1).Draw(t, "non_latency_multiplier")
		}

		return topology.FaultSpec{
			Target: ValidFaultTarget(schema).Draw(t, "target"),
			Kind:   kind,
			Severity: topology.SeverityParams{
				Probability: ValidProbability().Draw(t, "probability"),
				Multiplier:  multiplier,
				Add:         ValidTimeout().Draw(t, "add"),
				Value:       ValidProbability().Draw(t, "value"),
			},
		}
	})
}

// AnyFaultSpec returns a fault specification that may violate domain invariants.
func AnyFaultSpec(schema *topology.Schema, opts ...FaultOption) *rapid.Generator[topology.FaultSpec] {
	return rapid.Custom(func(t *rapid.T) topology.FaultSpec {
		spec := ValidFaultSpec(schema, opts...).Draw(t, "valid_fault")
		switch rapid.IntRange(0, 3).Draw(t, "fault_mutation") {
		case 0:
			spec.Severity.Probability = AnyProbability().Draw(t, "any_probability")
		case 1:
			spec.Severity.Multiplier = rapid.Float64Range(-5, 0).Draw(t, "non_positive_multiplier")
		case 2:
			spec.Target = AnyFaultTarget(schema).Draw(t, "any_target")
		case 3:
			spec.Kind = topology.FaultKind(rapid.IntRange(10, 20).Draw(t, "invalid_kind"))
		}
		return spec
	})
}

// ValidFaultOverlay returns an overlay produced by ApplyFaults on a schema with faults.
func ValidFaultOverlay(schema *topology.Schema, opts ...FaultOverlayOption) *rapid.Generator[*topology.FaultOverlay] {
	o := applyFaultOverlayOptions(opts)
	return rapid.Custom(func(t *rapid.T) *topology.FaultOverlay {
		s := schema
		if s == nil {
			s = ValidSchema(MaxFaults(o.maxFaults)).Draw(t, "schema")
		} else {
			// rapid.Custom requires at least one bitstream consumption per
			// invocation to enable shrinking. When the caller passes a fixed
			// schema we have no other draws, so insert a no-op draw.
			_ = rapid.IntRange(0, 0).Draw(t, "_pinned_schema")
		}
		return s.ApplyFaults()
	})
}

// AnyFaultOverlay returns an overlay that may be empty or derived from degraded schemas.
func AnyFaultOverlay(schema *topology.Schema, opts ...FaultOverlayOption) *rapid.Generator[*topology.FaultOverlay] {
	return rapid.Custom(func(t *rapid.T) *topology.FaultOverlay {
		switch rapid.IntRange(0, 2).Draw(t, "overlay_variant") {
		case 0:
			return ValidFaultOverlay(schema, opts...).Draw(t, "valid_overlay")
		case 1:
			return (&topology.Schema{}).ApplyFaults()
		default:
			s := ValidSchema(MaxFaults(2)).Draw(t, "degraded_schema")
			return misreferenceFault(t, s).ApplyFaults()
		}
	})
}
