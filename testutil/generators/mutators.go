// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

type mutator func(t *rapid.T, s *topology.Schema) *topology.Schema

func schemaMutators() []mutator {
	return []mutator{
		unresolveEdgeTarget,
		introduceCycle,
		misreferenceJourney,
		misreferenceFault,
		dropServiceMap,
		breakBackPointer,
		violateCallNodeVariant,
		misownFallback,
	}
}

// unresolveEdgeTarget violates R-STR-3 by pointing an edge to an unregistered operation.
func unresolveEdgeTarget(t *rapid.T, s *topology.Schema) *topology.Schema {
	clone := cloneSchema(s)
	ops := allOperations(clone)
	edges := collectEdgesFromOperations(ops)
	stale := &topology.Operation{Name: "stale-target", Service: &topology.Service{Name: "stale"}}
	if len(edges) == 0 {
		op := rapid.SampledFrom(ops).Draw(t, "unresolve_from")
		op.Calls = append(op.Calls, &topology.CallNode{Edge: &topology.Edge{From: op, To: stale, Protocol: topology.ProtocolHTTP}})
		op.Service = stale.Service
		return clone
	}
	rapid.SampledFrom(edges).Draw(t, "unresolve_edge").To = stale
	return clone
}

// introduceCycle violates R-STR-4 by adding a reverse or self edge.
func introduceCycle(t *rapid.T, s *topology.Schema) *topology.Schema {
	clone := cloneSchema(s)
	ops := allOperations(clone)
	edges := collectEdgesFromOperations(ops)
	if len(edges) > 0 {
		edge := rapid.SampledFrom(edges).Draw(t, "cycle_edge")
		edge.To.Calls = append(edge.To.Calls, &topology.CallNode{
			Edge: &topology.Edge{From: edge.To, To: edge.From, Protocol: edge.Protocol},
		})
		return clone
	}
	from := ops[0]
	from.Calls = append(from.Calls, &topology.CallNode{
		Edge: &topology.Edge{From: from, To: from, Protocol: topology.ProtocolHTTP},
	})
	return clone
}

// misreferenceJourney violates R-STR-5 by using an unregistered journey operation.
func misreferenceJourney(t *rapid.T, s *topology.Schema) *topology.Schema {
	clone := cloneSchema(s)
	stale := &topology.Operation{Name: "stale-journey-op", Service: &topology.Service{Name: "stale"}}
	if len(clone.Journeys) == 0 {
		clone.Journeys = map[string]*topology.Journey{
			"stale": {Name: "stale", Steps: []*topology.Step{{Op: stale}}, Weight: 1},
		}
		return clone
	}
	names := journeyNames(clone)
	journey := clone.Journeys[rapid.SampledFrom(names).Draw(t, "journey_to_misreference")]
	if len(journey.Steps) == 0 {
		journey.Steps = []*topology.Step{{Op: stale}}
		return clone
	}
	journey.Steps[rapid.IntRange(0, len(journey.Steps)-1).Draw(t, "step_to_misreference")].Op = stale
	return clone
}

// misreferenceFault violates R-STR-6 by pointing a fault target to a stale entity.
func misreferenceFault(t *rapid.T, s *topology.Schema) *topology.Schema {
	clone := cloneSchema(s)
	stale := &topology.Operation{Name: "stale-fault-op", Service: &topology.Service{Name: "stale"}}
	target := topology.FaultTarget{Kind: topology.TargetOperation, Operation: stale}
	if len(clone.Faults) == 0 {
		clone.Faults = []topology.FaultSpec{{Target: target, Kind: topology.FaultCrash}}
		return clone
	}
	clone.Faults[rapid.IntRange(0, len(clone.Faults)-1).Draw(t, "fault_to_misreference")].Target = target
	return clone
}

// dropServiceMap violates R-STR-1 by removing a referenced service from Schema.Services.
func dropServiceMap(t *rapid.T, s *topology.Schema) *topology.Schema {
	clone := cloneSchema(s)
	services := serviceIDs(clone)
	if len(services) == 0 {
		return clone
	}
	id := rapid.SampledFrom(services).Draw(t, "service_to_drop")
	dropped := clone.Services[id]
	var orphan *topology.Operation
	for _, op := range dropped.Operations {
		orphan = op
		break
	}
	delete(clone.Services, id)
	if orphan != nil {
		clone.Journeys["orphan-after-drop"] = &topology.Journey{
			Name:   "orphan-after-drop",
			Steps:  []*topology.Step{{Op: orphan}},
			Weight: 1,
		}
	}
	return clone
}

