package k6otelgen

import (
	"context"
	"sync"
	"time"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/modules"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/journey"
	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

// RootModule is the process-singleton k6 extension module.
type RootModule struct {
	schemaOnce sync.Once
	schemaErr  error
	schema     *topology.Schema
	overlay    *topology.FaultOverlay
	loadedPath string

	configureOnce sync.Once
	configureErr  error
	config        exporter.Config
	configured    bool

	handle *TopologyHandle
}

// ModuleInstance is constructed once per k6 VU and holds per-VU state.
type ModuleInstance struct {
	root    *RootModule
	vu      modules.VU
	engine  *journey.Engine
	synth   synth.Synthesizer
	handle  *TopologyHandle
	initErr error
}

func init() {
	modules.Register("k6/x/otel-gen", New())
}

// New returns a fresh zero-state k6 OpenTelemetry generator module.
func New() *RootModule {
	return &RootModule{}
}

// NewModuleInstance constructs the per-VU module instance for k6.
func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance {
	instance := &ModuleInstance{root: r, vu: vu}
	if r.schema == nil {
		return instance
	}
	if err := instance.lateInit(); err != nil {
		instance.initErr = err
	}
	return instance
}

// Exports returns the JS-visible API surface for this VU.
func (i *ModuleInstance) Exports() modules.Exports {
	return modules.Exports{}
}

func (i *ModuleInstance) getOrBuildPipeline() (*exporter.Pipeline, error) {
	return exporter.GetShared(func() (*exporter.Pipeline, error) {
		return exporter.New(i.root.config)
	})
}

func (i *ModuleInstance) lateInit() error {
	if i.root == nil || i.root.schema == nil {
		return &ConfigError{Kind: "not_loaded"}
	}
	pipeline, err := i.getOrBuildPipeline()
	if err != nil {
		return err
	}

	syn := synth.NewDefault(
		pipeline.TracerProvider(),
		pipeline.MeterProvider(),
		pipeline.LoggerProvider(),
	)
	engine := journey.NewEngineWithSeed(i.root.schema, i.root.overlay, syn, seedForVU(i.vu))
	i.synth = syn
	i.engine = engine
	i.handle = &TopologyHandle{
		runtime:  runtimeForVU(i.vu),
		engine:   engine,
		module:   i.root,
		instance: i,
		name:     i.root.loadedPath,
	}
	i.root.handle = i.handle
	return nil
}

func (i *ModuleInstance) vuContext() context.Context {
	if i == nil || i.vu == nil || i.vu.Context() == nil {
		return context.Background()
	}
	return i.vu.Context()
}

func seedForVU(vu modules.VU) uint64 {
	seed := uint64(time.Now().UnixNano())
	if vu != nil && vu.State() != nil {
		seed ^= vu.State().VUID
	}
	return seed
}

func runtimeForVU(vu modules.VU) *sobek.Runtime {
	if vu == nil {
		return nil
	}
	return vu.Runtime()
}
