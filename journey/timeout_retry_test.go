// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestExecute_EdgeTimeout_ClampsDurationAndFailsHTTP(t *testing.T) {
	t.Parallel()

	schema, edge := timeoutRetrySchema(200*time.Millisecond, 50*time.Millisecond)
	engine, plan, mock := newExecutablePlanForSchema(t, schema)
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	span := requireSpanForEdge(t, mock.snapshotSpans(), edge)
	if span.Outcome.Success {
		t.Fatalf("Outcome.Success = true, want false")
	}
	if span.Outcome.ErrorType != "timeout" || span.Outcome.StatusCode != 504 {
		t.Fatalf("Outcome = %+v, want timeout 504", span.Outcome)
	}
	if got := span.Outcome.EndTime.Sub(span.Input.StartTime); got != 50*time.Millisecond {
		t.Fatalf("duration = %s, want 50ms", got)
	}
}

func TestExecute_EdgeTimeout_BelowThresholdUnaffected(t *testing.T) {
	t.Parallel()

	schema, edge := timeoutRetrySchema(20*time.Millisecond, 50*time.Millisecond)
	engine, plan, mock := newExecutablePlanForSchema(t, schema)
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	span := requireSpanForEdge(t, mock.snapshotSpans(), edge)
	if !span.Outcome.Success {
		t.Fatalf("Outcome.Success = false, want true: %+v", span.Outcome)
	}
	if got := span.Outcome.EndTime.Sub(span.Input.StartTime); got != 20*time.Millisecond {
		t.Fatalf("duration = %s, want 20ms", got)
	}
}

func TestExecute_RetryBackoff_ProducesAttemptSpans(t *testing.T) {
	t.Parallel()

	schema, edge := timeoutRetrySchema(10*time.Millisecond, 0)
	edge.ErrorRate = 1
	edge.Retries = 2
	edge.RetryBackoff = topology.BackoffLinear
	edge.RetryBaseDelay = 100 * time.Millisecond
	engine, plan, mock := newExecutablePlanForSchema(t, schema)
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	attempts := spansForEdge(mock.snapshotSpans(), edge)
	if len(attempts) != 3 {
		t.Fatalf("attempt spans = %d, want 3", len(attempts))
	}
	if gap := attempts[1].Input.StartTime.Sub(attempts[0].Input.StartTime); gap != 110*time.Millisecond {
		t.Fatalf("attempt 2 start gap = %s, want 110ms", gap)
	}
	if gap := attempts[2].Input.StartTime.Sub(attempts[1].Input.StartTime); gap != 210*time.Millisecond {
		t.Fatalf("attempt 3 start gap = %s, want 210ms", gap)
	}
	root, ok := findSpanByOperation(mock.snapshotSpans(), "GET /root")
	if !ok {
		t.Fatal("root span not found")
	}
	if got := root.Outcome.EndTime.Sub(root.Input.StartTime); got < 230*time.Millisecond {
		t.Fatalf("root duration = %s, want at least retry total 230ms", got)
	}
}

func newExecutablePlanForSchema(t *testing.T, schema *topology.Schema) (*Engine, *Plan, *mockSynth) {
	t.Helper()

	mock := newMockSynth()
	engine := NewEngineWithSeed(schema, schema.ApplyFaults(), mock, 1)
	plan, err := engine.BuildPlan("flow")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	return engine, plan, mock
}

func timeoutRetrySchema(latency, timeout time.Duration) (*topology.Schema, *topology.Edge) {
	rootSvc := &topology.Service{Name: "root", Namespace: topology.DefaultNamespace, Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	depSvc := &topology.Service{Name: "dep", Namespace: topology.DefaultNamespace, Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	root := &topology.Operation{Name: "GET /root", Service: rootSvc}
	dep := &topology.Operation{Name: "GET /dep", Service: depSvc}
	rootSvc.Operations[root.Name] = root
	depSvc.Operations[dep.Name] = dep
	edge := &topology.Edge{
		From:           root,
		To:             dep,
		Protocol:       topology.ProtocolHTTP,
		Latency:        topology.LatencyDist{Distribution: "fixed", P50: latency, P95: latency},
		Timeout:        timeout,
		RetryBackoff:   topology.BackoffExponential,
		RetryBaseDelay: topology.DefaultRetryBaseDelay,
	}
	root.Calls = []*topology.CallNode{{Edge: edge}}
	return &topology.Schema{
		Namespace: topology.DefaultNamespace,
		Services: map[topology.ServiceID]*topology.Service{
			rootSvc.Name: rootSvc,
			depSvc.Name:  depSvc,
		},
		Journeys: map[string]*topology.Journey{
			"flow": {Name: "flow", Steps: []*topology.Step{{Op: root}}, Weight: 1},
		},
	}, edge
}

func requireSpanForEdge(t *testing.T, spans []spanCall, edge *topology.Edge) spanCall {
	t.Helper()

	matches := spansForEdge(spans, edge)
	if len(matches) == 0 {
		t.Fatalf("no span found for edge %p in %+v", edge, spans)
	}
	return matches[0]
}

func spansForEdge(spans []spanCall, edge *topology.Edge) []spanCall {
	var out []spanCall
	for _, span := range spans {
		if span.Input.Edge == edge {
			out = append(out, span)
		}
	}
	return out
}
