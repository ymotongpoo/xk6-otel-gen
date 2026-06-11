// SPDX-License-Identifier: Apache-2.0

package topology

import "reflect"

// Equal reports whether two schemas are identifier-equivalent.
func Equal(a, b *Schema) bool {
	if a == nil || b == nil {
		return a == b
	}
	return equalServices(a.Services, b.Services) &&
		equalJourneys(a.Journeys, b.Journeys) &&
		equalFaults(a.Faults, b.Faults)
}

func equalServices(a, b map[ServiceID]*Service) bool {
	if len(a) != len(b) {
		return false
	}
	for id, aSvc := range a {
		if !equalService(aSvc, b[id]) {
			return false
		}
	}
	return true
}

func equalService(a, b *Service) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.Kind == b.Kind &&
		a.Replicas == b.Replicas &&
		a.Language == b.Language &&
		a.Framework == b.Framework &&
		a.Version == b.Version &&
		equalOperations(a.Operations, b.Operations)
}

func equalOperations(a, b map[string]*Operation) bool {
	if len(a) != len(b) {
		return false
	}
	for name, aOp := range a {
		if !equalOperation(aOp, b[name]) {
			return false
		}
	}
	return true
}

func equalOperation(a, b *Operation) bool {
	if a == nil || b == nil {
		return a == b
	}
	return identifyOp(a) == identifyOp(b) && equalCalls(a.Calls, b.Calls)
}

func equalCalls(a, b []*CallNode) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalCallNode(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalCallNode(a, b *CallNode) bool {
	if a == nil || b == nil {
		return a == b
	}
	return equalEdge(a.Edge, b.Edge) && equalCalls(a.Parallel, b.Parallel)
}

func equalEdge(a, b *Edge) bool {
	if a == nil || b == nil {
		return a == b
	}
	return identifyOp(a.From) == identifyOp(b.From) &&
		identifyOp(a.To) == identifyOp(b.To) &&
		a.Protocol == b.Protocol &&
		equalLatency(a.Latency, b.Latency) &&
		a.ErrorRate == b.ErrorRate &&
		a.Timeout == b.Timeout &&
		a.Retries == b.Retries &&
		a.RetryBackoff == b.RetryBackoff &&
		equalRecoveryPolicy(a.OnFailure, b.OnFailure)
}

func equalRecoveryPolicy(a, b *RecoveryPolicy) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.OnExhausted != b.OnExhausted || !reflect.DeepEqual(a.DefaultResponse, b.DefaultResponse) {
		return false
	}
	if len(a.Fallback) != len(b.Fallback) {
		return false
	}
	for i := range a.Fallback {
		if !equalEdge(a.Fallback[i], b.Fallback[i]) {
			return false
		}
	}
	return true
}

func equalJourneys(a, b map[string]*Journey) bool {
	if len(a) != len(b) {
		return false
	}
	for name, aJourney := range a {
		if !equalJourney(aJourney, b[name]) {
			return false
		}
	}
	return true
}

func equalJourney(a, b *Journey) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name && a.Weight == b.Weight && equalSteps(a.Steps, b.Steps)
}

func equalSteps(a, b []*Step) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalStep(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalStep(a, b *Step) bool {
	if a == nil || b == nil {
		return a == b
	}
	return identifyOp(a.Op) == identifyOp(b.Op) && equalSteps(a.Parallel, b.Parallel)
}

func equalFaults(a, b []FaultSpec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalFaultSpec(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalFaultSpec(a, b FaultSpec) bool {
	return a.Kind == b.Kind &&
		equalFaultTarget(a.Target, b.Target) &&
		equalSeverity(a.Severity, b.Severity)
}

func equalFaultTarget(a, b FaultTarget) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case TargetNode:
		return serviceID(a.Service) == serviceID(b.Service)
	case TargetOperation:
		return identifyOp(a.Operation) == identifyOp(b.Operation)
	case TargetEdge:
		return edgeIdentity(a.Edge) == edgeIdentity(b.Edge)
	default:
		return serviceID(a.Service) == serviceID(b.Service) &&
			identifyOp(a.Operation) == identifyOp(b.Operation) &&
			edgeIdentity(a.Edge) == edgeIdentity(b.Edge)
	}
}

func equalLatency(a, b LatencyDist) bool {
	return a.Distribution == b.Distribution && a.P50 == b.P50 && a.P95 == b.P95
}

func equalSeverity(a, b SeverityParams) bool {
	return a.Probability == b.Probability &&
		a.Multiplier == b.Multiplier &&
		a.Add == b.Add &&
		a.Value == b.Value
}

func serviceID(svc *Service) ServiceID {
	if svc == nil {
		return ""
	}
	return svc.Name
}

func edgeIdentity(edge *Edge) string {
	if edge == nil {
		return "<nil>"
	}
	return identifyOp(edge.From) + "->" + identifyOp(edge.To)
}
