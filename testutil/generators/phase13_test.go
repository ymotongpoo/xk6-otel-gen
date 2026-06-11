// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"fmt"
	"math"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func TestValidErrorRate_InRange(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		rate := ValidErrorRate().Draw(t, "error_rate")
		if rate < 0 || rate > 1 {
			t.Fatalf("error rate %v outside [0,1]", rate)
		}
	})
}

func TestAnyErrorRate_CoversInvalidStatistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		invalid := false
		for i := 0; i < 100; i++ {
			rate := AnyErrorRate().Draw(t, fmt.Sprintf("rate_%d", i))
			if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 || rate > 1 {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyErrorRate produced no invalid values in 100 draws")
		}
	})
}

func TestValidOperation_BackPointer(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		svc := ValidService().Draw(t, "service")
		op := ValidOperation(svc).Draw(t, "operation")
		if op.Service != svc {
			t.Fatalf("operation service = %p, want %p", op.Service, svc)
		}
		if svc.Operations[op.Name] != op {
			t.Fatalf("operation %q not registered in service operations map", op.Name)
		}
	})
}

func TestAnyOperation_ProducesInvalidStatistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		svc := ValidService().Draw(t, "service")
		invalid := false
		for i := 0; i < 100; i++ {
			op := AnyOperation(svc).Draw(t, fmt.Sprintf("op_%d", i))
			if op.Service != svc || op.Name == "" || violatesCallNodeVariant(op.Calls) {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyOperation produced no invalid values in 100 draws")
		}
	})
}

func TestValidEdge_EndpointsAndRanges(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		svc := ValidService(MaxOpsPerService(2)).Draw(t, "service")
		ops := operationTargetsExcept(svc, nil)
		if len(ops) < 2 {
			t.Skip("need at least two operations")
		}
		from, to := ops[0], ops[1]
		edge := ValidEdge(from, to).Draw(t, "edge")
		if edge.From != from || edge.To != to {
			t.Fatal("edge endpoints mismatch")
		}
		if edge.ErrorRate < 0 || edge.ErrorRate > 1 {
			t.Fatalf("error rate %v outside [0,1]", edge.ErrorRate)
		}
		if edge.Latency.P95 < edge.Latency.P50 {
			t.Fatalf("p95 %s < p50 %s", edge.Latency.P95, edge.Latency.P50)
		}
	})
}

func TestAnyEdge_ProducesInvalidStatistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		svc := ValidService(MaxOpsPerService(2)).Draw(t, "service")
		ops := operationTargetsExcept(svc, nil)
		if len(ops) < 2 {
			t.Skip("need at least two operations")
		}
		invalid := false
		for i := 0; i < 100; i++ {
			edge := AnyEdge(ops[0], ops[1]).Draw(t, fmt.Sprintf("edge_%d", i))
			if edge.From == nil || edge.To == nil || edge.ErrorRate < 0 || edge.ErrorRate > 1 || edge.Latency.P95 < edge.Latency.P50 {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyEdge produced no invalid values in 100 draws")
		}
	})
}

func TestValidCallNode_Variant(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		svc := ValidService(MaxOpsPerService(2)).Draw(t, "service")
		ops := operationTargetsExcept(svc, nil)
		if len(ops) < 2 {
			t.Skip("need at least two operations")
		}
		node := ValidCallNode(ops[0], ops[1]).Draw(t, "call_node")
		if variantProblems(node) != "" {
			t.Fatalf("invalid call node variant: %s", variantProblems(node))
		}
	})
}

func TestAnyCallNode_ProducesInvalidStatistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		svc := ValidService(MaxOpsPerService(2)).Draw(t, "service")
		ops := operationTargetsExcept(svc, nil)
		if len(ops) < 2 {
			t.Skip("need at least two operations")
		}
		invalid := false
		for i := 0; i < 100; i++ {
			node := AnyCallNode(ops[0], ops[1]).Draw(t, fmt.Sprintf("node_%d", i))
			if variantProblems(node) != "" {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyCallNode produced no invalid values in 100 draws")
		}
	})
}

