// SPDX-License-Identifier: Apache-2.0

package topology

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// Validate checks structural invariants and domain ranges for a schema.
func Validate(s *Schema) error {
	errs := make([]error, 0, 16)

	errs = append(errs, validateMapKeyConsistency(s)...)
	errs = append(errs, validateBackPointers(s)...)
	errs = append(errs, validateNoOrphanReferences(s)...)
	errs = append(errs, validateDAG(s)...)
	errs = append(errs, validateJourneyReachability(s)...)
	errs = append(errs, validateFaultTargets(s)...)
	errs = append(errs, validateCallNodeVariants(s)...)
	errs = append(errs, validateRecoveryPolicyOwnership(s)...)
	errs = append(errs, validateDomainRanges(s)...)

	return errors.Join(errs...)
}

func validateMapKeyConsistency(s *Schema) []error {
	if s == nil {
		return []error{newValidationError("schema", "R-STR-1", "schema is nil")}
	}

	errs := make([]error, 0)
	for id, svc := range s.Services {
		path := fmt.Sprintf("services.%s", id)
		if svc == nil {
			errs = append(errs, newValidationError(path, "R-STR-1", "service is nil"))
			continue
		}
		if svc.Name != id {
			errs = append(errs, newValidationErrorf(path, "R-STR-1", "name mismatch (key=%s, Service.Name=%s)", id, svc.Name))
		}
	}
	return errs
}

func validateBackPointers(s *Schema) []error {
	if s == nil {
		return nil
	}

	errs := make([]error, 0)
	for id, svc := range s.Services {
		if svc == nil {
			continue
		}
		for name, op := range svc.Operations {
			path := fmt.Sprintf("services.%s.operations.%s", id, name)
			if op == nil {
				errs = append(errs, newValidationError(path, "R-STR-2", "operation is nil"))
				continue
			}
			if op.Service != svc {
				errs = append(errs, newValidationErrorf(path, "R-STR-2", "Service back-pointer points to %q, expected %q", serviceName(op.Service), id))
			}
			if op.Name != name {
				errs = append(errs, newValidationErrorf(path, "R-STR-2", "operation name mismatch (key=%s, Operation.Name=%s)", name, op.Name))
			}
			if svc.Operations[op.Name] != op {
				errs = append(errs, newValidationErrorf(path, "R-STR-2", "operation is not reachable by its own name %q", op.Name))
			}
		}
	}
	return errs
}

func validateNoOrphanReferences(s *Schema) []error {
	if s == nil {
		return nil
	}

	opSet := operationSet(s)
	errs := make([]error, 0)
	for id, svc := range s.Services {
		if svc == nil {
			continue
		}
		for name, op := range svc.Operations {
			if op == nil {
				continue
			}
			path := fmt.Sprintf("services.%s.operations.%s.calls", id, name)
			errs = append(errs, validateNoOrphanCallReferences(op.Calls, path, opSet)...)
		}
	}
	return errs
}

func validateNoOrphanCallReferences(nodes []*CallNode, path string, opSet map[*Operation]struct{}) []error {
	errs := make([]error, 0)
	for i, node := range nodes {
		nodePath := fmt.Sprintf("%s[%d]", path, i)
		if node == nil {
			continue
		}
		if node.Edge != nil {
			errs = append(errs, validateEdgeEndpoints(node.Edge, nodePath, opSet)...)
			if node.Edge.OnFailure != nil {
				for j, fallback := range node.Edge.OnFailure.Fallback {
					errs = append(errs, validateEdgeEndpoints(fallback, fmt.Sprintf("%s.on_failure.fallback[%d]", nodePath, j), opSet)...)
				}
			}
		}
		errs = append(errs, validateNoOrphanCallReferences(node.Parallel, nodePath+".parallel", opSet)...)
	}
	return errs
}

func validateEdgeEndpoints(edge *Edge, path string, opSet map[*Operation]struct{}) []error {
	if edge == nil {
		return []error{newValidationError(path, "R-STR-3", "edge is nil")}
	}

	errs := make([]error, 0, 2)
	if _, ok := opSet[edge.From]; !ok {
		errs = append(errs, newValidationErrorf(path+".from", "R-STR-3", "operation %q is not in schema", identifyOp(edge.From)))
	}
	if _, ok := opSet[edge.To]; !ok {
		errs = append(errs, newValidationErrorf(path+".to", "R-STR-3", "operation %q is not in schema", identifyOp(edge.To)))
	}
	return errs
}

