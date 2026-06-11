// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"errors"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestExecute_NilArgs_ReturnsError(t *testing.T) {
	t.Parallel()

	engine, plan, _ := newExecutablePlan(t, newTestSchema(t))
	//nolint:staticcheck // Execute explicitly accepts nil to return ExecuteError{Kind:"nil_ctx"}.
	if err := engine.Execute(nil, plan); executeErrorKind(err) != "nil_ctx" {
		t.Fatalf("Execute(nil, plan) error = %v, want nil_ctx", err)
	}
	if err := engine.Execute(context.Background(), nil); executeErrorKind(err) != "nil_plan" {
		t.Fatalf("Execute(ctx, nil) error = %v, want nil_plan", err)
	}
}

func TestExecute_Sequential_HappyPath(t *testing.T) {
	t.Parallel()

	engine, plan, mock := newExecutablePlan(t, newTestSchema(t))
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	spans := mock.snapshotSpans()
	if len(spans) != 3 {
		t.Fatalf("span count = %d, want 3", len(spans))
	}
	wantOps := []string{"GET /checkout", "POST /charge", "INSERT payment"}
	for i, want := range wantOps {
		if spans[i].Input.Operation != want {
			t.Fatalf("span[%d].operation = %q, want %q", i, spans[i].Input.Operation, want)
		}
		if !spans[i].Outcome.Success {
			t.Fatalf("span[%d].Outcome.Success = false, want true", i)
		}
	}
	if len(mock.snapshotMetrics()) != 3 {
		t.Fatalf("metric count = %d, want 3", len(mock.snapshotMetrics()))
	}
	if len(mock.snapshotLogs()) != 3 {
		t.Fatalf("log count = %d, want 3", len(mock.snapshotLogs()))
	}
}

func TestExecute_Parallel_HappyPath(t *testing.T) {
	t.Parallel()

	engine, plan, mock := newExecutablePlan(t, newParallelTestSchema())
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	gotOps := map[string]bool{}
	for _, span := range mock.snapshotSpans() {
		gotOps[span.Input.Operation] = span.Outcome.Success
	}
	if !gotOps["GET /left"] || !gotOps["GET /right"] {
		t.Fatalf("parallel spans = %v, want successful left and right branches", gotOps)
	}
}

func TestExecute_CtxCancel_AlreadyCanceled(t *testing.T) {
	t.Parallel()

	engine, plan, mock := newExecutablePlan(t, newSingleStepSchema())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := engine.Execute(ctx, plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	spans := mock.snapshotSpans()
	if len(spans) != 1 {
		t.Fatalf("span count = %d, want 1", len(spans))
	}
	if spans[0].Outcome.ErrorType != "context_canceled" {
		t.Fatalf("ErrorType = %q, want context_canceled", spans[0].Outcome.ErrorType)
	}
}

func TestExecute_PanicInSynth_RecoversAndReturns(t *testing.T) {
	t.Parallel()

	schema := newSingleStepSchema()
	engine := NewEngine(schema, schema.ApplyFaults(), panicBeginSynth{})
	plan, err := engine.BuildPlan("single")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	err = engine.Execute(context.Background(), plan)
	var executeErr *ExecuteError
	if !errors.As(err, &executeErr) {
		t.Fatalf("Execute() error = %T, want *ExecuteError", err)
	}
	if executeErr.Kind != "internal" {
		t.Fatalf("ExecuteError.Kind = %q, want internal", executeErr.Kind)
	}
}

func TestExecute_FaultCrash_FailureOutcome(t *testing.T) {
	t.Parallel()

	schema := newSingleStepSchema()
	svc := schema.Services["api"]
	engine, plan, mock := newExecutablePlanWithFaults(t, schema, faultOnService(svc, topology.FaultCrash, 1, 0, 0))
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	outcome := mock.snapshotSpans()[0].Outcome
	if outcome.Success || outcome.ErrorType != "crashed" {
		t.Fatalf("Outcome = %+v, want crashed failure", outcome)
	}
}

func TestExecute_FaultDisconnect_FailureOutcome(t *testing.T) {
	t.Parallel()

	schema := newTestSchema(t)
	edge := schema.Services["api"].Operations["GET /checkout"].Calls[0].Edge
	engine, plan, mock := newExecutablePlanWithFaults(t, schema, faultOnEdge(edge, topology.FaultDisconnect, 1, 0, 0))
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !hasSpanOutcome(mock.snapshotSpans(), "connection_refused") {
		t.Fatalf("spans = %+v, want connection_refused outcome", mock.snapshotSpans())
	}
}

func TestExecute_FaultErrorRate_PrimaryFailureForced(t *testing.T) {
	t.Parallel()

	schema := newTestSchema(t)
	op := schema.Services["api"].Operations["GET /checkout"]
	engine, plan, mock := newExecutablePlanWithFaults(t, schema, faultOnOperation(op, topology.FaultErrorRateOverride, 1, 1, 0))
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	outcome := mock.snapshotSpans()[0].Outcome
	if outcome.Success || outcome.ErrorType != "http.500" {
		t.Fatalf("root Outcome = %+v, want forced http.500 failure", outcome)
	}
}

func newExecutablePlan(t *testing.T, schema *topology.Schema) (*Engine, *Plan, *mockSynth) {
	t.Helper()
	return newExecutablePlanWithFaults(t, schema)
}

func newExecutablePlanWithFaults(t *testing.T, schema *topology.Schema, faults ...topology.FaultSpec) (*Engine, *Plan, *mockSynth) {
	t.Helper()
	mock := newMockSynth()
	engine := NewEngine(schema, newTestOverlay(t, schema, faults...), mock)
	name := firstJourneyName(schema)
	plan, err := engine.BuildPlan(name)
	if err != nil {
		t.Fatalf("BuildPlan(%q) error = %v", name, err)
	}
	return engine, plan, mock
}

func executeErrorKind(err error) string {
	var executeErr *ExecuteError
	if errors.As(err, &executeErr) {
		return executeErr.Kind
	}
	return ""
}

func hasSpanOutcome(spans []spanCall, errorType string) bool {
	for _, span := range spans {
		if span.Outcome.ErrorType == errorType {
			return true
		}
	}
	return false
}

func firstJourneyName(schema *topology.Schema) string {
	for name := range schema.Journeys {
		return name
	}
	return ""
}

func newSingleStepSchema() *topology.Schema {
	svc := &topology.Service{
		Name:       "api",
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: make(map[string]*topology.Operation),
	}
	op := &topology.Operation{Name: "GET /single", Service: svc}
	svc.Operations[op.Name] = op
	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{svc.Name: svc},
		Journeys: map[string]*topology.Journey{
			"single": {Name: "single", Steps: []*topology.Step{{Op: op}}, Weight: 1},
		},
	}
}

func newParallelTestSchema() *topology.Schema {
	leftSvc := &topology.Service{Name: "left", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	rightSvc := &topology.Service{Name: "right", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	left := &topology.Operation{Name: "GET /left", Service: leftSvc}
	right := &topology.Operation{Name: "GET /right", Service: rightSvc}
	leftSvc.Operations[left.Name] = left
	rightSvc.Operations[right.Name] = right
	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{
			leftSvc.Name:  leftSvc,
			rightSvc.Name: rightSvc,
		},
		Journeys: map[string]*topology.Journey{
			"parallel": {
				Name: "parallel",
				Steps: []*topology.Step{{
					Parallel: []*topology.Step{{Op: left}, {Op: right}},
				}},
				Weight: 1,
			},
		},
	}
}
