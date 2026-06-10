package journey

import (
	"context"
	"reflect"
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

type phase2Synth struct{}

func (phase2Synth) BeginSpan(ctx context.Context, _ synth.SpanInput) (context.Context, synth.FinishSpanFunc) {
	return ctx, func(synth.Outcome) {}
}

func (phase2Synth) RecordMetric(context.Context, synth.MetricInput) {}

func (phase2Synth) EmitLog(context.Context, synth.LogInput) {}

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