// breakBackPointer violates R-STR-2 by changing an operation's owning service pointer.
func breakBackPointer(t *rapid.T, s *topology.Schema) *topology.Schema {
	clone := cloneSchema(s)
	ops := allOperations(clone)
	services := uniqueServices(ops)
	op := rapid.SampledFrom(ops).Draw(t, "op_to_reown")
	if len(services) > 1 {
		for _, svc := range services {
			if svc != op.Service {
				op.Service = svc
				return clone
			}
		}
	}
	op.Service = &topology.Service{Name: "stale-owner"}
	return clone
}

// violateCallNodeVariant violates R-STR-7 by setting Edge and Parallel together.
func violateCallNodeVariant(t *rapid.T, s *topology.Schema) *topology.Schema {
	clone := cloneSchema(s)
	ops := allOperations(clone)
	nodes := edgeCallNodes(ops)
	if len(nodes) == 0 {
		op := rapid.SampledFrom(ops).Draw(t, "variant_op")
		node := &topology.CallNode{Edge: &topology.Edge{From: op, To: op, Protocol: topology.ProtocolHTTP}}
		op.Calls = append(op.Calls, node)
		nodes = []*topology.CallNode{node}
	}
	node := rapid.SampledFrom(nodes).Draw(t, "variant_node")
	node.Parallel = []*topology.CallNode{{Edge: node.Edge}}
	return clone
}

// misownFallback violates R-STR-8 by changing fallback edge ownership.
func misownFallback(t *rapid.T, s *topology.Schema) *topology.Schema {
	clone := cloneSchema(s)
	ops := allOperations(clone)
	edges := collectEdgesFromOperations(ops)
	if len(edges) == 0 {
		op := ops[0]
		edge := &topology.Edge{From: op, To: op, Protocol: topology.ProtocolHTTP}
		op.Calls = append(op.Calls, &topology.CallNode{Edge: edge})
		edges = []*topology.Edge{edge}
	}
	edge := rapid.SampledFrom(edges).Draw(t, "fallback_edge")
	if edge.OnFailure == nil || len(edge.OnFailure.Fallback) == 0 {
		edge.OnFailure = &topology.RecoveryPolicy{
			Fallback:    []*topology.Edge{{From: edge.From, To: edge.To, Protocol: edge.Protocol}},
			OnExhausted: topology.ExhaustedPropagate,
		}
	}
	replacement := &topology.Operation{Name: "stale-fallback-owner", Service: &topology.Service{Name: "stale"}}
	for _, op := range ops {
		if op != edge.From {
			replacement = op
			break
		}
	}
	edge.OnFailure.Fallback[0].From = replacement
	return clone
}

func cloneSchema(s *topology.Schema) *topology.Schema {
	if s == nil {
		return nil
	}
	clone := &topology.Schema{
		Services: make(map[topology.ServiceID]*topology.Service, len(s.Services)),
		Journeys: make(map[string]*topology.Journey, len(s.Journeys)),
		Faults:   make([]topology.FaultSpec, len(s.Faults)),
	}
	serviceMap := make(map[*topology.Service]*topology.Service)
	opMap := make(map[*topology.Operation]*topology.Operation)
	edgeMap := make(map[*topology.Edge]*topology.Edge)

	for id, svc := range s.Services {
		copied := &topology.Service{
			Name:       svc.Name,
			Kind:       svc.Kind,
			Replicas:   svc.Replicas,
			Language:   svc.Language,
			Framework:  svc.Framework,
			Version:    svc.Version,
			Operations: make(map[string]*topology.Operation, len(svc.Operations)),
		}
		clone.Services[id] = copied
		serviceMap[svc] = copied
	}
	for oldSvc, newSvc := range serviceMap {
		for name, op := range oldSvc.Operations {
			copied := &topology.Operation{Name: op.Name, Service: newSvc}
			newSvc.Operations[name] = copied
			opMap[op] = copied
		}
	}
	for oldOp, newOp := range opMap {
		newOp.Calls = cloneCallNodes(oldOp.Calls, opMap, edgeMap)
	}
	for name, journey := range s.Journeys {
		clone.Journeys[name] = &topology.Journey{
			Name:   journey.Name,
			Steps:  cloneSteps(journey.Steps, opMap),
			Weight: journey.Weight,
		}
	}
	for i, fault := range s.Faults {
		clone.Faults[i] = topology.FaultSpec{
			Target:   cloneFaultTarget(fault.Target, serviceMap, opMap, edgeMap),
			Kind:     fault.Kind,
			Severity: fault.Severity,
			Schedule: append([]topology.FaultSchedulePoint(nil), fault.Schedule...),
		}
	}
	return clone
}

