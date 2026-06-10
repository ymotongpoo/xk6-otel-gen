package journey

import (
	"context"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestApplyRecovery_FirstFallbackSucceeds(t *testing.T) {
	t.Parallel()

	engine, node, fallback := newRecoveryFixture(t, topology.ExhaustedPropagate)
	outcome := engine.impl.applyRecovery(context.Background(), node, primaryFailureOutcome())

	if !outcome.Success {
		t.Fatalf("Outcome.Success = false, want true")
	}
	if outcome.FallbackUsed != fallback {
		t.Fatalf("FallbackUsed = %p, want %p", outcome.FallbackUsed, fallback)
	}
	if len(outcome.FallbackAttempts) != 0 {
		t.Fatalf("FallbackAttempts = %d, want 0", len(outcome.FallbackAttempts))
	}
	if !outcome.PrimaryFailed {
		t.Fatal("PrimaryFailed = false, want true")
	}
}

func TestApplyRecovery_AllFallbacksFail_Propagate(t *testing.T) {
	t.Parallel()

	engine, node, _ := newRecoveryFixture(t, topology.ExhaustedPropagate, failFallbacks())
	outcome := engine.impl.applyRecovery(context.Background(), node, primaryFailureOutcome())

	if outcome.Success {
		t.Fatalf("Outcome.Success = true, want false")
	}
	if outcome.ErrorType != "http.500" {
		t.Fatalf("ErrorType = %q, want http.500", outcome.ErrorType)
	}
	if len(outcome.FallbackAttempts) != 2 {
		t.Fatalf("FallbackAttempts = %d, want 2", len(outcome.FallbackAttempts))
	}
}

func TestApplyRecovery_AllFallbacksFail_ReturnDefault(t *testing.T) {
	t.Parallel()

	engine, node, _ := newRecoveryFixture(t, topology.ExhaustedReturnDefault, failFallbacks())
	outcome := engine.impl.applyRecovery(context.Background(), node, primaryFailureOutcome())

	if !outcome.Success || !outcome.DefaultUsed {
		t.Fatalf("Outcome = %+v, want successful default", outcome)
	}
	if outcome.ErrorType != "" {
		t.Fatalf("ErrorType = %q, want empty", outcome.ErrorType)
	}
}

func TestApplyRecovery_AllFallbacksFail_SucceedSilently(t *testing.T) {
	t.Parallel()

	engine, node, _ := newRecoveryFixture(t, topology.ExhaustedSucceedSilently, failFallbacks())
	outcome := engine.impl.applyRecovery(context.Background(), node, primaryFailureOutcome())

	if !outcome.Success || !outcome.SilentlySucceeded {
		t.Fatalf("Outcome = %+v, want silent success", outcome)
	}
	if outcome.ErrorType != "" {
		t.Fatalf("ErrorType = %q, want empty", outcome.ErrorType)
	}
}

func TestExecute_CascadeChildSpan_EmittedWithAttribute(t *testing.T) {
	t.Parallel()

	schema := newTestSchema(t)
	payments := schema.Services["payments"]
	engine, plan, mock := newExecutablePlanWithFaults(t, schema, faultOnService(payments, topology.FaultCrash, 1, 0, 0))
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	cascade, ok := findSpanByOperation(mock.snapshotSpans(), "INSERT payment")
	if !ok {
		t.Fatalf("spans = %+v, want INSERT payment cascade span", mock.snapshotSpans())
	}
	if !cascade.Outcome.Cascaded || cascade.Outcome.Success {
		t.Fatalf("cascade Outcome = %+v, want cascaded failure", cascade.Outcome)
	}
}

func TestExecute_CascadeChildSpan_NearZeroDuration(t *testing.T) {
	t.Parallel()

	schema := newTestSchema(t)
	payments := schema.Services["payments"]
	engine, plan, mock := newExecutablePlanWithFaults(t, schema, faultOnService(payments, topology.FaultCrash, 1, 0, 0))
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	cascade, ok := findSpanByOperation(mock.snapshotSpans(), "INSERT payment")
	if !ok {
		t.Fatal("cascade span not found")
	}
	if duration := cascade.Outcome.EndTime.Sub(cascade.Input.StartTime); duration > 2*time.Millisecond {
		t.Fatalf("cascade span duration = %s, want near zero", duration)
	}
}

type recoveryFixtureOption func(*topology.Schema)

func failFallbacks() recoveryFixtureOption {
	return func(schema *topology.Schema) {
		schema.Faults = append(schema.Faults,
			faultOnService(schema.Services["fallback-a"], topology.FaultCrash, 1, 0, 0),
			faultOnService(schema.Services["fallback-b"], topology.FaultCrash, 1, 0, 0),
		)
	}
}

func newRecoveryFixture(t *testing.T, action topology.ExhaustedAction, opts ...recoveryFixtureOption) (*Engine, *Node, *topology.Edge) {
	t.Helper()

	schema, fallback := newRecoverySchema(action)
	for _, opt := range opts {
		opt(schema)
	}
	mock := newMockSynth()
	engine := NewEngine(schema, schema.ApplyFaults(), mock)
	plan, err := engine.BuildPlan("recover")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if len(plan.Root.Children) != 1 {
		t.Fatalf("root children = %d, want 1", len(plan.Root.Children))
	}
	return engine, plan.Root.Children[0], fallback
}

func primaryFailureOutcome() Outcome {
	return Outcome{Success: false, StatusCode: 500, ErrorType: "http.500", Latency: time.Millisecond}
}

func findSpanByOperation(spans []spanCall, op string) (spanCall, bool) {
	for _, span := range spans {
		if span.Input.Operation == op {
			return span, true
		}
	}
	return spanCall{}, false
}

func newRecoverySchema(action topology.ExhaustedAction) (*topology.Schema, *topology.Edge) {
	callerSvc := &topology.Service{Name: "caller", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	primarySvc := &topology.Service{Name: "primary", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	fallbackASvc := &topology.Service{Name: "fallback-a", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	fallbackBSvc := &topology.Service{Name: "fallback-b", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}

	caller := &topology.Operation{Name: "GET /caller", Service: callerSvc}
	primary := &topology.Operation{Name: "GET /primary", Service: primarySvc}
	fallbackA := &topology.Operation{Name: "GET /fallback-a", Service: fallbackASvc}
	fallbackB := &topology.Operation{Name: "GET /fallback-b", Service: fallbackBSvc}
	callerSvc.Operations[caller.Name] = caller
	primarySvc.Operations[primary.Name] = primary
	fallbackASvc.Operations[fallbackA.Name] = fallbackA
	fallbackBSvc.Operations[fallbackB.Name] = fallbackB

	fallbackEdgeA := &topology.Edge{From: caller, To: fallbackA, Protocol: topology.ProtocolHTTP, Latency: topology.LatencyDist{Distribution: "fixed"}}
	fallbackEdgeB := &topology.Edge{From: caller, To: fallbackB, Protocol: topology.ProtocolHTTP, Latency: topology.LatencyDist{Distribution: "fixed"}}
	primaryEdge := &topology.Edge{
		From:     caller,
		To:       primary,
		Protocol: topology.ProtocolHTTP,
		Latency:  topology.LatencyDist{Distribution: "fixed"},
		OnFailure: &topology.RecoveryPolicy{
			Fallback:    []*topology.Edge{fallbackEdgeA, fallbackEdgeB},
			OnExhausted: action,
		},
	}
	caller.Calls = []*topology.CallNode{{Edge: primaryEdge}}

	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{
			callerSvc.Name:    callerSvc,
			primarySvc.Name:   primarySvc,
			fallbackASvc.Name: fallbackASvc,
			fallbackBSvc.Name: fallbackBSvc,
		},
		Journeys: map[string]*topology.Journey{
			"recover": {Name: "recover", Steps: []*topology.Step{{Op: caller}}, Weight: 1},
		},
	}, fallbackEdgeA
}
