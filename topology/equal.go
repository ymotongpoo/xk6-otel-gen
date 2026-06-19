// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology

import (
	"reflect"
	"time"
)

// Equal reports whether two schemas are identifier-equivalent.
func Equal(a, b *Schema) bool {
	if a == nil || b == nil {
		return a == b
	}
	return effectiveSchemaNamespace(a) == effectiveSchemaNamespace(b) &&
		equalServices(a.Services, b.Services) &&
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
		effectiveServiceNamespace(a) == effectiveServiceNamespace(b) &&
		a.Kind == b.Kind &&
		a.Replicas == b.Replicas &&
		a.Language == b.Language &&
		a.Framework == b.Framework &&
		a.Version == b.Version &&
		equalObservableMetrics(a.Metrics, b.Metrics) &&
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
	return identifyOp(a) == identifyOp(b) && equalCalls(a.Calls, b.Calls) && equalLogEvents(a.LogEvents, b.LogEvents) && equalMetrics(a.Metrics, b.Metrics) && equalStateUpdates(a.StateUpdates, b.StateUpdates) && equalProfile(a.Profile, b.Profile)
}

func equalProfile(a, b *ProfileSpec) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Enabled != b.Enabled || a.SampleRate != b.SampleRate {
		return false
	}
	if !equalStackSamples(a.Baseline, b.Baseline) || !equalStackSamples(a.Incident, b.Incident) {
		return false
	}
	if a.WhenFault == nil || b.WhenFault == nil {
		return a.WhenFault == b.WhenFault
	}
	return a.WhenFault.Kind == b.WhenFault.Kind
}

func equalStackSamples(a, b []StackSample) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Weight != b[i].Weight || !reflect.DeepEqual(a[i].Frames, b[i].Frames) {
			return false
		}
	}
	return true
}

func equalMetrics(a, b []MetricSpec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalMetricSpec(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalMetricSpec(a, b MetricSpec) bool {
	if a.Name != b.Name || a.Type != b.Type || a.Unit != b.Unit || a.Baseline != b.Baseline || a.Condition != b.Condition {
		return false
	}
	if !reflect.DeepEqual(a.Attributes, b.Attributes) {
		return false
	}
	if (a.WhenFault == nil) != (b.WhenFault == nil) {
		return false
	}
	if a.WhenFault == nil {
		return true
	}
	return a.WhenFault.Kind == b.WhenFault.Kind &&
		a.WhenFault.Delta == b.WhenFault.Delta &&
		a.WhenFault.Value == b.WhenFault.Value &&
		a.WhenFault.HasValue == b.WhenFault.HasValue
}

func equalObservableMetrics(a, b []ObservableMetricSpec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalObservableMetricSpec(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalObservableMetricSpec(a, b ObservableMetricSpec) bool {
	if a.Name != b.Name || a.Type != b.Type || a.Unit != b.Unit || a.Baseline != b.Baseline {
		return false
	}
	if !reflect.DeepEqual(a.Attributes, b.Attributes) {
		return false
	}
	if (a.WhenFault == nil) != (b.WhenFault == nil) || (a.Source == nil) != (b.Source == nil) {
		return false
	}
	if a.WhenFault != nil && !equalMetricFaultLink(a.WhenFault, b.WhenFault) {
		return false
	}
	if a.Source != nil && (a.Source.Accumulator != b.Source.Accumulator || a.Source.Minus != b.Source.Minus) {
		return false
	}
	return true
}

func equalStateUpdates(a, b []MetricStateUpdateSpec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Delta != b[i].Delta || a[i].Condition != b[i].Condition {
			return false
		}
		if (a[i].WhenFault == nil) != (b[i].WhenFault == nil) {
			return false
		}
		if a[i].WhenFault != nil && !equalMetricFaultLink(a[i].WhenFault, b[i].WhenFault) {
			return false
		}
	}
	return true
}

func equalMetricFaultLink(a, b *MetricFaultLink) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Kind == b.Kind &&
		a.Delta == b.Delta &&
		a.Value == b.Value &&
		a.HasValue == b.HasValue
}

func equalLogEvents(a, b []LogEventSpec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalLogEventSpec(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalLogEventSpec(a, b LogEventSpec) bool {
	return a.Name == b.Name &&
		a.Severity == b.Severity &&
		a.Condition == b.Condition &&
		a.Body == b.Body &&
		reflect.DeepEqual(a.Attributes, b.Attributes)
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
		effectiveRetryBaseDelay(a) == effectiveRetryBaseDelay(b) &&
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
		equalSeverity(a.Severity, b.Severity) &&
		equalFaultSchedule(a.Schedule, b.Schedule)
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

func equalFaultSchedule(a, b []FaultSchedulePoint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

func effectiveServiceNamespace(svc *Service) string {
	if svc == nil || svc.Namespace == "" {
		return DefaultNamespace
	}
	return svc.Namespace
}

func effectiveRetryBaseDelay(edge *Edge) time.Duration {
	if edge == nil || edge.RetryBaseDelay == 0 {
		return DefaultRetryBaseDelay
	}
	return edge.RetryBaseDelay
}
