// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"context"
	"sync"
	"testing"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/common"

	"github.com/ymotongpoo/xk6-otel-gen/journey"
	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestHandle_SetFaultIntensity_SetsEngineIntensity(t *testing.T) {
	t.Parallel()

	schema := faultIntensityTestSchema()
	syn := newOutcomeRecordingSynth()
	handle := newTestHandleFromSchema(t, context.Background(), syn, schema, 7)

	handle.SetFaultIntensity(0)
	handle.RunJourney("checkout")
	if syn.hasErrorOutcome() {
		t.Fatal("RunJourney() with intensity 0 produced an error outcome")
	}

	syn.resetOutcomes()
	handle.SetFaultIntensity(1)
	handle.RunJourney("checkout")
	if !syn.hasErrorOutcome() {
		t.Fatal("RunJourney() with intensity 1 did not produce an error outcome")
	}
}

func TestHandle_SetFaultIntensity_NotLoaded_Throws(t *testing.T) {
	t.Parallel()

	handle := &TopologyHandle{runtime: sobek.New()}
	defer func() {
		if recover() == nil {
			t.Fatal("SetFaultIntensity() did not throw")
		}
	}()
	handle.SetFaultIntensity(0.5)
}

func TestHandle_SetFaultIntensity_TargetOverride(t *testing.T) {
	t.Parallel()

	schema := faultIntensityTestSchema()
	syn := newOutcomeRecordingSynth()
	handle := newTestHandleFromSchema(t, context.Background(), syn, schema, 7)

	handle.SetFaultIntensity("operation:api.GET /", 0)
	handle.RunJourney("checkout")
	if syn.hasErrorOutcome() {
		t.Fatal("RunJourney() with target intensity 0 produced an error outcome")
	}

	syn.resetOutcomes()
	handle.SetFaultIntensity("operation:api.GET /", 1)
	handle.RunJourney("checkout")
	if !syn.hasErrorOutcome() {
		t.Fatal("RunJourney() with target intensity 1 did not produce an error outcome")
	}
}

func TestHandle_SetFaultIntensity_TargetOverrideFromJS(t *testing.T) {
	t.Parallel()

	schema := faultIntensityTestSchema()
	syn := newOutcomeRecordingSynth()
	handle := newTestHandleFromSchema(t, context.Background(), syn, schema, 7)
	handle.runtime.SetFieldNameMapper(common.FieldNameMapper{})
	if err := handle.runtime.Set("topology", handle.runtime.ToValue(handle)); err != nil {
		t.Fatalf("Runtime.Set() error = %v", err)
	}
	if _, err := handle.runtime.RunString(`topology.setFaultIntensity("operation:api.GET /", 0);`); err != nil {
		t.Fatalf("RunString() error = %v", err)
	}

	handle.RunJourney("checkout")
	if syn.hasErrorOutcome() {
		t.Fatal("RunJourney() after JS target intensity 0 produced an error outcome")
	}
}

func TestHandle_SetFaultIntensity_UnknownTarget_Throws(t *testing.T) {
	t.Parallel()

	schema := faultIntensityTestSchema()
	handle := newTestHandleFromSchema(t, context.Background(), newOutcomeRecordingSynth(), schema, 7)
	defer func() {
		if recover() == nil {
			t.Fatal("SetFaultIntensity() did not throw")
		}
	}()
	handle.SetFaultIntensity("operation:missing.GET /", 1)
}

func faultIntensityTestSchema() *topology.Schema {
	schema := testModuleSchema()
	op := schema.Services["api"].Operations["GET /"]
	schema.Faults = []topology.FaultSpec{{
		Target: topology.FaultTarget{Kind: topology.TargetOperation, Operation: op},
		Kind:   topology.FaultErrorRateOverride,
		Severity: topology.SeverityParams{
			Probability: 1,
			Value:       1,
		},
	}}
	return schema
}

func newTestHandleFromSchema(t *testing.T, ctx context.Context, syn synth.Synthesizer, schema *topology.Schema, seed uint64) *TopologyHandle {
	t.Helper()

	root := newTestRootModule(t)
	root.schema = schema
	root.overlay = schema.ApplyFaults()
	root.loadedPath = "topology.yaml"
	engine := journey.NewEngineWithSeed(root.schema, root.overlay, syn, seed)
	instance := &ModuleInstance{
		root:   root,
		vu:     newFakeVUWithContext(t, 1, ctx),
		engine: engine,
		synth:  syn,
	}
	handle := &TopologyHandle{
		runtime:  sobek.New(),
		engine:   engine,
		module:   root,
		instance: instance,
		name:     root.loadedPath,
	}
	instance.handle = handle
	return handle
}

type outcomeRecordingSynth struct {
	mu       sync.Mutex
	outcomes []synth.Outcome
}

func newOutcomeRecordingSynth() *outcomeRecordingSynth {
	return &outcomeRecordingSynth{}
}

func (s *outcomeRecordingSynth) BeginSpan(ctx context.Context, in synth.SpanInput) (context.Context, synth.FinishSpanFunc) {
	s.mu.Lock()
	idx := len(s.outcomes)
	s.outcomes = append(s.outcomes, synth.Outcome{})
	s.mu.Unlock()
	return ctx, func(out synth.Outcome) {
		s.mu.Lock()
		s.outcomes[idx] = out
		s.mu.Unlock()
	}
}

func (s *outcomeRecordingSynth) RecordMetric(context.Context, synth.MetricInput) {}

func (s *outcomeRecordingSynth) EmitLog(context.Context, synth.LogInput) {}

func (s *outcomeRecordingSynth) RecordCustom(context.Context, synth.CustomMetricInput) {}

func (s *outcomeRecordingSynth) UpdateState(context.Context, synth.StateUpdateInput) {}

func (s *outcomeRecordingSynth) EmitProfile(context.Context, synth.ProfileInput) {}

func (s *outcomeRecordingSynth) hasErrorOutcome() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, out := range s.outcomes {
		if !out.Success {
			return true
		}
	}
	return false
}

func (s *outcomeRecordingSynth) resetOutcomes() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomes = nil
}
