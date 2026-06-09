package generators

import (
	"fmt"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

// CallNodeOption mutates call-node generation parameters.
type CallNodeOption func(*callNodeOptions)

type callNodeOptions struct {
	preferParallel bool
}

func applyCallNodeOptions(opts []CallNodeOption) callNodeOptions {
	o := callNodeOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// PreferParallel biases ValidCallNode toward generating a parallel group.
func PreferParallel() CallNodeOption {
	return func(o *callNodeOptions) {
		o.preferParallel = true
	}
}

// ValidCallNode returns a call node with exactly one of Edge or Parallel set.
func ValidCallNode(from, target *topology.Operation, opts ...CallNodeOption) *rapid.Generator[*topology.CallNode] {
	o := applyCallNodeOptions(opts)
	return rapid.Custom(func(t *rapid.T) *topology.CallNode {
		useParallel := o.preferParallel || rapid.Float64Range(0, 1).Draw(t, "parallel_roll") < 0.15
		if useParallel {
			childCount := rapid.IntRange(2, 3).Draw(t, "parallel_children")
			children := make([]*topology.CallNode, 0, childCount)
			for i := 0; i < childCount; i++ {
				children = append(children, &topology.CallNode{
					Edge: ValidEdge(from, target).Draw(t, fmt.Sprintf("parallel_edge_%d", i)),
				})
			}
			return &topology.CallNode{Parallel: children}
		}
		return &topology.CallNode{
			Edge: ValidEdge(from, target).Draw(t, "edge"),
		}
	})
}

// AnyCallNode returns a call node that may violate the Edge/Parallel variant rule.
func AnyCallNode(from, target *topology.Operation, opts ...CallNodeOption) *rapid.Generator[*topology.CallNode] {
	return rapid.Custom(func(t *rapid.T) *topology.CallNode {
		node := ValidCallNode(from, target, opts...).Draw(t, "valid_call_node")
		switch rapid.IntRange(0, 3).Draw(t, "call_node_mutation") {
		case 0:
			node.Edge = nil
			node.Parallel = nil
		case 1:
			if node.Edge != nil {
				node.Parallel = []*topology.CallNode{{Edge: node.Edge}}
			} else if len(node.Parallel) > 0 {
				node.Edge = node.Parallel[0].Edge
			}
		case 2:
			node.Parallel = []*topology.CallNode{}
		case 3:
			node.Edge = AnyEdge(from, target).Draw(t, "any_edge")
		}
		return node
	})
}
