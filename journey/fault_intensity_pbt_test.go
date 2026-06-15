// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func TestFaultIntensityErrorRateOverride_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		intensity := rapid.Float64Range(0, 2).Draw(t, "intensity")
		value := rapid.Float64Range(0, 1).Draw(t, "value")

		schema := newPlanTestSchema()
		op := schema.Services["api"].Operations["GET /checkout"]
		schema.Faults = []topology.FaultSpec{
			faultOnOperation(op, topology.FaultErrorRateOverride, 1, value, 0),
		}
		engine := NewEngineWithSeed(schema, schema.ApplyFaults(), phase2Synth{}, 1)
		engine.SetFaultIntensity(intensity)
		plan, err := engine.BuildPlan("checkout")
		if err != nil {
			t.Fatalf("BuildPlan() error = %v", err)
		}
		node := plan.Root

		ff := engine.impl.foldFaults(node)
		want := clampProbability(value * engine.impl.faultIntensityValue())
		if ff.errorRate != want {
			t.Fatalf("foldFaults().errorRate = %f, want %f (value=%f intensity=%f)", ff.errorRate, want, value, engine.impl.faultIntensityValue())
		}
	})
}

func TestFaultIntensityFaultActiveEffectiveProbability_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		intensity := rapid.Float64Range(0, 2).Draw(t, "intensity")
		probability := rapid.Float64Range(0, 1).Draw(t, "probability")

		engine := NewEngineWithSeed(&topology.Schema{}, &topology.FaultOverlay{}, phase2Synth{}, 1)
		engine.SetFaultIntensity(intensity)

		wantEff := clampProbability(probability * engine.impl.faultIntensityValue())
		spec := topology.FaultSpec{Severity: topology.SeverityParams{Probability: probability}}
		got := engine.impl.faultActive(spec)

		switch {
		case wantEff <= 0:
			if got {
				t.Fatalf("faultActive() = true for effective probability %f (p=%f intensity=%f)", wantEff, probability, engine.impl.faultIntensityValue())
			}
		case wantEff >= 1:
			if !got {
				t.Fatalf("faultActive() = false for effective probability %f (p=%f intensity=%f)", wantEff, probability, engine.impl.faultIntensityValue())
			}
		}
	})
}
