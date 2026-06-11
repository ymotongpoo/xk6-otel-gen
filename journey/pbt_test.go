// SPDX-License-Identifier: Apache-2.0

package journey_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/journey"
	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func TestBuildPlan_Idempotent_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		schema := generators.ValidSchema(
			generators.MaxServices(5),
			generators.MaxOpsPerService(3),
			generators.MaxCallsPerOp(3),
			generators.MaxFaults(0),
		).Draw(t, "schema")
		engine := journey.NewEngine(schema, schema.ApplyFaults(), &pbtSynth{})
		name := rapid.SampledFrom(engine.ListJourneys()).Draw(t, "journey")

		p1, err := engine.BuildPlan(name)
		if err != nil {
			t.Fatalf("BuildPlan first error = %v", err)
		}
		p2, err := engine.BuildPlan(name)
		if err != nil {
			t.Fatalf("BuildPlan second error = %v", err)
		}
		if !plansEqual(p1, p2) {
			t.Fatalf("BuildPlan is not idempotent:\n%+v\n%+v", p1, p2)
		}
	})
}

func TestBuildPlan_AllOpsVisited_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		schema := generators.ValidSchema(
			generators.MaxServices(5),
			generators.MaxOpsPerService(3),
			generators.MaxCallsPerOp(3),
			generators.MaxFaults(0),
		).Draw(t, "schema")
		engine := journey.NewEngine(schema, schema.ApplyFaults(), &pbtSynth{})
		name := rapid.SampledFrom(engine.ListJourneys()).Draw(t, "journey")
		plan, err := engine.BuildPlan(name)
		if err != nil {
			t.Fatalf("BuildPlan(%q) error = %v", name, err)
		}

		want := map[string]struct{}{}
		collectStepOps(schema.Journeys[name].Steps, want)
		got := map[string]struct{}{}
		collectPlanOps(plan.Root, got)
		for op := range want {
			if _, ok := got[op]; !ok {
				t.Fatalf("operation %s from journey steps not found in plan nodes %v", op, got)
			}
		}
	})
}

func TestExecute_OutcomeCascadeConditional_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		schema := pbtChainSchema()
		schema.Faults = []topology.FaultSpec{{
			Target:   topology.FaultTarget{Kind: topology.TargetNode, Service: schema.Services["middle"]},
			Kind:     topology.FaultCrash,
			Severity: topology.SeverityParams{Probability: 1, Multiplier: 1},
		}}
		mock := &pbtSynth{}
		engine := journey.NewEngine(schema, schema.ApplyFaults(), mock)
		plan, err := engine.BuildPlan("chain")
		if err != nil {
			t.Fatalf("BuildPlan() error = %v", err)
		}
		if err := engine.Execute(context.Background(), plan); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		for _, span := range mock.snapshot() {
			if span.Outcome.Cascaded {
				if span.Outcome.Success {
					t.Fatalf("cascaded outcome succeeded: %+v", span.Outcome)
				}
				if span.Outcome.EndTime.Sub(span.Input.StartTime) > 2*time.Millisecond {
					t.Fatalf("cascaded duration = %s, want near zero", span.Outcome.EndTime.Sub(span.Input.StartTime))
				}
			}
		}
	})
}

func TestExecute_OutcomeErrorTypeAllowed_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		schema := pbtChainSchema()
		switch rapid.IntRange(0, 3).Draw(t, "fault_case") {
		case 0:
			schema.Faults = []topology.FaultSpec{{
				Target:   topology.FaultTarget{Kind: topology.TargetNode, Service: schema.Services["middle"]},
				Kind:     topology.FaultCrash,
				Severity: topology.SeverityParams{Probability: 1, Multiplier: 1},
			}}
		case 1:
			edge := schema.Services["entry"].Operations["GET /entry"].Calls[0].Edge
			schema.Faults = []topology.FaultSpec{{
				Target:   topology.FaultTarget{Kind: topology.TargetEdge, Edge: edge},
				Kind:     topology.FaultDisconnect,
				Severity: topology.SeverityParams{Probability: 1, Multiplier: 1},
			}}
		case 2:
			op := schema.Services["middle"].Operations["GET /middle"]
			schema.Faults = []topology.FaultSpec{{
				Target:   topology.FaultTarget{Kind: topology.TargetOperation, Operation: op},
				Kind:     topology.FaultErrorRateOverride,
				Severity: topology.SeverityParams{Probability: 1, Multiplier: 1, Value: 1},
			}}
		}

		mock := &pbtSynth{}
		engine := journey.NewEngine(schema, schema.ApplyFaults(), mock)
		plan, err := engine.BuildPlan("chain")
		if err != nil {
			t.Fatalf("BuildPlan() error = %v", err)
		}
		if err := engine.Execute(context.Background(), plan); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		for _, span := range mock.snapshot() {
			if span.Outcome.ErrorType != "" && !allowedJourneyError(span.Outcome.ErrorType) {
				t.Fatalf("ErrorType = %q is not allowed", span.Outcome.ErrorType)
			}
		}
	})
}

