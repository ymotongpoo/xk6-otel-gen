// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"gopkg.in/yaml.v3"
)

func TestEqual_Reflexive(t *testing.T) {
	t.Parallel()

	s := validManualSchema()
	if !topology.Equal(s, s) {
		t.Fatal("Equal(s, s) = false")
	}
}

func TestEqual_Symmetric(t *testing.T) {
	t.Parallel()

	a := validManualSchema()
	b := mustParseFromSchema(t, a)
	if topology.Equal(a, b) != topology.Equal(b, a) {
		t.Fatal("Equal is not symmetric")
	}
}

func TestEqual_DistinguishesDifferentCallsOrder(t *testing.T) {
	t.Parallel()

	a := validManualSchema()
	rootA := a.Services["frontend"].Operations["GET /"]
	backendA := a.Services["backend"].Operations["Fetch"]
	rootA.Calls = append(rootA.Calls, &topology.CallNode{Edge: &topology.Edge{
		From:         rootA,
		To:           backendA,
		Protocol:     topology.ProtocolGRPC,
		Latency:      topology.LatencyDist{Distribution: "constant"},
		RetryBackoff: topology.BackoffExponential,
	}})
	b := validManualSchema()
	rootB := b.Services["frontend"].Operations["GET /"]
	backendB := b.Services["backend"].Operations["Fetch"]
	rootB.Calls = append(rootB.Calls, &topology.CallNode{Edge: &topology.Edge{
		From:         rootB,
		To:           backendB,
		Protocol:     topology.ProtocolGRPC,
		Latency:      topology.LatencyDist{Distribution: "constant"},
		RetryBackoff: topology.BackoffExponential,
	}})
	rootB.Calls[0], rootB.Calls[1] = rootB.Calls[1], rootB.Calls[0]

	if topology.Equal(a, b) {
		t.Fatal("Equal returned true for different call order")
	}
}

func TestEqual_IgnoresMapIterationOrder(t *testing.T) {
	t.Parallel()

	a := validManualSchema()
	b := &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{},
		Journeys: map[string]*topology.Journey{},
		Faults:   append([]topology.FaultSpec(nil), a.Faults...),
	}
	for _, id := range []topology.ServiceID{"backend", "frontend"} {
		b.Services[id] = a.Services[id]
	}
	for _, name := range []string{"home"} {
		b.Journeys[name] = a.Journeys[name]
	}
	if !topology.Equal(a, b) {
		t.Fatal("Equal returned false for same maps inserted in different order")
	}
}

func mustParseFromSchema(t *testing.T, s *topology.Schema) *topology.Schema {
	t.Helper()
	data := mustMarshalYAML(t, s)
	return mustParse(t, string(data))
}

func mustMarshalYAML(t *testing.T, s *topology.Schema) []byte {
	t.Helper()
	data, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	return data
}
