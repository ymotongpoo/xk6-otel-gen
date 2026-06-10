package k6otelgen

import (
	"context"
	"testing"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/lib"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestNew_ReturnsZeroState(t *testing.T) {
	t.Parallel()

	root := New()
	if root == nil {
		t.Fatal("New() = nil")
	}
	if root.schema != nil || root.overlay != nil || root.loadedPath != "" || root.configured || root.handle != nil {
		t.Fatalf("New() returned non-zero root: %#v", root)
	}
}

func TestNewModuleInstance_BeforeLoad_PartialInstance(t *testing.T) {
	t.Parallel()

	root := newTestRootModule(t)
	vu := newFakeVU(t, 1)
	instance, ok := root.NewModuleInstance(vu).(*ModuleInstance)
	if !ok {
		t.Fatalf("NewModuleInstance() type = %T, want *ModuleInstance", root.NewModuleInstance(vu))
	}
	if instance.root != root || instance.vu != vu {
		t.Fatalf("NewModuleInstance() root/vu not wired: %#v", instance)
	}
	if instance.engine != nil || instance.synth != nil || instance.handle != nil || instance.initErr != nil {
		t.Fatalf("partial instance initialized early: %#v", instance)
	}
}

func TestNewModuleInstance_AfterLoad_BuildsEngine(t *testing.T) {
	t.Parallel()
	exporter.ResetShared()
	t.Cleanup(exporter.ResetShared)

	root := newTestRootModule(t)
	root.schema = testModuleSchema()
	root.overlay = root.schema.ApplyFaults()
	root.loadedPath = "topology.yaml"

	instance := root.NewModuleInstance(newFakeVU(t, 7)).(*ModuleInstance)
	if instance.initErr != nil {
		t.Fatalf("initErr = %v, want nil", instance.initErr)
	}
	if instance.engine == nil || instance.synth == nil || instance.handle == nil {
		t.Fatalf("NewModuleInstance() did not initialize per-VU state: %#v", instance)
	}
	if instance.handle.instance != instance || instance.handle.module != root || instance.handle.name != "topology.yaml" {
		t.Fatalf("handle not wired to instance/root/path: %#v", instance.handle)
	}
}

func newTestRootModule(t *testing.T) *RootModule {
	t.Helper()
	return New()
}

type fakeVU struct {
	ctx     context.Context
	runtime *sobek.Runtime
	state   *lib.State
}

func newFakeVU(t *testing.T, id uint64) modules.VU {
	t.Helper()
	return &fakeVU{
		ctx:     context.Background(),
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
