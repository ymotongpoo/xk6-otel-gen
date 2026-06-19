// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology

import (
	"sort"
	"time"
)

// MarshalYAML converts a resolved Schema into the topology YAML shape.
func (s *Schema) MarshalYAML() (any, error) {
	raw := &rawSchema{
		Namespace: s.Namespace,
		Services:  make(map[string]*rawService, len(s.Services)),
		Journeys:  make(map[string]*rawJourney, len(s.Journeys)),
		Faults:    make([]*rawFault, 0, len(s.Faults)),
	}
	if raw.Namespace == "" || raw.Namespace == DefaultNamespace {
		raw.Namespace = ""
	}
	for _, id := range sortedServiceIDs(s.Services) {
		raw.Services[string(id)] = marshalService(s.Services[id], effectiveSchemaNamespace(s))
	}
	for _, name := range sortedKeys(s.Journeys) {
		raw.Journeys[name] = marshalJourney(s.Journeys[name])
	}
	for _, fault := range s.Faults {
		raw.Faults = append(raw.Faults, marshalFault(fault))
	}
	return raw, nil
}

func sortedServiceIDs(m map[ServiceID]*Service) []ServiceID {
	ids := make([]ServiceID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func marshalService(svc *Service, schemaNamespace string) *rawService {
	if svc == nil {
		return &rawService{}
	}
	rs := &rawService{
		Kind:       svc.Kind.String(),
		Language:   svc.Language,
		Framework:  svc.Framework,
		Version:    svc.Version,
		Metrics:    marshalObservableMetrics(svc.Metrics),
		Operations: marshalOperations(svc.Operations),
	}
	if svc.Namespace != "" && svc.Namespace != schemaNamespace {
		rs.Namespace = svc.Namespace
	}
	if svc.Replicas != 1 {
		rs.Replicas = ptrInt(svc.Replicas)
	}
	return rs
}

func effectiveSchemaNamespace(s *Schema) string {
	if s == nil || s.Namespace == "" {
		return DefaultNamespace
	}
	return s.Namespace
}

func marshalOperations(ops map[string]*Operation) []*rawOperation {
	names := sortedKeys(ops)
	out := make([]*rawOperation, 0, len(names))
	for _, name := range names {
		op := ops[name]
		if op == nil {
			out = append(out, &rawOperation{Name: name})
			continue
		}
		out = append(out, &rawOperation{
			Name:         op.Name,
			Calls:        marshalCallNodes(op.Calls),
			LogEvents:    marshalLogEvents(op.LogEvents),
			Metrics:      marshalMetrics(op.Metrics),
			StateUpdates: marshalStateUpdates(op.StateUpdates),
			Profile:      marshalProfile(op.Profile),
		})
	}
	return out
}

func marshalLogEvents(events []LogEventSpec) []*rawLogEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]*rawLogEvent, 0, len(events))
	for _, ev := range events {
		raw := &rawLogEvent{
			Name:       ev.Name,
			Body:       ev.Body,
			Attributes: ev.Attributes,
		}
		if ev.Severity != SeverityInfo {
			raw.Severity = ev.Severity.String()
		}
		if ev.Condition != ConditionAlways {
			raw.Condition = ev.Condition.String()
		}
		out = append(out, raw)
	}
	return out
}

func marshalProfile(spec *ProfileSpec) *rawProfile {
	if spec == nil {
		return nil
	}
	raw := &rawProfile{
		Enabled:  spec.Enabled,
		Baseline: marshalStacks(spec.Baseline),
		Incident: marshalStacks(spec.Incident),
	}
	if spec.SampleRate != 0 && spec.SampleRate != 100 {
		raw.SampleRate = ptrInt(spec.SampleRate)
	}
	if spec.WhenFault != nil {
		raw.WhenFault = &rawProfileFault{Kind: spec.WhenFault.Kind.String()}
	}
	return raw
}

func marshalStacks(stacks []StackSample) []*rawStack {
	if len(stacks) == 0 {
		return nil
	}
	out := make([]*rawStack, 0, len(stacks))
	for _, stack := range stacks {
		out = append(out, &rawStack{
			Frames: append([]string(nil), stack.Frames...),
			Weight: stack.Weight,
		})
	}
	return out
}

