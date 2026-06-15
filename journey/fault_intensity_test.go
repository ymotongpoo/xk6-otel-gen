// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestFaultIntensityZero_DisablesInjectedFaults(t *testing.T) {
	t.Parallel()

	engine, node := newFaultTestEngine(t, func(schema *topology.Schema) []topology.FaultSpec {
		api := schema.Services["api"]
		op := api.Operations["GET /checkout"]
		edge := op.Calls[0].Edge
		return []topology.FaultSpec{
			faultOnService(api, topology.FaultCrash, 1, 0, 0),
			faultOnOperation(op, topology.FaultErrorRateOverride, 1, 0.75, 0),
			faultOnEdge(edge, topology.FaultDisconnect, 1, 0, 0),
		}
	})
	engine.SetFaultIntensity(0)

	spec := topology.FaultSpec{Severity: topology.SeverityParams{Probability: 1}}
	if engine.impl.faultActive(spec) {
		t.Fatal("faultActive() with intensity 0 = true, want false")
	}

	rootFF := engine.impl.foldFaults(node)
	if rootFF.crashed || rootFF.errorRate != 0 || rootFF.disconnected {
		t.Fatalf("foldFaults() with intensity 0 = %+v, want no injected faults", rootFF)
	}
}

func TestFaultIntensityOne_PreservesDefaultBehavior(t *testing.T) {
	t.Parallel()

	const seed uint64 = 4242
	const runs = 16

	defaultSeq := runSeededFaultEngineSequence(t, seed, runs, -1)
	explicitSeq := runSeededFaultEngineSequence(t, seed, runs, 1)
	if !reflect.DeepEqual(defaultSeq, explicitSeq) {
		t.Fatalf("intensity 1.0 changed behavior:\ndefault=%+v\nexplicit=%+v", defaultSeq, explicitSeq)
	}
}

func TestFaultIntensityHalf_ScalesProbabilityAndErrorRate(t *testing.T) {
	t.Parallel()

	engine, node := newFaultTestEngine(t, func(schema *topology.Schema) []topology.FaultSpec {
		op := schema.Services["api"].Operations["GET /checkout"]
		return []topology.FaultSpec{
			faultOnOperation(op, topology.FaultErrorRateOverride, 1, 0.8, 0),
		}
	})
	engine.SetFaultIntensity(0.5)

	ff := engine.impl.foldFaults(node)
	if ff.errorRate != 0.4 {
		t.Fatalf("foldFaults().errorRate = %f, want 0.4", ff.errorRate)
	}

	spec := topology.FaultSpec{Severity: topology.SeverityParams{Probability: 1}}
	eff := clampProbability(spec.Severity.Probability * engine.impl.faultIntensityValue())
	if eff != 0.5 {
		t.Fatalf("effective probability = %f, want 0.5", eff)
	}
}

func TestFaultIntensityHalf_DoesNotScaleEdgeErrorRate(t *testing.T) {
	t.Parallel()

	schema := newPlanTestSchema()
	schema.Faults = nil
	api := schema.Services["api"]
	op := api.Operations["GET /checkout"]
	op.Calls[0].Edge.ErrorRate = 0.8
	engine := NewEngineWithSeed(schema, schema.ApplyFaults(), phase2Synth{}, 99)
	engine.SetFaultIntensity(0.5)
	plan, err := engine.BuildPlan("checkout")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	ff := engine.impl.foldFaults(plan.Root.Children[0])
	if ff.errorRate != 0.8 {
		t.Fatalf("foldFaults().errorRate = %f, want native edge error rate 0.8", ff.errorRate)
	}
}

func TestSetFaultIntensity_Concurrent(t *testing.T) {
	t.Parallel()

	engine := NewEngineWithSeed(&topology.Schema{}, &topology.FaultOverlay{}, phase2Synth{}, 1)
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(v float64) {
			defer wg.Done()
			engine.SetFaultIntensity(v)
			_ = engine.impl.faultIntensityValue()
			spec := topology.FaultSpec{Severity: topology.SeverityParams{Probability: 0.5}}
			_ = engine.impl.faultActive(spec)
		}(float64(i) / 100)
	}
	wg.Wait()
}

func TestSetFaultIntensity_NegativeClampedToZero(t *testing.T) {
	t.Parallel()

	engine := NewEngineWithSeed(&topology.Schema{}, &topology.FaultOverlay{}, phase2Synth{}, 1)
	engine.SetFaultIntensity(-0.5)
	if got := engine.impl.faultIntensityValue(); got != 0 {
		t.Fatalf("faultIntensityValue() = %f, want 0", got)
	}
}

type faultOutcome struct {
	crashed      bool
	disconnected bool
	errorRate    float64
}

func runSeededFaultEngineSequence(t *testing.T, seed uint64, runs int, intensity float64) []faultOutcome {
	t.Helper()

	schema := newPlanTestSchema()
	schema.Faults = func(s *topology.Schema) []topology.FaultSpec {
		api := s.Services["api"]
		op := api.Operations["GET /checkout"]
		edge := op.Calls[0].Edge
		return []topology.FaultSpec{
			faultOnService(api, topology.FaultCrash, 0.5, 0, 0),
			faultOnOperation(op, topology.FaultErrorRateOverride, 1, 0.6, 0),
			faultOnEdge(edge, topology.FaultDisconnect, 0.25, 0, 0),
		}
	}(schema)
	engine := NewEngineWithSeed(schema, schema.ApplyFaults(), phase2Synth{}, seed)
	if intensity >= 0 {
		engine.SetFaultIntensity(intensity)
	}
	plan, err := engine.BuildPlan("checkout")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	out := make([]faultOutcome, 0, runs)
	for range runs {
		if err := engine.Execute(context.Background(), plan); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		rootFF := engine.impl.foldFaults(plan.Root)
		childFF := engine.impl.foldFaults(plan.Root.Children[0])
		out = append(out, faultOutcome{
			crashed:      rootFF.crashed,
			disconnected: childFF.disconnected,
			errorRate:    rootFF.errorRate,
		})
	}
	return out
}