func TestValidRecoveryPolicy_Ownership(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		svc := ValidService(MaxOpsPerService(3)).Draw(t, "service")
		ops := operationTargetsExcept(svc, nil)
		if len(ops) < 2 {
			t.Skip("need at least two operations")
		}
		from, to := ops[0], ops[1]
		policy := ValidRecoveryPolicy(from, []*topology.Operation{to}).Draw(t, "recovery")
		if len(policy.Fallback) == 0 {
			t.Fatal("expected at least one fallback edge")
		}
		for _, fallback := range policy.Fallback {
			if fallback.From != from {
				t.Fatal("fallback edge has wrong From operation")
			}
		}
	})
}

func TestAnyRecoveryPolicy_ProducesInvalidStatistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		svc := ValidService(MaxOpsPerService(3)).Draw(t, "service")
		ops := operationTargetsExcept(svc, nil)
		if len(ops) < 2 {
			t.Skip("need at least two operations")
		}
		invalid := false
		for i := 0; i < 100; i++ {
			policy := AnyRecoveryPolicy(ops[0], []*topology.Operation{ops[1]}).Draw(t, fmt.Sprintf("policy_%d", i))
			if len(policy.Fallback) == 0 {
				invalid = true
				break
			}
			for _, fallback := range policy.Fallback {
				if fallback.From != ops[0] {
					invalid = true
					break
				}
			}
		}
		if !invalid {
			t.Fatal("AnyRecoveryPolicy produced no invalid values in 100 draws")
		}
	})
}

func TestValidJourney_NonEmptySteps(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema().Draw(t, "schema")
		journey := ValidJourney(schema).Draw(t, "journey")
		if journey.Name == "" || len(journey.Steps) == 0 || journey.Weight <= 0 {
			t.Fatalf("invalid journey: name=%q steps=%d weight=%v", journey.Name, len(journey.Steps), journey.Weight)
		}
	})
}

func TestAnyJourney_ProducesInvalidStatistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema().Draw(t, "schema")
		invalid := false
		for i := 0; i < 100; i++ {
			journey := AnyJourney(schema).Draw(t, fmt.Sprintf("journey_%d", i))
			if journey.Name == "" || len(journey.Steps) == 0 || journey.Weight <= 0 {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyJourney produced no invalid values in 100 draws")
		}
	})
}

func TestValidStep_Variant(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema().Draw(t, "schema")
		step := ValidStep(schema).Draw(t, "step")
		if stepVariantProblems(step) != "" {
			t.Fatalf("invalid step variant: %s", stepVariantProblems(step))
		}
	})
}

func TestAnyStep_ProducesInvalidStatistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema().Draw(t, "schema")
		invalid := false
		for i := 0; i < 100; i++ {
			step := AnyStep(schema).Draw(t, fmt.Sprintf("step_%d", i))
			if stepVariantProblems(step) != "" {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyStep produced no invalid values in 100 draws")
		}
	})
}

func TestValidFaultTarget_Resolvable(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema(MaxFaults(2)).Draw(t, "schema")
		target := ValidFaultTarget(schema).Draw(t, "target")
		if faultTargetProblems(schema, target) != "" {
			t.Fatalf("invalid fault target: %s", faultTargetProblems(schema, target))
		}
	})
}

func TestAnyFaultTarget_ProducesInvalidStatistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema(MaxFaults(2)).Draw(t, "schema")
		invalid := false
		for i := 0; i < 100; i++ {
			target := AnyFaultTarget(schema).Draw(t, fmt.Sprintf("target_%d", i))
			if faultTargetProblems(schema, target) != "" {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyFaultTarget produced no invalid values in 100 draws")
		}
	})
}

