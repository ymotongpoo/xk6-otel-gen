package journey

import (
	"context"
	"fmt"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func BenchmarkBuildPlan_Typical(b *testing.B) {
	schema := benchmarkSchema(15, 1)
	engine := NewEngine(schema, schema.ApplyFaults(), newMockSynth())
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := engine.BuildPlan("journey-0"); err != nil {
			b.Fatalf("BuildPlan() error = %v", err)
		}
	}
}

func BenchmarkExecute_PureOverhead(b *testing.B) {
	const steps = 15
	schema := benchmarkSchema(steps, 1)
	engine := NewEngine(schema, schema.ApplyFaults(), newMockSynth())
	plan := benchmarkZeroLatencyPlan(steps)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := engine.Execute(ctx, plan); err != nil {
			b.Fatalf("Execute() error = %v", err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*steps), "ns/step")
}

func BenchmarkListJourneys(b *testing.B) {
	schema := benchmarkSchema(3, 5)
	engine := NewEngine(schema, schema.ApplyFaults(), newMockSynth())
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = engine.ListJourneys()
	}
}

func benchmarkSchema(ops, journeys int) *topology.Schema {
	if ops < 1 {
		ops = 1
	}
	service := &topology.Service{
		Name:       "bench",
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: make(map[string]*topology.Operation, ops),
	}
	operations := make([]*topology.Operation, 0, ops)
	for i := 0; i < ops; i++ {
		op := &topology.Operation{Name: fmt.Sprintf("GET /op/%02d", i), Service: service}
		service.Operations[op.Name] = op
		operations = append(operations, op)
	}
	for i := 0; i < len(operations)-1; i++ {
		edge := &topology.Edge{
			From:     operations[i],
			To:       operations[i+1],
			Protocol: topology.ProtocolHTTP,
			Latency:  topology.LatencyDist{Distribution: "fixed"},
		}
		operations[i].Calls = []*topology.CallNode{{Edge: edge}}
	}

	schema := &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{service.Name: service},
		Journeys: make(map[string]*topology.Journey, journeys),
	}
	for i := 0; i < journeys; i++ {
		name := fmt.Sprintf("journey-%d", i)
		schema.Journeys[name] = &topology.Journey{Name: name, Steps: []*topology.Step{{Op: operations[0]}}, Weight: 1}
	}
	return schema
}

func benchmarkZeroLatencyPlan(steps int) *Plan {
	service := &topology.Service{
		Name:       "bench",
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: make(map[string]*topology.Operation, steps),
	}
	var root *Node
	var prev *Node
	dummyFrom := &topology.Operation{Name: "external"}
	for i := 0; i < steps; i++ {
		op := &topology.Operation{Name: fmt.Sprintf("GET /op/%02d", i), Service: service}
		service.Operations[op.Name] = op
		edge := &topology.Edge{
			From:     dummyFrom,
			To:       op,
			Protocol: topology.ProtocolHTTP,
			Latency:  topology.LatencyDist{Distribution: "fixed"},
		}
		node := &Node{Service: service, Operation: op.Name, Edge: edge}
		if root == nil {
			root = node
		}
		if prev != nil {
			prev.Children = []*Node{node}
		}
		prev = node
	}
	return &Plan{JourneyName: "bench", Root: root}
}