func cloneCallNodes(nodes []*topology.CallNode, opMap map[*topology.Operation]*topology.Operation, edgeMap map[*topology.Edge]*topology.Edge) []*topology.CallNode {
	if len(nodes) == 0 {
		return nil
	}
	copied := make([]*topology.CallNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			copied = append(copied, nil)
			continue
		}
		copied = append(copied, &topology.CallNode{
			Edge:     cloneEdge(node.Edge, opMap, edgeMap),
			Parallel: cloneCallNodes(node.Parallel, opMap, edgeMap),
		})
	}
	return copied
}

func cloneEdge(edge *topology.Edge, opMap map[*topology.Operation]*topology.Operation, edgeMap map[*topology.Edge]*topology.Edge) *topology.Edge {
	if edge == nil {
		return nil
	}
	if copied, ok := edgeMap[edge]; ok {
		return copied
	}
	copied := &topology.Edge{
		From:         mapOperation(edge.From, opMap),
		To:           mapOperation(edge.To, opMap),
		Protocol:     edge.Protocol,
		Latency:      edge.Latency,
		ErrorRate:    edge.ErrorRate,
		Timeout:      edge.Timeout,
		Retries:      edge.Retries,
		RetryBackoff: edge.RetryBackoff,
	}
	edgeMap[edge] = copied
	if edge.OnFailure != nil {
		fallback := make([]*topology.Edge, 0, len(edge.OnFailure.Fallback))
		for _, fallbackEdge := range edge.OnFailure.Fallback {
			fallback = append(fallback, cloneEdge(fallbackEdge, opMap, edgeMap))
		}
		copied.OnFailure = &topology.RecoveryPolicy{
			Fallback:        fallback,
			OnExhausted:     edge.OnFailure.OnExhausted,
			DefaultResponse: cloneAnyMap(edge.OnFailure.DefaultResponse),
		}
	}
	return copied
}

func cloneSteps(steps []*topology.Step, opMap map[*topology.Operation]*topology.Operation) []*topology.Step {
	if len(steps) == 0 {
		return nil
	}
	copied := make([]*topology.Step, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			copied = append(copied, nil)
			continue
		}
		copied = append(copied, &topology.Step{
			Op:       mapOperation(step.Op, opMap),
			Parallel: cloneSteps(step.Parallel, opMap),
		})
	}
	return copied
}

func cloneFaultTarget(target topology.FaultTarget, serviceMap map[*topology.Service]*topology.Service, opMap map[*topology.Operation]*topology.Operation, edgeMap map[*topology.Edge]*topology.Edge) topology.FaultTarget {
	return topology.FaultTarget{
		Kind:      target.Kind,
		Service:   mapService(target.Service, serviceMap),
		Operation: mapOperation(target.Operation, opMap),
		Edge:      cloneEdge(target.Edge, opMap, edgeMap),
	}
}

func mapService(svc *topology.Service, serviceMap map[*topology.Service]*topology.Service) *topology.Service {
	if svc == nil {
		return nil
	}
	if copied, ok := serviceMap[svc]; ok {
		return copied
	}
	return svc
}

func mapOperation(op *topology.Operation, opMap map[*topology.Operation]*topology.Operation) *topology.Operation {
	if op == nil {
		return nil
	}
	if copied, ok := opMap[op]; ok {
		return copied
	}
	return op
}

func cloneAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	copied := make(map[string]any, len(m))
	for key, value := range m {
		copied[key] = value
	}
	return copied
}

func allOperations(s *topology.Schema) []*topology.Operation {
	ops := make([]*topology.Operation, 0)
	for _, svc := range s.Services {
		for _, op := range svc.Operations {
			ops = append(ops, op)
		}
	}
	return ops
}

func edgeCallNodes(ops []*topology.Operation) []*topology.CallNode {
	nodes := make([]*topology.CallNode, 0)
	for _, op := range ops {
		nodes = append(nodes, edgeCallNodesFrom(op.Calls)...)
	}
	return nodes
}

func edgeCallNodesFrom(nodes []*topology.CallNode) []*topology.CallNode {
	found := make([]*topology.CallNode, 0)
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Edge != nil {
			found = append(found, node)
		}
		found = append(found, edgeCallNodesFrom(node.Parallel)...)
	}
	return found
}

func journeyNames(s *topology.Schema) []string {
	names := make([]string, 0, len(s.Journeys))
	for name := range s.Journeys {
		names = append(names, name)
	}
	return names
}

func serviceIDs(s *topology.Schema) []topology.ServiceID {
	ids := make([]topology.ServiceID, 0, len(s.Services))
	for id := range s.Services {
		ids = append(ids, id)
	}
	return ids
}