func TestExecute_TimeMonotonic_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		schema := pbtChainSchema()
		latency := time.Duration(rapid.IntRange(0, 2).Draw(t, "edge_latency_ms")) * time.Millisecond
		for _, edge := range allPBTEdges(schema) {
			edge.Latency = topology.LatencyDist{Distribution: "fixed", P50: latency, P95: latency}
		}
		mock := &pbtSynth{}
		engine := journey.NewEngine(schema, schema.ApplyFaults(), mock)
		plan, err := engine.BuildPlan("chain")
		if err != nil {
			t.Fatalf("BuildPlan() error = %v", err)
		}
		if err := engine.Execute(context.Background(), plan); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		spans := mock.snapshot()
		if len(spans) == 0 {
			t.Fatal("no spans emitted")
		}
		rootStart := spans[0].Input.StartTime
		for _, span := range spans {
			if span.Outcome.EndTime.Before(span.Input.StartTime) {
				t.Fatalf("span %s ended before it started", span.Input.Operation)
			}
			if span.Input.StartTime.Before(rootStart) {
				t.Fatalf("span %s starts before root", span.Input.Operation)
			}
		}
	})
}

type pbtSynth struct {
	mu    sync.Mutex
	spans []pbtSpan
}

type pbtSpan struct {
	Input   synth.SpanInput
	Outcome synth.Outcome
}

func (m *pbtSynth) BeginSpan(ctx context.Context, in synth.SpanInput) (context.Context, synth.FinishSpanFunc) {
	m.mu.Lock()
	idx := len(m.spans)
	m.spans = append(m.spans, pbtSpan{Input: in})
	m.mu.Unlock()
	return ctx, func(out synth.Outcome) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.spans[idx].Outcome = out
	}
}

func (m *pbtSynth) RecordMetric(context.Context, synth.MetricInput) {}

func (m *pbtSynth) EmitLog(context.Context, synth.LogInput) {}

func (m *pbtSynth) snapshot() []pbtSpan {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]pbtSpan, len(m.spans))
	copy(out, m.spans)
	return out
}

func plansEqual(a, b *journey.Plan) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.JourneyName == b.JourneyName && nodesEqual(a.Root, b.Root)
}

func nodesEqual(a, b *journey.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if opKey(a.Service, a.Operation) != opKey(b.Service, b.Operation) || edgeKey(a.Edge) != edgeKey(b.Edge) {
		return false
	}
	if len(a.Children) != len(b.Children) || len(a.Parallel) != len(b.Parallel) {
		return false
	}
	for i := range a.Children {
		if !nodesEqual(a.Children[i], b.Children[i]) {
			return false
		}
	}
	for i := range a.Parallel {
		if !nodesEqual(a.Parallel[i], b.Parallel[i]) {
			return false
		}
	}
	return true
}

func collectStepOps(steps []*topology.Step, out map[string]struct{}) {
	for _, step := range steps {
		if step == nil {
			continue
		}
		if step.Op != nil {
			out[opKey(step.Op.Service, step.Op.Name)] = struct{}{}
		}
		collectStepOps(step.Parallel, out)
	}
}

func collectPlanOps(node *journey.Node, out map[string]struct{}) {
	if node == nil {
		return
	}
	if node.Service != nil {
		out[opKey(node.Service, node.Operation)] = struct{}{}
	}
	for _, child := range node.Children {
		collectPlanOps(child, out)
	}
	for _, child := range node.Parallel {
		collectPlanOps(child, out)
	}
}

func opKey(svc *topology.Service, op string) string {
	if svc == nil {
		return "." + op
	}
	return string(svc.Name) + "." + op
}

func edgeKey(edge *topology.Edge) string {
	if edge == nil {
		return ""
	}
	return opKey(edge.From.Service, edge.From.Name) + "->" + opKey(edge.To.Service, edge.To.Name)
}

func allowedJourneyError(errorType string) bool {
	for _, allowed := range journey.AllowedErrorTypes {
		if errorType == allowed {
			return true
		}
	}
	return false
}

func pbtChainSchema() *topology.Schema {
	entrySvc := &topology.Service{Name: "entry", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	middleSvc := &topology.Service{Name: "middle", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	leafSvc := &topology.Service{Name: "leaf", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	entry := &topology.Operation{Name: "GET /entry", Service: entrySvc}
	middle := &topology.Operation{Name: "GET /middle", Service: middleSvc}
	leaf := &topology.Operation{Name: "GET /leaf", Service: leafSvc}
	entrySvc.Operations[entry.Name] = entry
	middleSvc.Operations[middle.Name] = middle
	leafSvc.Operations[leaf.Name] = leaf
	entryToMiddle := &topology.Edge{From: entry, To: middle, Protocol: topology.ProtocolHTTP, Latency: topology.LatencyDist{Distribution: "fixed"}}
	middleToLeaf := &topology.Edge{From: middle, To: leaf, Protocol: topology.ProtocolHTTP, Latency: topology.LatencyDist{Distribution: "fixed"}}
	entry.Calls = []*topology.CallNode{{Edge: entryToMiddle}}
	middle.Calls = []*topology.CallNode{{Edge: middleToLeaf}}
	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{
			entrySvc.Name:  entrySvc,
			middleSvc.Name: middleSvc,
			leafSvc.Name:   leafSvc,
		},
		Journeys: map[string]*topology.Journey{
			"chain": {Name: "chain", Steps: []*topology.Step{{Op: entry}}, Weight: 1},
		},
	}
}

func allPBTEdges(schema *topology.Schema) []*topology.Edge {
	edges := make([]*topology.Edge, 0)
	for _, svc := range schema.Services {
		for _, op := range svc.Operations {
			for _, call := range op.Calls {
				if call.Edge != nil {
					edges = append(edges, call.Edge)
				}
			}
		}
	}
	return edges
}
