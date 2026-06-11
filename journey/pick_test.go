// SPDX-License-Identifier: Apache-2.0

package journey_test

import (
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/journey"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func TestPickJourney_SeededWeightedFrequency(t *testing.T) {
	t.Parallel()

	schema := weightedPickSchema(map[string]float64{"light": 1, "heavy": 3})
	engine := journey.NewEngineWithSeed(schema, schema.ApplyFaults(), &pbtSynth{}, 42)
	counts := map[string]int{}
	for range 10_000 {
		counts[engine.PickJourney()]++
	}

	if counts["light"] < 2400 || counts["light"] > 2600 {
		t.Fatalf("light count = %d, want fixed-seed count near 2500", counts["light"])
	}
	if counts["heavy"] < 7400 || counts["heavy"] > 7600 {
		t.Fatalf("heavy count = %d, want fixed-seed count near 7500", counts["heavy"])
	}
}

func TestPickJourney_ReturnsDefinedJourney_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		weights := map[string]float64{
			"one": rapid.Float64Range(0.1, 10).Draw(t, "one_weight"),
			"two": rapid.Float64Range(0.1, 10).Draw(t, "two_weight"),
		}
		schema := weightedPickSchema(weights)
		engine := journey.NewEngineWithSeed(schema, schema.ApplyFaults(), &pbtSynth{}, 7)
		names := map[string]struct{}{}
		for _, name := range engine.ListJourneys() {
			names[name] = struct{}{}
		}

		for i := 0; i < 100; i++ {
			picked := engine.PickJourney()
			if _, ok := names[picked]; !ok {
				t.Fatalf("PickJourney() = %q, not in %v", picked, names)
			}
		}
	})
}

func TestPickJourney_SingleJourney_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		weight := rapid.Float64Range(0.1, 10).Draw(t, "weight")
		schema := weightedPickSchema(map[string]float64{"only": weight})
		engine := journey.NewEngineWithSeed(schema, schema.ApplyFaults(), &pbtSynth{}, 9)
		for i := 0; i < 100; i++ {
			if got := engine.PickJourney(); got != "only" {
				t.Fatalf("PickJourney() = %q, want only", got)
			}
		}
	})
}

func weightedPickSchema(weights map[string]float64) *topology.Schema {
	svc := &topology.Service{
		Name:       "frontend",
		Namespace:  topology.DefaultNamespace,
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: map[string]*topology.Operation{},
	}
	op := &topology.Operation{Name: "GET /", Service: svc}
	svc.Operations[op.Name] = op

	journeys := make(map[string]*topology.Journey, len(weights))
	for name, weight := range weights {
		journeys[name] = &topology.Journey{Name: name, Weight: weight, Steps: []*topology.Step{{Op: op}}}
	}
	return &topology.Schema{
		Namespace: topology.DefaultNamespace,
		Services:  map[topology.ServiceID]*topology.Service{svc.Name: svc},
		Journeys:  journeys,
	}
}