func validateDAG(s *Schema) []error {
	if s == nil {
		return nil
	}

	allOps := allSchemaOperations(s)
	inDegree := make(map[*Operation]int, len(allOps))
	for _, op := range allOps {
		if op != nil {
			inDegree[op] = 0
		}
	}
	forEachOutgoingEdge(s, func(e *Edge) {
		if e == nil || e.To == nil {
			return
		}
		if _, ok := inDegree[e.To]; ok {
			inDegree[e.To]++
		}
	})

	queue := make([]*Operation, 0, len(inDegree))
	for op, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, op)
		}
	}

	visited := 0
	for len(queue) > 0 {
		op := queue[0]
		queue = queue[1:]
		visited++
		for _, child := range outgoingTargets(op) {
			if _, ok := inDegree[child]; !ok {
				continue
			}
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	if visited == len(inDegree) {
		return nil
	}

	unvisited := make([]string, 0)
	for op, deg := range inDegree {
		if deg > 0 {
			unvisited = append(unvisited, identifyOp(op))
		}
	}
	sort.Strings(unvisited)
	return []error{newValidationErrorf("operation graph", "R-STR-4", "cycle detected; operations in cycle: %v", unvisited)}
}

func validateJourneyReachability(s *Schema) []error {
	if s == nil {
		return nil
	}

	opSet := operationSet(s)
	errs := make([]error, 0)
	for name, journey := range s.Journeys {
		path := fmt.Sprintf("journeys.%s", name)
		if journey == nil {
			errs = append(errs, newValidationError(path, "R-STR-5", "journey is nil"))
			continue
		}
		errs = append(errs, validateStepsReachable(journey.Steps, path+".steps", opSet)...)
	}
	return errs
}

func validateStepsReachable(steps []*Step, path string, opSet map[*Operation]struct{}) []error {
	errs := make([]error, 0)
	for i, step := range steps {
		stepPath := fmt.Sprintf("%s[%d]", path, i)
		if step == nil {
			errs = append(errs, newValidationError(stepPath, "R-STR-5", "step is nil"))
			continue
		}
		hasOp := step.Op != nil
		hasParallel := len(step.Parallel) > 0
		if hasOp == hasParallel {
			errs = append(errs, newValidationError(stepPath, "R-STR-5", "exactly one of Op or Parallel is required"))
		}
		if hasOp {
			if _, ok := opSet[step.Op]; !ok {
				errs = append(errs, newValidationErrorf(stepPath+".op", "R-STR-5", "operation %q is not in schema", identifyOp(step.Op)))
			}
		}
		if hasParallel {
			errs = append(errs, validateStepsReachable(step.Parallel, stepPath+".parallel", opSet)...)
		}
	}
	return errs
}

func validateFaultTargets(s *Schema) []error {
	if s == nil {
		return nil
	}

	opSet := operationSet(s)
	edgeSet := edgeSet(s)
	errs := make([]error, 0)
	for i, fault := range s.Faults {
		path := fmt.Sprintf("faults[%d].target", i)
		setCount := 0
		if fault.Target.Service != nil {
			setCount++
		}
		if fault.Target.Operation != nil {
			setCount++
		}
		if fault.Target.Edge != nil {
			setCount++
		}
		if setCount != 1 {
			errs = append(errs, newValidationErrorf(path, "R-STR-6", "exactly one target pointer is required, got %d", setCount))
			continue
		}

		switch fault.Target.Kind {
		case TargetNode:
			if fault.Target.Service == nil {
				errs = append(errs, newValidationError(path, "R-STR-6", "node target requires Service"))
			} else if !serviceInSchema(s, fault.Target.Service) {
				errs = append(errs, newValidationErrorf(path, "R-STR-6", "service %q is not in schema", fault.Target.Service.Name))
			}
		case TargetOperation:
			if fault.Target.Operation == nil {
				errs = append(errs, newValidationError(path, "R-STR-6", "operation target requires Operation"))
			} else if _, ok := opSet[fault.Target.Operation]; !ok {
				errs = append(errs, newValidationErrorf(path, "R-STR-6", "operation %q is not in schema", identifyOp(fault.Target.Operation)))
			}
		case TargetEdge:
			if fault.Target.Edge == nil {
				errs = append(errs, newValidationError(path, "R-STR-6", "edge target requires Edge"))
			} else if _, ok := edgeSet[fault.Target.Edge]; !ok {
				errs = append(errs, newValidationError(path, "R-STR-6", "edge is not in schema"))
			}
		default:
			errs = append(errs, newValidationErrorf(path+".kind", "R-STR-6", "unsupported target kind %d", fault.Target.Kind))
		}
	}
	return errs
}

func validateCallNodeVariants(s *Schema) []error {
	if s == nil {
		return nil
	}

	errs := make([]error, 0)
	for id, svc := range s.Services {
		if svc == nil {
			continue
		}
		for name, op := range svc.Operations {
			if op == nil {
				continue
			}
			errs = append(errs, validateCallNodeVariantList(op.Calls, fmt.Sprintf("services.%s.operations.%s.calls", id, name))...)
		}
	}
	return errs
}

func validateCallNodeVariantList(nodes []*CallNode, path string) []error {
	errs := make([]error, 0)
	for i, node := range nodes {
		nodePath := fmt.Sprintf("%s[%d]", path, i)
		if node == nil {
			errs = append(errs, newValidationError(nodePath, "R-STR-7", "call node is nil"))
			continue
		}
		hasEdge := node.Edge != nil
		hasParallel := len(node.Parallel) > 0
		if hasEdge == hasParallel {
			errs = append(errs, newValidationError(nodePath, "R-STR-7", "exactly one of Edge or Parallel is required"))
		}
		if hasParallel {
			errs = append(errs, validateCallNodeVariantList(node.Parallel, nodePath+".parallel")...)
		}
	}
	return errs
}

func validateRecoveryPolicyOwnership(s *Schema) []error {
	if s == nil {
		return nil
	}

	errs := make([]error, 0)
	forEachOutgoingEdgeWithPath(s, func(edge *Edge, path string) {
		errs = append(errs, validateRecoveryPolicyOwnershipForEdge(edge, path)...)
	})
	return errs
}

func validateRecoveryPolicyOwnershipForEdge(edge *Edge, path string) []error {
	if edge == nil || edge.OnFailure == nil {
		return nil
	}

	errs := make([]error, 0)
	for i, fallback := range edge.OnFailure.Fallback {
		fallbackPath := fmt.Sprintf("%s.on_failure.fallback[%d]", path, i)
		if fallback == nil {
			errs = append(errs, newValidationError(fallbackPath, "R-STR-8", "fallback edge is nil"))
			continue
		}
		if fallback.From != edge.From {
			errs = append(errs, newValidationErrorf(fallbackPath, "R-STR-8", "From mismatch: got %s, expected %s", identifyOp(fallback.From), identifyOp(edge.From)))
		}
		errs = append(errs, validateRecoveryPolicyOwnershipForEdge(fallback, fallbackPath)...)
	}
	return errs
}

func validateDomainRanges(s *Schema) []error {
	if s == nil {
		return []error{
			newValidationError("services", "D-13", "must contain at least one service"),
			newValidationError("journeys", "D-14", "must contain at least one journey"),
		}
	}

	errs := make([]error, 0)
	if len(s.Services) == 0 {
		errs = append(errs, newValidationError("services", "D-13", "must contain at least one service"))
	}
	if len(s.Journeys) == 0 {
		errs = append(errs, newValidationError("journeys", "D-14", "must contain at least one journey"))
	}

	for id, svc := range s.Services {
		path := fmt.Sprintf("services.%s", id)
		if svc == nil {
			continue
		}
		if svc.Replicas < 1 {
			errs = append(errs, newValidationErrorf(path+".replicas", "D-1", "must be >= 1, got %d", svc.Replicas))
		}
		if !validServiceKind(svc.Kind) {
			errs = append(errs, newValidationErrorf(path+".kind", "D-ENUM", "unsupported service kind %d", svc.Kind))
		}
		if len(svc.Operations) == 0 {
			errs = append(errs, newValidationError(path+".operations", "D-12", "must contain at least one operation"))
		}
		for name, op := range svc.Operations {
			if op == nil {
				continue
			}
			opPath := fmt.Sprintf("%s.operations.%s", path, name)
			if op.Name == "" {
				errs = append(errs, newValidationError(opPath+".name", "D-OP", "operation name must be non-empty"))
			}
			if len(op.Name) > 120 {
				errs = append(errs, newValidationErrorf(opPath+".name", "D-OP", "operation name must be <= 120 bytes, got %d", len(op.Name)))
			}
		}
	}

	forEachOutgoingEdgeWithPath(s, func(edge *Edge, path string) {
		errs = append(errs, validateEdgeDomain(edge, path)...)
	})

	for name, journey := range s.Journeys {
		path := fmt.Sprintf("journeys.%s", name)
		if journey == nil {
			continue
		}
		if journey.Name != name {
			errs = append(errs, newValidationErrorf(path+".name", "D-JOURNEY", "name mismatch (key=%s, Journey.Name=%s)", name, journey.Name))
		}
		if journey.Weight <= 0 {
			errs = append(errs, newValidationErrorf(path+".weight", "D-10", "must be > 0, got %g", journey.Weight))
		}
		if len(journey.Steps) == 0 {
			errs = append(errs, newValidationError(path+".steps", "D-11", "must contain at least one step"))
		}
	}

	for i, fault := range s.Faults {
		path := fmt.Sprintf("faults[%d]", i)
		if !validFaultKind(fault.Kind) {
			errs = append(errs, newValidationErrorf(path+".kind", "D-ENUM", "unsupported fault kind %d", fault.Kind))
		}
		if !validProbability(fault.Severity.Probability) {
			errs = append(errs, newValidationErrorf(path+".severity.probability", "D-8", "must be in [0,1], got %g", fault.Severity.Probability))
		}
		if fault.Kind == FaultLatencyInflation && fault.Severity.Multiplier <= 0 {
			errs = append(errs, newValidationErrorf(path+".severity.multiplier", "D-9", "must be > 0 for latency_inflation, got %g", fault.Severity.Multiplier))
		}
	}

	return errs
}

func validateEdgeDomain(edge *Edge, path string) []error {
	if edge == nil {
		return nil
	}

	errs := make([]error, 0)
	if !validProtocol(edge.Protocol) {
		errs = append(errs, newValidationErrorf(path+".protocol", "D-ENUM", "unsupported protocol %d", edge.Protocol))
	}
	if !validProbability(edge.ErrorRate) {
		errs = append(errs, newValidationErrorf(path+".error_rate", "D-2", "must be in [0,1], got %g", edge.ErrorRate))
	}
	if edge.Timeout < 0 {
		errs = append(errs, newValidationErrorf(path+".timeout", "D-3", "must be >= 0, got %s", edge.Timeout))
	}
	if edge.Retries < 0 {
		errs = append(errs, newValidationErrorf(path+".retries", "D-4", "must be >= 0, got %d", edge.Retries))
	}
	if edge.Latency.P50 < 0 {
		errs = append(errs, newValidationErrorf(path+".latency.p50", "D-6", "must be >= 0, got %s", edge.Latency.P50))
	}
	if edge.Latency.P95 < edge.Latency.P50 {
		errs = append(errs, newValidationErrorf(path+".latency", "D-5", "p95 (%s) must be >= p50 (%s)", edge.Latency.P95, edge.Latency.P50))
	}
	if !validLatencyDistribution(edge.Latency.Distribution) {
		errs = append(errs, newValidationErrorf(path+".latency.distribution", "D-7", "unsupported %q; allowed: constant/lognormal/normal/exponential", edge.Latency.Distribution))
	}
	if !validBackoff(edge.RetryBackoff) {
		errs = append(errs, newValidationErrorf(path+".retry_backoff", "D-ENUM", "unsupported backoff policy %d", edge.RetryBackoff))
	}
	if edge.OnFailure != nil && !validExhausted(edge.OnFailure.OnExhausted) {
		errs = append(errs, newValidationErrorf(path+".on_failure.on_exhausted", "D-ENUM", "unsupported exhausted action %d", edge.OnFailure.OnExhausted))
	}
	return errs
}

func forEachOutgoingEdge(s *Schema, fn func(*Edge)) {
	if s == nil {
		return
	}
	for _, svc := range s.Services {
		if svc == nil {
			continue
		}
		for _, op := range svc.Operations {
			if op == nil {
				continue
			}
			forEachEdgeInCalls(op.Calls, fn)
		}
	}
}

func forEachEdgeInCalls(calls []*CallNode, fn func(*Edge)) {
	for _, node := range calls {
		if node == nil {
			continue
		}
		if node.Edge != nil {
			fn(node.Edge)
			if node.Edge.OnFailure != nil {
				for _, fallback := range node.Edge.OnFailure.Fallback {
					if fallback != nil {
						fn(fallback)
					}
				}
			}
		}
		forEachEdgeInCalls(node.Parallel, fn)
	}
}

func outgoingTargets(op *Operation) []*Operation {
	targets := make([]*Operation, 0)
	if op == nil {
		return targets
	}
	forEachEdgeInCalls(op.Calls, func(e *Edge) {
		if e != nil && e.To != nil {
			targets = append(targets, e.To)
		}
	})
	return targets
}

func identifyOp(op *Operation) string {
	if op == nil {
		return "<nil>"
	}
	if op.Service == nil {
		return fmt.Sprintf("<nil>.%s", op.Name)
	}
	return fmt.Sprintf("%s.%s", op.Service.Name, op.Name)
}

func allSchemaOperations(s *Schema) []*Operation {
	if s == nil {
		return nil
	}
	ops := make([]*Operation, 0)
	for _, svc := range s.Services {
		if svc == nil {
			continue
		}
		for _, op := range svc.Operations {
			if op != nil {
				ops = append(ops, op)
			}
		}
	}
	return ops
}

func operationSet(s *Schema) map[*Operation]struct{} {
	ops := allSchemaOperations(s)
	set := make(map[*Operation]struct{}, len(ops))
	for _, op := range ops {
		set[op] = struct{}{}
	}
	return set
}

func edgeSet(s *Schema) map[*Edge]struct{} {
	set := make(map[*Edge]struct{})
	forEachOutgoingEdge(s, func(edge *Edge) {
		if edge != nil {
			set[edge] = struct{}{}
		}
	})
	return set
}

func serviceInSchema(s *Schema, svc *Service) bool {
	if s == nil || svc == nil {
		return false
	}
	for _, existing := range s.Services {
		if existing == svc {
			return true
		}
	}
	return false
}

func forEachOutgoingEdgeWithPath(s *Schema, fn func(*Edge, string)) {
	if s == nil {
		return
	}
	for id, svc := range s.Services {
		if svc == nil {
			continue
		}
		for name, op := range svc.Operations {
			if op == nil {
				continue
			}
			forEachCallEdgeWithPath(op.Calls, fmt.Sprintf("services.%s.operations.%s.calls", id, name), fn)
		}
	}
}

func forEachCallEdgeWithPath(nodes []*CallNode, path string, fn func(*Edge, string)) {
	for i, node := range nodes {
		nodePath := fmt.Sprintf("%s[%d]", path, i)
		if node == nil {
			continue
		}
		if node.Edge != nil {
			fn(node.Edge, nodePath)
			if node.Edge.OnFailure != nil {
				for j, fallback := range node.Edge.OnFailure.Fallback {
					fn(fallback, fmt.Sprintf("%s.on_failure.fallback[%d]", nodePath, j))
				}
			}
		}
		forEachCallEdgeWithPath(node.Parallel, nodePath+".parallel", fn)
	}
}

func serviceName(svc *Service) string {
	if svc == nil {
		return "<nil>"
	}
	return string(svc.Name)
}

func validProbability(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1
}

func validServiceKind(k ServiceKind) bool {
	return k == KindApplication || k == KindDatabase || k == KindExternalAPI || k == KindCache || k == KindQueue
}

func validProtocol(p Protocol) bool {
	return p == ProtocolHTTP || p == ProtocolGRPC || p == ProtocolMessaging
}

func validBackoff(p BackoffPolicy) bool {
	return p == BackoffExponential || p == BackoffLinear || p == BackoffConstant
}

func validFaultKind(k FaultKind) bool {
	return k == FaultLatencyInflation || k == FaultErrorRateOverride || k == FaultDisconnect || k == FaultCrash
}

func validExhausted(a ExhaustedAction) bool {
	return a == ExhaustedPropagate || a == ExhaustedReturnDefault || a == ExhaustedSucceedSilently
}

func validLatencyDistribution(d string) bool {
	switch d {
	case "constant", "lognormal", "normal", "exponential":
		return true
	default:
		return false
	}
}
