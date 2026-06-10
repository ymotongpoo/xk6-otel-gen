package journey

import (
	"fmt"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

// Plan is an immutable, precomputed tree of operation invocations for a
// single journey. Construct Plans with Engine.BuildPlan.
type Plan struct {
	JourneyName string
	Root        *Node
}

// Node is one operation invocation in a Plan tree. A Node has either Children
// for sequential execution or Parallel for fan-out execution. Virtual nodes use
// a nil Service and an empty Operation.
type Node struct {
	Service   *topology.Service
	Operation string
	Edge      *topology.Edge
	Parallel  []*Node
	Children  []*Node
}

// BuildPlan returns the cached immutable Plan for journeyName.
func (e *Engine) BuildPlan(journeyName string) (*Plan, error) {
	plan, ok := e.impl.plans[journeyName]
	if !ok {
		return nil, &PlanError{Kind: "unknown_journey", Path: []string{journeyName}}
	}
	return plan, nil
}

func (e *engineImpl) buildPlan(name string) (*Plan, error) {
	j, ok := e.schema.Journeys[name]
	if !ok {
		return nil, &PlanError{Kind: "unknown_journey", Path: []string{name}}
	}
	if j == nil || len(j.Steps) == 0 {
		return nil, &PlanError{Kind: "empty_journey", Path: []string{name}}
	}

	children := make([]*Node, 0, len(j.Steps))
	for i, step := range j.Steps {
		child, err := e.buildStepNode(step, nil, map[*topology.Operation]bool{}, []string{name, fmt.Sprintf("step[%d]", i)})
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	if len(children) == 1 {
		return &Plan{JourneyName: name, Root: children[0]}, nil
	}
	return &Plan{JourneyName: name, Root: &Node{Children: children}}, nil
}

func (e *engineImpl) buildStepNode(step *topology.Step, edge *topology.Edge, visiting map[*topology.Operation]bool, path []string) (*Node, error) {
	if step == nil {
		return nil, &PlanError{Kind: "nil_step", Path: path}
	}
	if len(step.Parallel) > 0 {
		group := &Node{Parallel: make([]*Node, 0, len(step.Parallel))}
		for i, childStep := range step.Parallel {
			child, err := e.buildStepNode(childStep, edge, cloneVisitSet(visiting), appendPath(path, fmt.Sprintf("parallel[%d]", i)))
			if err != nil {
				return nil, err
			}
			group.Parallel = append(group.Parallel, child)
		}
		return group, nil
	}
	if step.Op == nil {
		return nil, &PlanError{Kind: "nil_operation", Path: path}
	}
	return e.buildOperationNode(step.Op, edge, visiting, appendPath(path, opLabel(step.Op)))
}

func (e *engineImpl) buildOperationNode(op *topology.Operation, edge *topology.Edge, visiting map[*topology.Operation]bool, path []string) (*Node, error) {
	if op == nil || op.Service == nil {
		return nil, &PlanError{Kind: "nil_operation", Path: path}
	}
	if visiting[op] {
		return nil, &PlanError{Kind: "cycle", Path: path, Inner: fmt.Errorf("operation %s is already on the build path", opLabel(op))}
	}
	visiting[op] = true
	defer delete(visiting, op)

	node := &Node{
		Service:   op.Service,
		Operation: op.Name,
		Edge:      edge,
	}
	for i, call := range op.Calls {
		child, err := e.buildCallNode(call, visiting, appendPath(path, fmt.Sprintf("call[%d]", i)))
		if err != nil {
			return nil, err
		}
		node.Children = append(node.Children, child)
	}
	return node, nil
}

func (e *engineImpl) buildCallNode(call *topology.CallNode, visiting map[*topology.Operation]bool, path []string) (*Node, error) {
	if call == nil {
		return nil, &PlanError{Kind: "nil_call", Path: path}
	}
	if len(call.Parallel) > 0 {
		group := &Node{Parallel: make([]*Node, 0, len(call.Parallel))}
		for i, childCall := range call.Parallel {
			child, err := e.buildCallNode(childCall, cloneVisitSet(visiting), appendPath(path, fmt.Sprintf("parallel[%d]", i)))
			if err != nil {
				return nil, err
			}
			group.Parallel = append(group.Parallel, child)
		}
		return group, nil
	}
	if call.Edge == nil {
		return nil, &PlanError{Kind: "nil_edge", Path: path}
	}
	if call.Edge.To == nil {
		return nil, &PlanError{Kind: "nil_edge_to", Path: path}
	}
	return e.buildOperationNode(call.Edge.To, call.Edge, visiting, appendPath(path, opLabel(call.Edge.To)))
}

func cloneVisitSet(in map[*topology.Operation]bool) map[*topology.Operation]bool {
	out := make(map[*topology.Operation]bool, len(in))
	for op, visiting := range in {
		out[op] = visiting
	}
	return out
}

func appendPath(path []string, elem string) []string {
	out := make([]string, 0, len(path)+1)
	out = append(out, path...)
	out = append(out, elem)
	return out
}

func opLabel(op *topology.Operation) string {
	if op == nil {
		return "<nil>"
	}
	if op.Service == nil {
		return "." + op.Name
	}
	return string(op.Service.Name) + "." + op.Name
}
