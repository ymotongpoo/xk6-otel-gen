// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func TestValidSchema_ValidatePlaceholder(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema().Draw(t, "schema")
		if err := topology.Validate(schema); err != nil {
			t.Fatalf("Validate(ValidSchema()) error = %v", err)
		}
	})
}

func TestValidSchema_StructuralInvariants(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema().Draw(t, "schema")
		if problems := schemaProblems(schema); len(problems) > 0 {
			t.Fatalf("ValidSchema structural problems: %v", problems)
		}
	})
}

func TestValidSchema_IsDAG(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		schema := ValidSchema().Draw(t, "schema")
		if !isOperationDAG(schema) {
			t.Fatal("ValidSchema produced a cyclic operation graph")
		}
	})
}

func TestAnySchema_ContainsInvalid_Statistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		invalid := false
		for i := 0; i < 100; i++ {
			schema := AnySchema().Draw(t, fmt.Sprintf("schema_%d", i))
			if problems := schemaProblems(schema); len(problems) > 0 {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnySchema produced no invalid schemas in 100 draws")
		}
	})
}

func TestValidSchema_NotDegenerate_Statistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		fingerprints := make(map[string]struct{})
		for i := 0; i < 20; i++ {
			schema := ValidSchema().Draw(t, fmt.Sprintf("schema_%d", i))
			fingerprints[schemaFingerprint(schema)] = struct{}{}
		}
		if len(fingerprints) == 1 {
			t.Fatal("ValidSchema produced byte-identical fingerprints for all draws")
		}
	})
}

func TestSchemaMutators_IntroduceInvalidExamples(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		base := ValidSchema(MaxServices(3), MaxOpsPerService(3), MaxCallsPerOp(2), MaxFaults(2)).Draw(t, "base")
		for _, mutate := range schemaMutators() {
			mutated := mutate(t, base)
			if problems := schemaProblems(mutated); len(problems) == 0 {
				t.Fatalf("mutator %T did not introduce a structural problem", mutate)
			}
			if problems := schemaProblems(base); len(problems) > 0 {
				t.Fatalf("mutator %T mutated the input schema: %v", mutate, problems)
			}
		}
	})
}

func schemaProblems(schema *topology.Schema) []string {
	if schema == nil {
		return []string{"schema is nil"}
	}
	var problems []string
	if len(schema.Services) == 0 {
		problems = append(problems, "schema has no services")
	}

	opSet := make(map[*topology.Operation]struct{})
	edgeSet := make(map[*topology.Edge]struct{})
	for id, svc := range schema.Services {
		if svc == nil {
			problems = append(problems, fmt.Sprintf("service %s is nil", id))
			continue
		}
		if svc.Name != id {
			problems = append(problems, fmt.Sprintf("service key %s mismatches name %s", id, svc.Name))
		}
		if schema.Services[svc.Name] != svc {
			problems = append(problems, fmt.Sprintf("service %s is not reachable by its name", svc.Name))
		}
		for name, op := range svc.Operations {
			if op == nil {
				problems = append(problems, fmt.Sprintf("operation %s.%s is nil", svc.Name, name))
				continue
			}
			if op.Service != svc {
				problems = append(problems, fmt.Sprintf("operation %s.%s has wrong service back-pointer", svc.Name, name))
			}
			opSet[op] = struct{}{}
		}
	}
	for op := range opSet {
		problems = append(problems, callNodeProblems(op.Calls, opSet, edgeSet)...)
	}
	for edge := range edgeSet {
		if _, ok := opSet[edge.From]; !ok {
			problems = append(problems, "edge has unresolved From operation")
		}
		if _, ok := opSet[edge.To]; !ok {
			problems = append(problems, "edge has unresolved To operation")
		}
		if edge.OnFailure != nil {
			for _, fallback := range edge.OnFailure.Fallback {
				if fallback.From != edge.From {
					problems = append(problems, "fallback edge has wrong From operation")
				}
			}
		}
	}
	for name, journey := range schema.Journeys {
		if journey == nil {
			problems = append(problems, fmt.Sprintf("journey %s is nil", name))
			continue
		}
		problems = append(problems, stepProblems(journey.Steps, opSet)...)
	}
	for _, fault := range schema.Faults {
		switch fault.Target.Kind {
		case topology.TargetNode:
			if fault.Target.Service == nil || !serviceInSchema(schema, fault.Target.Service) {
				problems = append(problems, "fault targets unresolved service")
			}
		case topology.TargetOperation:
			if _, ok := opSet[fault.Target.Operation]; !ok {
				problems = append(problems, "fault targets unresolved operation")
			}
		case topology.TargetEdge:
			if _, ok := edgeSet[fault.Target.Edge]; !ok {
				problems = append(problems, "fault targets unresolved edge")
			}
		default:
			problems = append(problems, "fault has unknown target kind")
		}
	}
	if !isOperationDAG(schema) {
		problems = append(problems, "operation graph is cyclic")
	}
	return problems
}