func TestValidFaultSpec_TargetAndSeverity(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema(MaxFaults(2)).Draw(t, "schema")
		spec := ValidFaultSpec(schema).Draw(t, "fault")
		if faultTargetProblems(schema, spec.Target) != "" {
			t.Fatalf("invalid fault target: %s", faultTargetProblems(schema, spec.Target))
		}
		if spec.Severity.Probability < 0 || spec.Severity.Probability > 1 {
			t.Fatalf("probability %v outside [0,1]", spec.Severity.Probability)
		}
	})
}

func TestAnyFaultSpec_ProducesInvalidStatistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema(MaxFaults(2)).Draw(t, "schema")
		invalid := false
		for i := 0; i < 100; i++ {
			spec := AnyFaultSpec(schema).Draw(t, fmt.Sprintf("fault_%d", i))
			if spec.Severity.Probability < 0 || spec.Severity.Probability > 1 || faultTargetProblems(schema, spec.Target) != "" {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyFaultSpec produced no invalid values in 100 draws")
		}
	})
}

func TestValidFaultOverlay_FromApplyFaults(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema(MaxFaults(3)).Draw(t, "schema")
		overlay := ValidFaultOverlay(schema).Draw(t, "overlay")
		if overlay == nil {
			t.Fatal("overlay is nil")
		}
		expected := schema.ApplyFaults()
		if !topology.FaultOverlayEqual(overlay, expected) {
			t.Fatal("ValidFaultOverlay mismatch with schema.ApplyFaults()")
		}
	})
}

func TestAnyFaultOverlay_ProducesInvalidStatistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema(MaxFaults(3)).Draw(t, "schema")
		expected := schema.ApplyFaults()
		different := false
		for i := 0; i < 100; i++ {
			overlay := AnyFaultOverlay(schema).Draw(t, fmt.Sprintf("overlay_%d", i))
			if overlay == nil {
				different = true
				break
			}
			if !topology.FaultOverlayEqual(overlay, expected) {
				different = true
				break
			}
		}
		if !different {
			t.Fatal("AnyFaultOverlay produced no differing overlays in 100 draws")
		}
	})
}

func violatesCallNodeVariant(nodes []*topology.CallNode) bool {
	for _, node := range nodes {
		if variantProblems(node) != "" {
			return true
		}
	}
	return false
}

func variantProblems(node *topology.CallNode) string {
	if node == nil {
		return "nil call node"
	}
	hasEdge := node.Edge != nil
	hasParallel := len(node.Parallel) > 0
	if hasEdge == hasParallel {
		return "call node violates Edge/Parallel variant"
	}
	if hasParallel {
		for _, child := range node.Parallel {
			if variantProblems(child) != "" {
				return variantProblems(child)
			}
		}
	}
	return ""
}

func stepVariantProblems(step *topology.Step) string {
	if step == nil {
		return "nil step"
	}
	hasOp := step.Op != nil
	hasParallel := len(step.Parallel) > 0
	if hasOp == hasParallel {
		return "step violates Op/Parallel variant"
	}
	if hasParallel {
		for _, child := range step.Parallel {
			if stepVariantProblems(child) != "" {
				return stepVariantProblems(child)
			}
		}
	}
	return ""
}

func faultTargetProblems(schema *topology.Schema, target topology.FaultTarget) string {
	ops := allOperations(schema)
	opSet := make(map[*topology.Operation]struct{}, len(ops))
	for _, op := range ops {
		opSet[op] = struct{}{}
	}
	edges := collectEdgesFromOperations(ops)
	edgeSet := make(map[*topology.Edge]struct{}, len(edges))
	for _, edge := range edges {
		edgeSet[edge] = struct{}{}
	}

	switch target.Kind {
	case topology.TargetNode:
		if target.Service == nil || !serviceInSchema(schema, target.Service) {
			return "fault targets unresolved service"
		}
	case topology.TargetOperation:
		if _, ok := opSet[target.Operation]; !ok {
			return "fault targets unresolved operation"
		}
	case topology.TargetEdge:
		if _, ok := edgeSet[target.Edge]; !ok {
			return "fault targets unresolved edge"
		}
	default:
		return "fault has unknown target kind"
	}
	return ""
}
