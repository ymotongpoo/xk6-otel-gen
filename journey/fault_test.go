// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestFoldFaults_Crash(t *testing.T) {
	t.Parallel()

	engine, node := newFaultTestEngine(t, func(schema *topology.Schema) []topology.FaultSpec {
		return []topology.FaultSpec{faultOnService(schema.Services["api"], topology.FaultCrash, 1, 0, 0)}
	})

	if ff := engine.impl.foldFaults(node); !ff.crashed {
		t.Fatalf("foldFaults().crashed = false, want true")
	}
}

func TestFoldFaults_Disconnect(t *testing.T) {
	t.Parallel()

	engine, node := newFaultTestEngine(t, func(schema *topology.Schema) []topology.FaultSpec {
		edge := schema.Services["api"].Operations["GET /checkout"].Calls[0].Edge
		return []topology.FaultSpec{faultOnEdge(edge, topology.FaultDisconnect, 1, 0, 0)}
	})
	child := node.Children[0]

	if ff := engine.impl.foldFaults(child); !ff.disconnected {
		t.Fatalf("foldFaults().disconnected = false, want true")
	}
}

func TestFoldFaults_ErrorRate(t *testing.T) {
	t.Parallel()

	engine, node := newFaultTestEngine(t, func(schema *topology.Schema) []topology.FaultSpec {
		op := schema.Services["api"].Operations["GET /checkout"]
		return []topology.FaultSpec{faultOnOperation(op, topology.FaultErrorRateOverride, 1, 0.75, 0)}
	})

	ff := engine.impl.foldFaults(node)
	if ff.errorRate != 0.75 {
		t.Fatalf("foldFaults().errorRate = %f, want 0.75", ff.errorRate)
	}
	if ff.errorType == "" {
		t.Fatal("foldFaults().errorType is empty")
	}
}

func TestFoldFaults_LatencyInflation_Accumulates(t *testing.T) {
	t.Parallel()

	engine, node := newFaultTestEngine(t, func(schema *topology.Schema) []topology.FaultSpec {
		payments := schema.Services["payments"]
		op := payments.Operations["POST /charge"]
		edge := schema.Services["api"].Operations["GET /checkout"].Calls[0].Edge
		return []topology.FaultSpec{
			faultOnService(payments, topology.FaultLatencyInflation, 1, 0, 5*time.Millisecond),
			faultOnOperation(op, topology.FaultLatencyInflation, 1, 0, 7*time.Millisecond),
			faultOnEdge(edge, topology.FaultLatencyInflation, 1, 0, 11*time.Millisecond),
		}
	})
	child := node.Children[0]

	ff := engine.impl.foldFaults(child)
	want := 5*time.Millisecond + 7*time.Millisecond + 11*time.Millisecond
	if ff.latencyInflate != want {
		t.Fatalf("child latencyInflate = %s, want %s", ff.latencyInflate, want)
	}
}

func TestFoldFaults_Precedence(t *testing.T) {
	t.Parallel()

	engine, node := newFaultTestEngine(t, func(schema *topology.Schema) []topology.FaultSpec {
		api := schema.Services["api"]
		op := api.Operations["GET /checkout"]
		edge := op.Calls[0].Edge
		return []topology.FaultSpec{
			faultOnService(api, topology.FaultCrash, 1, 0, 0),
			faultOnService(api, topology.FaultLatencyInflation, 1, 0, time.Millisecond),
			faultOnOperation(op, topology.FaultErrorRateOverride, 1, 1, 0),
			faultOnEdge(edge, topology.FaultDisconnect, 1, 0, 0),
			faultOnEdge(edge, topology.FaultLatencyInflation, 1, 0, time.Millisecond),
		}
	})

	rootFF := engine.impl.foldFaults(node)
	if !rootFF.crashed || rootFF.errorRate != 1 || rootFF.latencyInflate != time.Millisecond {
		t.Fatalf("root foldedFault = %+v, want crash + error rate + latency", rootFF)
	}
	childFF := engine.impl.foldFaults(node.Children[0])
	if !childFF.disconnected || childFF.latencyInflate != time.Millisecond {
		t.Fatalf("child foldedFault = %+v, want disconnect + edge latency", childFF)
	}
}

func TestSampleEdgeLatency_NilEdge_Default(t *testing.T) {
	t.Parallel()

	impl := &engineImpl{rand: newDefaultRand()}
	if got := impl.sampleEdgeLatency(nil); got != defaultEntryLatency {
		t.Fatalf("sampleEdgeLatency(nil) = %s, want %s", got, defaultEntryLatency)
	}
}

func TestSampleEdgeLatency_Fixed(t *testing.T) {
	t.Parallel()

	impl := &engineImpl{rand: newDefaultRand()}
	for _, dist := range []string{"", "fixed"} {
		edge := &topology.Edge{Latency: topology.LatencyDist{Distribution: dist, P50: 23 * time.Millisecond, P95: 99 * time.Millisecond}}
		if got := impl.sampleEdgeLatency(edge); got != 23*time.Millisecond {
			t.Fatalf("sampleEdgeLatency(%q) = %s, want 23ms", dist, got)
		}
	}
}

func TestSampleEdgeLatency_Lognormal_InRange(t *testing.T) {
	t.Parallel()

	impl := &engineImpl{rand: newDefaultRand()}
	edge := &topology.Edge{Latency: topology.LatencyDist{
		Distribution: "lognormal",
		P50:          10 * time.Millisecond,
		P95:          50 * time.Millisecond,
	}}
	var total time.Duration
	for i := 0; i < 1000; i++ {
		got := impl.sampleEdgeLatency(edge)
		if got <= 0 {
			t.Fatalf("sampleEdgeLatency(lognormal) = %s, want positive", got)
		}
		total += got
	}
	avg := total / 1000
	if avg < 5*time.Millisecond || avg > 100*time.Millisecond {
		t.Fatalf("average lognormal sample = %s, want clustered around configured latency", avg)
	}
}

func newFaultTestEngine(t *testing.T, faults func(*topology.Schema) []topology.FaultSpec) (*Engine, *Node) {
	t.Helper()

	schema := newPlanTestSchema()
	schema.Faults = faults(schema)
	engine := NewEngine(schema, schema.ApplyFaults(), phase2Synth{})
	plan, err := engine.BuildPlan("checkout")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	return engine, plan.Root
}

func faultOnService(svc *topology.Service, kind topology.FaultKind, probability, value float64, add time.Duration) topology.FaultSpec {
	return topology.FaultSpec{
		Target: topology.FaultTarget{Kind: topology.TargetNode, Service: svc},
		Kind:   kind,
		Severity: topology.SeverityParams{
			Probability: probability,
			Multiplier:  1,
			Value:       value,
			Add:         add,
		},
	}
}

func faultOnOperation(op *topology.Operation, kind topology.FaultKind, probability, value float64, add time.Duration) topology.FaultSpec {
	return topology.FaultSpec{
		Target: topology.FaultTarget{Kind: topology.TargetOperation, Operation: op},
		Kind:   kind,
		Severity: topology.SeverityParams{
			Probability: probability,
			Multiplier:  1,
			Value:       value,
			Add:         add,
		},
	}
}

func faultOnEdge(edge *topology.Edge, kind topology.FaultKind, probability, value float64, add time.Duration) topology.FaultSpec {
	return topology.FaultSpec{
		Target: topology.FaultTarget{Kind: topology.TargetEdge, Edge: edge},
		Kind:   kind,
		Severity: topology.SeverityParams{
			Probability: probability,
			Multiplier:  1,
			Value:       value,
			Add:         add,
		},
	}
}
