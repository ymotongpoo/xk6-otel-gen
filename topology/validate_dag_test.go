package topology_test

import (
	"strings"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func TestValidate_AlwaysDAG(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := generators.ValidSchema().Draw(t, "schema")
		if err := topology.Validate(s); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
		if !operationGraphIsDAG(s) {
			t.Fatal("valid schema operation graph is cyclic")
		}
	})
}

func TestValidate_DetectsCycle(t *testing.T) {
	t.Parallel()

	s := validManualSchema()
	edge := firstEdge(s)
	edge.To.Calls = []*topology.CallNode{{
		Edge: &topology.Edge{
			From:         edge.To,
			To:           edge.From,
			Protocol:     topology.ProtocolHTTP,
			Latency:      topology.LatencyDist{Distribution: "constant"},
			RetryBackoff: topology.BackoffExponential,
		},
	}}

	err := topology.Validate(s)
	if err == nil {
		t.Fatal("Validate() error = nil, want cycle")
	}
	msg := err.Error()
	for _, want := range []string{"R-STR-4", "frontend.GET /", "backend.Fetch"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("cycle error %q does not contain %q", msg, want)
		}
	}
}

func operationGraphIsDAG(s *topology.Schema) bool {
	ops := allOperations(s)
	visiting := make(map[*topology.Operation]bool, len(ops))
	visited := make(map[*topology.Operation]bool, len(ops))
	var visit func(*topology.Operation) bool
	visit = func(op *topology.Operation) bool {
		if visiting[op] {
			return false
		}
		if visited[op] {
			return true
		}
		visiting[op] = true
		for _, edge := range edgesInCalls(op.Calls) {
			if edge != nil && edge.To != nil && !visit(edge.To) {
				return false
			}
		}
		visiting[op] = false
		visited[op] = true
		return true
	}
	for _, op := range ops {
		if !visit(op) {
			return false
		}
	}
	return true
}
