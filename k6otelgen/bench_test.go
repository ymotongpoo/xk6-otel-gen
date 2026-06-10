package k6otelgen

import (
	"context"
	"testing"

	"github.com/grafana/sobek"
	"go.k6.io/k6/lib"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

func BenchmarkNewModuleInstance(b *testing.B) {
	exporter.ResetShared()
	b.Cleanup(exporter.ResetShared)

	root := New()
	root.schema = testModuleSchema()
	root.overlay = root.schema.ApplyFaults()
	root.loadedPath = "topology.yaml"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance := root.NewModuleInstance(newBenchVU(uint64(i + 1))).(*ModuleInstance)
		if instance.initErr != nil {
			b.Fatalf("initErr = %v", instance.initErr)
		}
	}
}

func BenchmarkLoad(b *testing.B) {
	exporter.ResetShared()
	b.Cleanup(exporter.ResetShared)
	path := writeTempYAML(b, minimalTopologyYAML)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		root := New()
		instance := &ModuleInstance{root: root, vu: newBenchVU(uint64(i + 1))}
		if _, err := instance.Load(path); err != nil {
			b.Fatalf("Load() error = %v", err)
		}
	}
}

func BenchmarkConfigure(b *testing.B) {
	opts := map[string]any{
		"endpoint":     "localhost:4317",
		"protocol":     "http",
		"insecure":     true,
		"timeout":      "2s",
		"batchSize":    128,
		"maxQueueSize": 512,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		root := New()
		instance := &ModuleInstance{root: root, vu: newBenchVU(uint64(i + 1))}
		if err := instance.Configure(opts); err != nil {
			b.Fatalf("Configure() error = %v", err)
		}
	}
}

func newBenchVU(id uint64) *fakeVU {
	return &fakeVU{
		ctx:     context.Background(),
		runtime: sobek.New(),
		state:   &lib.State{VUID: id},
	}
}
