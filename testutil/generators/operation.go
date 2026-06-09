package generators

import (
	"fmt"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

// OperationOption mutates operation-generation parameters.
type OperationOption func(*operationOptions)

type operationOptions struct {
	maxCalls  int
	fixedName string
}

func defaultOperationOptions() operationOptions {
	return operationOptions{
		maxCalls: 3,
	}
}

func applyOperationOptions(opts []OperationOption) operationOptions {
	o := defaultOperationOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// MaxCalls caps the number of outgoing call nodes on a generated operation.
func MaxCalls(n int) OperationOption {
	return func(o *operationOptions) {
		o.maxCalls = clampInt(n, 0, n)
	}
}

// WithName fixes the generated operation name.
func WithName(name string) OperationOption {
	return func(o *operationOptions) {
		o.fixedName = name
	}
}

// ValidOperation returns a standalone operation with a valid service back-pointer.
func ValidOperation(svc *topology.Service, opts ...OperationOption) *rapid.Generator[*topology.Operation] {
	o := applyOperationOptions(opts)
	return rapid.Custom(func(t *rapid.T) *topology.Operation {
		name := o.fixedName
		if name == "" {
			name = ValidOperationName().Draw(t, "name")
		}
		op := &topology.Operation{
			Name:    name,
			Service: svc,
		}
		if svc != nil {
			if svc.Operations == nil {
				svc.Operations = make(map[string]*topology.Operation)
			}
			svc.Operations[name] = op
		}

		candidates := operationTargetsExcept(svc, op)
		if len(candidates) == 0 || o.maxCalls == 0 {
			return op
		}
		callCount := rapid.IntRange(0, min(o.maxCalls, len(candidates))).Draw(t, "n_calls")
		if callCount == 0 {
			return op
		}
		targets := rapid.SliceOfNDistinct(
			rapid.SampledFrom(candidates),
			callCount,
			callCount,
			func(target *topology.Operation) *topology.Operation { return target },
		).Draw(t, "targets")

		nodes := make([]*topology.CallNode, 0, len(targets))
		for i, target := range targets {
			nodes = append(nodes, ValidCallNode(op, target).Draw(t, fmt.Sprintf("call_%d", i)))
		}
		if len(nodes) > 1 && rapid.Float64Range(0, 1).Draw(t, "parallel_roll") < 0.2 {
			op.Calls = append(op.Calls, &topology.CallNode{Parallel: nodes})
			return op
		}
		op.Calls = append(op.Calls, nodes...)
		return op
	})
}

// AnyOperation returns an operation that may violate back-pointer or naming invariants.
func AnyOperation(svc *topology.Service, opts ...OperationOption) *rapid.Generator[*topology.Operation] {
	return rapid.Custom(func(t *rapid.T) *topology.Operation {
		op := ValidOperation(svc, opts...).Draw(t, "valid_operation")
		switch rapid.IntRange(0, 4).Draw(t, "operation_mutation") {
		case 0:
			op.Name = AnyOperationName().Draw(t, "any_name")
		case 1:
			op.Service = nil
		case 2:
			if len(op.Calls) > 0 {
				op.Calls[0].Edge = nil
				op.Calls[0].Parallel = nil
			} else {
				op.Calls = []*topology.CallNode{{Edge: &topology.Edge{From: op, To: op}}}
			}
		case 3:
			op.Service = &topology.Service{Name: "stale-owner"}
		case 4:
			if len(op.Calls) > 0 {
				node := op.Calls[0]
				if node.Edge != nil {
					node.Parallel = []*topology.CallNode{{Edge: node.Edge}}
				}
			}
		}
		return op
	})
}

func operationTargetsExcept(svc *topology.Service, self *topology.Operation) []*topology.Operation {
	if svc == nil || len(svc.Operations) == 0 {
		return nil
	}
	targets := make([]*topology.Operation, 0, len(svc.Operations))
	for _, op := range svc.Operations {
		if op != self {
			targets = append(targets, op)
		}
	}
	return targets
}
