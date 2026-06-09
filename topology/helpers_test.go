package topology_test

import (
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestEnumStringUnknowns(t *testing.T) {
	t.Parallel()

	if got := topology.TargetNode.String(); got != "node" {
		t.Fatalf("TargetNode.String() = %q", got)
	}
	if got := topology.TargetOperation.String(); got != "operation" {
		t.Fatalf("TargetOperation.String() = %q", got)
	}
	if got := topology.TargetEdge.String(); got != "edge" {
		t.Fatalf("TargetEdge.String() = %q", got)
	}
	if got := topology.TargetKind(99).String(); got != "unknown" {
		t.Fatalf("unknown TargetKind.String() = %q", got)
	}
	if got := topology.ExhaustedAction(99).String(); got != "unknown" {
		t.Fatalf("unknown ExhaustedAction.String() = %q", got)
	}
}

func TestFaultOverlayNilLookups(t *testing.T) {
	t.Parallel()

	var overlay *topology.FaultOverlay
	if overlay.NodeFaults(nil) != nil || overlay.OperationFaults(nil) != nil || overlay.EdgeFaults(nil) != nil {
		t.Fatal("nil overlay returned non-nil faults")
	}
	if topology.FaultOverlayEqual(nil, (&topology.Schema{}).ApplyFaults()) {
		t.Fatal("nil overlay unexpectedly equals empty overlay")
	}
}

func TestEqualNilSchemas(t *testing.T) {
	t.Parallel()

	if !topology.Equal(nil, nil) {
		t.Fatal("Equal(nil, nil) = false")
	}
	if topology.Equal(nil, validManualSchema()) {
		t.Fatal("Equal(nil, schema) = true")
	}
}