func marshalMetrics(metrics []MetricSpec) []*rawMetric {
	if len(metrics) == 0 {
		return nil
	}
	out := make([]*rawMetric, 0, len(metrics))
	for _, m := range metrics {
		raw := &rawMetric{
			Name:       m.Name,
			Type:       m.Type.String(),
			Unit:       m.Unit,
			Attributes: m.Attributes,
		}
		if m.Baseline != 0 {
			raw.Baseline = ptrFloat64(m.Baseline)
		}
		if m.Condition != ConditionAlways {
			raw.Condition = m.Condition.String()
		}
		if m.WhenFault != nil {
			link := &rawMetricFaultLink{Kind: m.WhenFault.Kind.String()}
			if m.WhenFault.HasValue {
				link.Value = ptrFloat64(m.WhenFault.Value)
			} else if m.WhenFault.Delta != 0 {
				link.Delta = ptrFloat64(m.WhenFault.Delta)
			}
			raw.WhenFault = link
		}
		out = append(out, raw)
	}
	return out
}

func marshalObservableMetrics(metrics []ObservableMetricSpec) []*rawMetric {
	if len(metrics) == 0 {
		return nil
	}
	out := make([]*rawMetric, 0, len(metrics))
	for _, m := range metrics {
		raw := &rawMetric{
			Name:       m.Name,
			Type:       m.Type.String(),
			Unit:       m.Unit,
			Attributes: m.Attributes,
		}
		if m.Baseline != 0 {
			raw.Baseline = ptrFloat64(m.Baseline)
		}
		if m.WhenFault != nil {
			raw.WhenFault = marshalMetricFaultLink(m.WhenFault)
		}
		if m.Source != nil {
			raw.Source = &rawMetricSource{
				Accumulator: m.Source.Accumulator,
				Minus:       m.Source.Minus,
			}
		}
		out = append(out, raw)
	}
	return out
}

func marshalStateUpdates(updates []MetricStateUpdateSpec) []*rawStateUpdate {
	if len(updates) == 0 {
		return nil
	}
	out := make([]*rawStateUpdate, 0, len(updates))
	for _, update := range updates {
		raw := &rawStateUpdate{
			Key: update.Key,
		}
		if update.Delta != 0 {
			raw.Delta = ptrFloat64(update.Delta)
		}
		if update.Condition != ConditionAlways {
			raw.Condition = update.Condition.String()
		}
		if update.WhenFault != nil {
			raw.WhenFault = marshalMetricFaultLink(update.WhenFault)
		}
		out = append(out, raw)
	}
	return out
}

func marshalMetricFaultLink(link *MetricFaultLink) *rawMetricFaultLink {
	if link == nil {
		return nil
	}
	raw := &rawMetricFaultLink{Kind: link.Kind.String()}
	if link.HasValue {
		raw.Value = ptrFloat64(link.Value)
	} else if link.Delta != 0 {
		raw.Delta = ptrFloat64(link.Delta)
	}
	return raw
}

func marshalCallNodes(nodes []*CallNode) []*rawCallNode {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]*rawCallNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, marshalCallNode(node))
	}
	return out
}

func marshalCallNode(n *CallNode) *rawCallNode {
	if n == nil {
		return &rawCallNode{}
	}
	if n.Edge != nil {
		return marshalEdge(n.Edge)
	}
	return &rawCallNode{Parallel: marshalCallNodes(n.Parallel)}
}

func marshalEdge(e *Edge) *rawCallNode {
	if e == nil {
		return &rawCallNode{}
	}
	rc := &rawCallNode{
		To:           marshalCallTarget(e.To),
		Protocol:     e.Protocol.String(),
		Latency:      marshalLatency(e.Latency),
		RetryBackoff: e.RetryBackoff.String(),
		OnFailure:    marshalRecoveryPolicy(e.OnFailure),
	}
	if e.ErrorRate != 0 {
		rc.ErrorRate = ptrFloat64(e.ErrorRate)
	}
	if e.Timeout != 0 {
		rc.Timeout = ptrDuration(e.Timeout)
	}
	if e.Retries != 0 {
		rc.Retries = ptrInt(e.Retries)
	}
	if e.RetryBaseDelay != 0 && e.RetryBaseDelay != DefaultRetryBaseDelay {
		rc.RetryBaseDelay = ptrDuration(e.RetryBaseDelay)
	}
	if e.RetryBackoff == BackoffExponential {
		rc.RetryBackoff = ""
	}
	return rc
}

