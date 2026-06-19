// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

const (
	minimalPath   = "../../examples/minimal/topology.yaml"
	astroshopPath = "../../examples/astroshop/topology.yaml"
)

func TestBuildVizData_MinimalTopology(t *testing.T) {
	t.Parallel()

	schema, err := topology.ParseFile(minimalPath)
	if err != nil {
		t.Fatalf("ParseFile(%q): %v", minimalPath, err)
	}

	data := buildVizData(schema)

	if len(data.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3", len(data.Nodes))
	}
	if len(data.Edges) != 2 {
		t.Fatalf("edges = %d, want 2", len(data.Edges))
	}
	if len(data.Journeys) != 1 {
		t.Fatalf("journeys = %d, want 1", len(data.Journeys))
	}
	if len(data.Faults) != 1 {
		t.Fatalf("faults = %d, want 1", len(data.Faults))
	}

	journey := data.Journeys[0]
	wantNodes := []string{"backend", "database", "frontend"}
	if !slices.Equal(journey.ReachableNodes, wantNodes) {
		t.Fatalf("reachable nodes = %v, want %v", journey.ReachableNodes, wantNodes)
	}
}

func TestBuildVizData_AstroshopTopology(t *testing.T) {
	t.Parallel()

	schema, err := topology.ParseFile(astroshopPath)
	if err != nil {
		t.Fatalf("ParseFile(%q): %v", astroshopPath, err)
	}

	data := buildVizData(schema)

	if len(data.Nodes) != 18 {
		t.Fatalf("nodes = %d, want 18", len(data.Nodes))
	}
	if len(data.Journeys) != 5 {
		t.Fatalf("journeys = %d, want 5", len(data.Journeys))
	}
	if len(data.Faults) != 4 {
		t.Fatalf("faults = %d, want 4", len(data.Faults))
	}

	var browse, placeOrder *vizJourney
	for i := range data.Journeys {
		switch data.Journeys[i].Name {
		case "browse":
			browse = &data.Journeys[i]
		case "place-order":
			placeOrder = &data.Journeys[i]
		}
	}
	if browse == nil {
		t.Fatal("browse journey not found")
	}
	if placeOrder == nil {
		t.Fatal("place-order journey not found")
	}

	browseWant := []string{"ad", "frontend", "image-provider", "postgres", "product-catalog"}
	for _, svc := range browseWant {
		if !slices.Contains(browse.ReachableNodes, svc) {
			t.Fatalf("browse reachable nodes missing %q: %v", svc, browse.ReachableNodes)
		}
	}

	placeOrderWant := []string{"payment", "shipping", "kafka"}
	for _, svc := range placeOrderWant {
		if !slices.Contains(placeOrder.ReachableNodes, svc) {
			t.Fatalf("place-order reachable nodes missing %q: %v", svc, placeOrder.ReachableNodes)
		}
	}
}

func TestBuildVizData_NodeSorting(t *testing.T) {
	t.Parallel()

	schema, err := topology.ParseFile(minimalPath)
	if err != nil {
		t.Fatalf("ParseFile(%q): %v", minimalPath, err)
	}

	data := buildVizData(schema)
	want := []string{"backend", "database", "frontend"}
	for i, node := range data.Nodes {
		if node.ID != want[i] {
			t.Fatalf("node[%d].ID = %q, want %q", i, node.ID, want[i])
		}
	}
}

func TestBuildVizData_EdgeIDFormat(t *testing.T) {
	t.Parallel()

	schema, err := topology.ParseFile(minimalPath)
	if err != nil {
		t.Fatalf("ParseFile(%q): %v", minimalPath, err)
	}

	data := buildVizData(schema)
	for _, edge := range data.Edges {
		if !strings.Contains(edge.ID, "->") {
			t.Fatalf("edge ID %q missing -> separator", edge.ID)
		}
		parts := strings.Split(edge.ID, "->")
		if len(parts) != 2 {
			t.Fatalf("edge ID %q has %d parts, want 2", edge.ID, len(parts))
		}
		if !strings.Contains(parts[0], ".") || !strings.Contains(parts[1], ".") {
			t.Fatalf("edge ID %q missing service.operation format", edge.ID)
		}
	}
}

func TestBuildVizData_FaultTargetID(t *testing.T) {
	t.Parallel()

	schema, err := topology.ParseFile(astroshopPath)
	if err != nil {
		t.Fatalf("ParseFile(%q): %v", astroshopPath, err)
	}

	data := buildVizData(schema)

	var nodeFault, opFault, edgeFault *vizFault
	for i := range data.Faults {
		switch data.Faults[i].TargetKind {
		case "node":
			nodeFault = &data.Faults[i]
		case "operation":
			if opFault == nil {
				opFault = &data.Faults[i]
			}
		case "edge":
			edgeFault = &data.Faults[i]
		}
	}

	if nodeFault == nil || nodeFault.TargetID != "recommendation" {
		t.Fatalf("node fault target = %v, want recommendation", nodeFault)
	}
	if opFault == nil || !strings.Contains(opFault.TargetID, ".") {
		t.Fatalf("operation fault target = %q, want svc.op format", opFault.TargetID)
	}
	if edgeFault == nil || !strings.Contains(edgeFault.TargetID, "->") {
		t.Fatalf("edge fault target = %q, want from->to format", edgeFault.TargetID)
	}
}
