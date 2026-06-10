package k6otelgen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/js/modulestest"
	"go.k6.io/k6/lib"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

const minimalTopologyYAML = `
services:
  frontend:
    kind: application
    operations:
      - name: GET /
journeys:
  home:
    steps:
      - service: frontend
        operation: GET /
`

const multiJourneyTopologyYAML = `
services:
  frontend:
    kind: application
    operations:
      - name: GET /
      - name: POST /checkout
journeys:
  checkout:
    steps:
      - service: frontend
        operation: POST /checkout
  home:
    steps:
      - service: frontend
        operation: GET /
`

func newTestRuntime(t *testing.T) *modulestest.Runtime {
	t.Helper()
	rt := modulestest.NewRuntime(t)
	if err := rt.SetupModuleSystem(map[string]any{"k6/x/otel-gen": New()}, nil, nil); err != nil {
		t.Fatalf("SetupModuleSystem() error = %v", err)
	}
	return rt
}

func newTestRootModule(t *testing.T) *RootModule {
	t.Helper()
	return New()
}

func loadTestSchema(t *testing.T, rt *modulestest.Runtime, yaml string) string {
	t.Helper()
	path := writeTempYAML(t, yaml)
	_, err := rt.RunOnEventLoop(fmt.Sprintf(`
		const otelgen = require("k6/x/otel-gen");
		otelgen.load(%q);
	`, path))
	if err != nil {
		t.Fatalf("load test schema: %v", err)
	}
	return path
}

func writeTempYAML(t testing.TB, yaml string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "topology.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

type fakeVU struct {
	ctx     context.Context
	runtime *sobek.Runtime
	state   *lib.State
}

func newFakeVU(t *testing.T, id uint64) modules.VU {
	t.Helper()
	return newFakeVUWithContext(t, id, context.Background())
}

func newFakeVUWithContext(t *testing.T, id uint64, ctx context.Context) modules.VU {
	t.Helper()
	return &fakeVU{
		ctx:     ctx,
		runtime: sobek.New(),
		state:   &lib.State{VUID: id},
	}
}

func (v *fakeVU) Context() context.Context {
	return v.ctx
}

func (v *fakeVU) Events() common.Events {
	return common.Events{}
}

func (v *fakeVU) InitEnv() *common.InitEnvironment {
	return nil
}

func (v *fakeVU) State() *lib.State {
	return v.state
}

func (v *fakeVU) Runtime() *sobek.Runtime {
	return v.runtime
}

func (v *fakeVU) RegisterCallback() func(func() error) {
	return func(fn func() error) {
		_ = fn()
	}
}

type mockSynth struct {
	mu       sync.Mutex
	spans    []synth.SpanInput
	metrics  []synth.MetricInput
	logs     []synth.LogInput
	contexts []context.Context
}

func newMockSynth() *mockSynth {
	return &mockSynth{}
}

func (m *mockSynth) BeginSpan(ctx context.Context, in synth.SpanInput) (context.Context, synth.FinishSpanFunc) {
	m.mu.Lock()
	m.spans = append(m.spans, in)
	m.contexts = append(m.contexts, ctx)
	m.mu.Unlock()
	return ctx, func(synth.Outcome) {}
}

func (m *mockSynth) RecordMetric(ctx context.Context, in synth.MetricInput) {
	m.mu.Lock()
	m.metrics = append(m.metrics, in)
	m.contexts = append(m.contexts, ctx)
	m.mu.Unlock()
}

func (m *mockSynth) EmitLog(ctx context.Context, in synth.LogInput) {
	m.mu.Lock()
	m.logs = append(m.logs, in)
	m.contexts = append(m.contexts, ctx)
	m.mu.Unlock()
}

func (m *mockSynth) spanCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.spans)
}

func (m *mockSynth) recordedContexts() []context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]context.Context, len(m.contexts))
	copy(out, m.contexts)
	return out
}

func testModuleSchema() *topology.Schema {
	service := &topology.Service{
		Name:       "api",
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: map[string]*topology.Operation{},
	}
	operation := &topology.Operation{Name: "GET /", Service: service}
	service.Operations[operation.Name] = operation
	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{service.Name: service},
		Journeys: map[string]*topology.Journey{
			"checkout": {
				Name:  "checkout",
				Steps: []*topology.Step{{Op: operation}},
			},
		},
	}
}