func callNodeProblems(nodes []*topology.CallNode, opSet map[*topology.Operation]struct{}, edgeSet map[*topology.Edge]struct{}) []string {
	var problems []string
	for _, node := range nodes {
		if node == nil {
			problems = append(problems, "call node is nil")
			continue
		}
		hasEdge := node.Edge != nil
		hasParallel := len(node.Parallel) > 0
		if hasEdge == hasParallel {
			problems = append(problems, "call node violates Edge/Parallel variant")
		}
		if hasEdge {
			edgeSet[node.Edge] = struct{}{}
			if node.Edge.OnFailure != nil {
				for _, fallback := range node.Edge.OnFailure.Fallback {
					edgeSet[fallback] = struct{}{}
				}
			}
		}
		if hasParallel {
			problems = append(problems, callNodeProblems(node.Parallel, opSet, edgeSet)...)
		}
	}
	return problems
}

func stepProblems(steps []*topology.Step, opSet map[*topology.Operation]struct{}) []string {
	var problems []string
	for _, step := range steps {
		if step == nil {
			problems = append(problems, "journey step is nil")
			continue
		}
		if step.Op != nil {
			if _, ok := opSet[step.Op]; !ok {
				problems = append(problems, "journey step targets unresolved operation")
			}
		}
		problems = append(problems, stepProblems(step.Parallel, opSet)...)
	}
	return problems
}

func serviceInSchema(schema *topology.Schema, svc *topology.Service) bool {
	for _, existing := range schema.Services {
		if existing == svc {
			return true
		}
	}
	return false
}

func isOperationDAG(schema *topology.Schema) bool {
	ops := allOperations(schema)
	visiting := make(map[*topology.Operation]bool, len(ops))
	visited := make(map[*topology.Operation]bool, len(ops))
	var visit func(*topology.Operation) bool
	visit = func(op *topology.Operation) bool {
		if visiting[op] {
			return false
		}
		if visited[op] {
			return true
		}
		visiting[op] = true
		for _, next := range operationTargets(op.Calls) {
			if !visit(next) {
				return false
			}
		}
		visiting[op] = false
		visited[op] = true
		return true
	}
	for _, op := range ops {
		if !visit(op) {
			return false
		}
	}
	return true
}

func operationTargets(nodes []*topology.CallNode) []*topology.Operation {
	targets := make([]*topology.Operation, 0)
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Edge != nil && node.Edge.To != nil {
			targets = append(targets, node.Edge.To)
			if node.Edge.OnFailure != nil {
				for _, fallback := range node.Edge.OnFailure.Fallback {
					if fallback.To != nil {
						targets = append(targets, fallback.To)
					}
				}
			}
		}
		targets = append(targets, operationTargets(node.Parallel)...)
	}
	return targets
}

func countCallEdges(nodes []*topology.CallNode) int {
	count := 0
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Edge != nil {
			count++
		}
		count += countCallEdges(node.Parallel)
	}
	return count
}

func schemaFingerprint(schema *topology.Schema) string {
	var builder strings.Builder
	serviceIDs := make([]string, 0, len(schema.Services))
	for id := range schema.Services {
		serviceIDs = append(serviceIDs, string(id))
	}
	sort.Strings(serviceIDs)
	for _, id := range serviceIDs {
		svc := schema.Services[topology.ServiceID(id)]
		builder.WriteString(id)
		builder.WriteByte(':')
		builder.WriteString(svc.Kind.String())
		opNames := make([]string, 0, len(svc.Operations))
		for name := range svc.Operations {
			opNames = append(opNames, name)
		}
		sort.Strings(opNames)
		for _, name := range opNames {
			op := svc.Operations[name]
			builder.WriteByte('/')
			builder.WriteString(name)
			builder.WriteByte('[')
			builder.WriteString(fmt.Sprint(countCallEdges(op.Calls)))
			builder.WriteByte(']')
		}
	}
	builder.WriteString(fmt.Sprintf("|journeys=%d|faults=%d", len(schema.Journeys), len(schema.Faults)))
	return builder.String()
}
