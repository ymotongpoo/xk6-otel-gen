package topology_test

import (
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func TestApplyFaults_OverlayCovers(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := generators.ValidSchema().Draw(t, "schema")
		overlay := s.ApplyFaults()
		covered := 0
		for _, fault := range s.Faults {
			var faults []topology.FaultSpec
			switch fault.Target.Kind {
			case topology.TargetNode:
				faults = overlay.NodeFaults(fault.Target.Service)
			case topology.TargetOperation:
				faults = overlay.OperationFaults(fault.Target.Operation)
			case topology.TargetEdge:
				faults = overlay.EdgeFaults(fault.Target.Edge)
			}
			if !containsFaultSpec(faults, fault) {
				t.Fatalf("overlay does not contain fault %+v", fault)
			}
		}
		for _, svc := range s.Services {
			covered += len(overlay.NodeFaults(svc))
			for _, op := range svc.Operations {
				covered += len(overlay.OperationFaults(op))
			}
		}
		for _, edge := range allEdges(s) {
			covered += len(overlay.EdgeFaults(edge))
		}
		if covered != len(s.Faults) {
			t.Fatalf("overlay covers %d faults, want %d", covered, len(s.Faults))
		}
	})
}

func TestApplyFaults_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := generators.ValidSchema().Draw(t, "schema")
		if !topology.FaultOverlayEqual(s.ApplyFaults(), s.ApplyFaults()) {
			t.Fatal("ApplyFaults() is not idempotent")
		}
	})
}

func containsFaultSpec(faults []topology.FaultSpec, want topology.FaultSpec) bool {
	for _, got := range faults {
		if got.Kind == want.Kind &&
			got.Severity == want.Severity &&
			got.Target.Kind == want.Target.Kind &&
			got.Target.Service == want.Target.Service &&
			got.Target.Operation == want.Target.Operation &&
			got.Target.Edge == want.Target.Edge {
			return true
		}
	}
	return false
}