func marshalCallTarget(op *Operation) *rawCallTarget {
	if op == nil {
		return &rawCallTarget{}
	}
	target := &rawCallTarget{Operation: op.Name}
	if op.Service != nil {
		target.Service = string(op.Service.Name)
	}
	return target
}

func marshalLatency(latency LatencyDist) *rawLatencyDist {
	distribution := latency.Distribution
	if distribution == "" {
		distribution = "constant"
	}
	if distribution == "constant" && latency.P50 == 0 && latency.P95 == 0 {
		return nil
	}

	raw := &rawLatencyDist{}
	if distribution != "constant" {
		raw.Distribution = distribution
	}
	if latency.P50 != 0 {
		raw.P50 = ptrDuration(latency.P50)
	}
	if latency.P95 != 0 && latency.P95 != latency.P50 {
		raw.P95 = ptrDuration(latency.P95)
	}
	return raw
}

func marshalRecoveryPolicy(rp *RecoveryPolicy) *rawRecoveryPolicy {
	if rp == nil {
		return nil
	}
	raw := &rawRecoveryPolicy{
		Fallback:        make([]*rawCallNode, 0, len(rp.Fallback)),
		DefaultResponse: rp.DefaultResponse,
	}
	for _, fallback := range rp.Fallback {
		raw.Fallback = append(raw.Fallback, marshalEdge(fallback))
	}
	if rp.OnExhausted != ExhaustedPropagate {
		raw.OnExhausted = rp.OnExhausted.String()
	}
	return raw
}

func marshalJourney(j *Journey) *rawJourney {
	if j == nil {
		return &rawJourney{}
	}
	raw := &rawJourney{
		Steps: marshalSteps(j.Steps),
	}
	if j.Weight != 1 {
		raw.Weight = ptrFloat64(j.Weight)
	}
	return raw
}

func marshalSteps(steps []*Step) []*rawStep {
	if len(steps) == 0 {
		return nil
	}
	out := make([]*rawStep, 0, len(steps))
	for _, step := range steps {
		out = append(out, marshalStep(step))
	}
	return out
}

func marshalStep(s *Step) *rawStep {
	if s == nil {
		return &rawStep{}
	}
	if s.Op != nil {
		raw := &rawStep{Operation: s.Op.Name}
		if s.Op.Service != nil {
			raw.Service = string(s.Op.Service.Name)
		}
		return raw
	}
	return &rawStep{Parallel: marshalSteps(s.Parallel)}
}

func marshalFault(f FaultSpec) *rawFault {
	return &rawFault{
		Target:   marshalFaultTarget(f.Target),
		Kind:     f.Kind.String(),
		Severity: marshalSeverity(f.Severity),
		Schedule: marshalFaultSchedule(f.Schedule),
	}
}

func marshalFaultTarget(t FaultTarget) string {
	switch t.Kind {
	case TargetNode:
		if t.Service == nil {
			return "node:"
		}
		return "node:" + string(t.Service.Name)
	case TargetOperation:
		return "operation:" + identifyOp(t.Operation)
	case TargetEdge:
		if t.Edge == nil {
			return "edge:"
		}
		return "edge:" + identifyOp(t.Edge.From) + "->" + identifyOp(t.Edge.To)
	default:
		return "unknown:"
	}
}

func marshalSeverity(severity SeverityParams) *rawSeverity {
	if severity == (SeverityParams{}) {
		return nil
	}
	raw := &rawSeverity{}
	if severity.Probability != 0 {
		raw.Probability = ptrFloat64(severity.Probability)
	}
	if severity.Multiplier != 0 {
		raw.Multiplier = ptrFloat64(severity.Multiplier)
	}
	if severity.Add != 0 {
		raw.Add = ptrDuration(severity.Add)
	}
	if severity.Value != 0 {
		raw.Value = ptrFloat64(severity.Value)
	}
	return raw
}

func marshalFaultSchedule(schedule []FaultSchedulePoint) []*rawFaultSchedule {
	if len(schedule) == 0 {
		return nil
	}
	out := make([]*rawFaultSchedule, 0, len(schedule))
	for _, point := range schedule {
		out = append(out, &rawFaultSchedule{
			At:        ptrDuration(point.At),
			Intensity: ptrFloat64(point.Intensity),
		})
	}
	return out
}

func ptrInt(v int) *int {
	return &v
}

func ptrFloat64(v float64) *float64 {
	return &v
}

func ptrDuration(v time.Duration) *time.Duration {
	return &v
}
