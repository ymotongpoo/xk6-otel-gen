// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"context"
	"testing"

	"github.com/grafana/sobek"

	"github.com/ymotongpoo/xk6-otel-gen/journey"
)

func TestHandle_RunJourney_HappyPath(t *testing.T) {
	t.Parallel()

	mock := newMockSynth()
	handle := newTestHandle(t, context.Background(), mock)
	handle.RunJourney("checkout")
	if mock.spanCount() == 0 {
		t.Fatal("RunJourney() emitted no spans")
	}
}

func TestHandle_RunJourney_UnknownJourney_ThrowsError(t *testing.T) {
	t.Parallel()

	handle := newTestHandle(t, context.Background(), newMockSynth())
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("RunJourney() did not throw")
		}
	}()
	handle.RunJourney("missing")
}

func TestHandle_RunJourney_CtxPassed(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "iteration")
	mock := newMockSynth()
	handle := newTestHandle(t, ctx, mock)
	handle.RunJourney("checkout")

	for _, got := range mock.recordedContexts() {
		if got != ctx {
			t.Fatalf("recorded ctx = %p, want %p", got, ctx)
		}
	}
}

func TestHandle_RunRandomJourney_ReturnsAndRunsPickedJourney(t *testing.T) {
	t.Parallel()

	mock := newMockSynth()
	handle := newTestHandle(t, context.Background(), mock)
	got := handle.RunRandomJourney()
	if got != "checkout" {
		t.Fatalf("RunRandomJourney() = %q, want checkout", got)
	}
	if mock.spanCount() == 0 {
		t.Fatal("RunRandomJourney() emitted no spans")
	}
}

func TestHandle_JourneyWeights_ReturnsConfiguredWeights(t *testing.T) {
	t.Parallel()

	handle := newTestHandle(t, context.Background(), newMockSynth())
	got := handle.JourneyWeights()
	if got["checkout"] != 1 {
		t.Fatalf("JourneyWeights() = %#v, want checkout weight 1", got)
	}
}

func TestHandle_Journeys_BeforeLoad_Empty(t *testing.T) {
	t.Parallel()

	handle := &TopologyHandle{}
	if got := handle.Journeys(); len(got) != 0 {
		t.Fatalf("Journeys() = %v, want empty", got)
	}
}

func TestHandle_Journeys_AfterLoad_Returns(t *testing.T) {
	t.Parallel()

	handle := newTestHandle(t, context.Background(), newMockSynth())
	got := handle.Journeys()
	if len(got) != 1 || got[0] != "checkout" {
		t.Fatalf("Journeys() = %v, want [checkout]", got)
	}
}

func newTestHandle(t *testing.T, ctx context.Context, syn *mockSynth) *TopologyHandle {
	t.Helper()

	root := newTestRootModule(t)
	root.schema = testModuleSchema()
	root.overlay = root.schema.ApplyFaults()
	root.loadedPath = "topology.yaml"
	engine := journey.NewEngineWithSeed(root.schema, root.overlay, syn, 1)
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
