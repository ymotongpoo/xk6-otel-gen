// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestNewEngine_NilArgs_Panic(t *testing.T) {
	t.Parallel()

	schema := &topology.Schema{}
	overlay := &topology.FaultOverlay{}
	syn := phase2Synth{}

	tests := []struct {
		name    string
		schema  *topology.Schema
		overlay *topology.FaultOverlay
		syn     synth.Synthesizer
	}{
		{name: "schema", schema: nil, overlay: overlay, syn: syn},
		{name: "overlay", schema: schema, overlay: nil, syn: syn},
		{name: "synth", schema: schema, overlay: overlay, syn: nil},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("NewEngine did not panic")
				}
			}()
			_ = NewEngine(tt.schema, tt.overlay, tt.syn)
		})
	}
}

func TestNewEngine_EmptySchema(t *testing.T) {
	t.Parallel()

	engine := NewEngine(&topology.Schema{}, &topology.FaultOverlay{}, phase2Synth{})
	if got := engine.ListJourneys(); len(got) != 0 {
		t.Fatalf("ListJourneys() = %v, want empty", got)
	}
}

func TestListJourneys_SortedKeys(t *testing.T) {
	t.Parallel()

	engine := NewEngine(phase2SchemaWithJourneys("checkout", "admin", "browse"), &topology.FaultOverlay{}, phase2Synth{})

	want := []string{"admin", "browse", "checkout"}
	if got := engine.ListJourneys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ListJourneys() = %v, want %v", got, want)
	}
}

func TestListJourneys_ReturnsCopy(t *testing.T) {
	t.Parallel()

	engine := NewEngine(phase2SchemaWithJourneys("checkout", "browse"), &topology.FaultOverlay{}, phase2Synth{})

	got := engine.ListJourneys()
	got[0] = "mutated"

	want := []string{"browse", "checkout"}
	if gotAgain := engine.ListJourneys(); !reflect.DeepEqual(gotAgain, want) {
		t.Fatalf("ListJourneys() after caller mutation = %v, want %v", gotAgain, want)
	}
}

func TestNewEngineWithSeed(t *testing.T) {
	t.Parallel()

	sameSeedA := runSeededEngineInstanceSequence(t, 42, 32)
	sameSeedB := runSeededEngineInstanceSequence(t, 42, 32)
	if !reflect.DeepEqual(sameSeedA, sameSeedB) {
		t.Fatalf("same seed sequences differ:\n%v\n%v", sameSeedA, sameSeedB)
	}

	differentSeed := runSeededEngineInstanceSequence(t, 43, 32)
	if reflect.DeepEqual(sameSeedA, differentSeed) {
		t.Fatalf("different seed sequence unexpectedly matched: %v", sameSeedA)
	}
}

type phase2Synth struct{}

func (phase2Synth) BeginSpan(ctx context.Context, _ synth.SpanInput) (context.Context, synth.FinishSpanFunc) {
	return ctx, func(synth.Outcome) {}
}

func (phase2Synth) RecordMetric(context.Context, synth.MetricInput) {}

func (phase2Synth) EmitLog(context.Context, synth.LogInput) {}

func (phase2Synth) RecordCustom(context.Context, synth.CustomMetricInput) {}

func (phase2Synth) EmitProfile(context.Context, synth.ProfileInput) {}

type recordingInstanceSynth struct {
	mu          sync.Mutex
	instanceIdx []int
}

func (s *recordingInstanceSynth) BeginSpan(ctx context.Context, in synth.SpanInput) (context.Context, synth.FinishSpanFunc) {
	s.mu.Lock()
	s.instanceIdx = append(s.instanceIdx, in.InstanceIdx)
	s.mu.Unlock()
	return ctx, func(synth.Outcome) {}
}

func (s *recordingInstanceSynth) RecordMetric(context.Context, synth.MetricInput) {}

func (s *recordingInstanceSynth) EmitLog(context.Context, synth.LogInput) {}

func (s *recordingInstanceSynth) RecordCustom(context.Context, synth.CustomMetricInput) {}

func (s *recordingInstanceSynth) EmitProfile(context.Context, synth.ProfileInput) {}

func (s *recordingInstanceSynth) sequence() []int {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]int, len(s.instanceIdx))
	copy(out, s.instanceIdx)
	return out
}

func runSeededEngineInstanceSequence(t *testing.T, seed uint64, runs int) []int {
	t.Helper()

	schema := phase2SchemaWithJourneys("checkout")
	for _, svc := range schema.Services {
		svc.Replicas = 32
	}
	overlay := schema.ApplyFaults()
	syn := &recordingInstanceSynth{}
	engine := NewEngineWithSeed(schema, overlay, syn, seed)
	plan, err := engine.BuildPlan("checkout")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	for range runs {
		if err := engine.Execute(context.Background(), plan); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	}
	return syn.sequence()
}

func phase2SchemaWithJourneys(names ...string) *topology.Schema {
	svc := &topology.Service{
		Name:       "api",
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: make(map[string]*topology.Operation),
	}
	op := &topology.Operation{Name: "GET /", Service: svc}
	svc.Operations[op.Name] = op
	schema := &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{svc.Name: svc},
		Journeys: make(map[string]*topology.Journey, len(names)),
	}
	for _, name := range names {
		schema.Journeys[name] = &topology.Journey{
			Name:  name,
			Steps: []*topology.Step{{Op: op}},
		}
	}
	return schema
}
