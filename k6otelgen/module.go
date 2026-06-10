package k6otelgen

import (
	"sync"
	"time"

	"go.k6.io/k6/js/modules"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
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
	configureMu   sync.Mutex
	configureErr  error
	config        exporter.Config
	configured    bool

	handle *TopologyHandle
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

func seedForVU(vu modules.VU) uint64 {
	seed := uint64(time.Now().UnixNano())
	if vu != nil && vu.State() != nil {
		seed ^= vu.State().VUID
	}
	return seed
}
