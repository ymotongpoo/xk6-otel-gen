// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/grafana/sobek"
	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/journey"
	gen "github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"go.k6.io/k6/lib"
	"pgregory.net/rapid"
)

func TestLoad_Idempotent_Property(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		relPath := gen.ValidLoadPath().Draw(t, "load_path")
		path := filepath.Join(baseDir, relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(path, []byte(minimalTopologyYAML), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		instance := &ModuleInstance{root: New(), vu: newRapidVU(1, context.Background())}
		first, err := instance.Load(path)
		if err != nil {
			t.Fatalf("first Load() error = %v", err)
		}
		second, err := instance.Load(path)
		if err != nil {
			t.Fatalf("second Load() error = %v", err)
		}
		if first != second {
			t.Fatalf("Load() handles = %p and %p, want same pointer", first, second)
		}
	})
}

func TestConfigure_Merge_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		restore := setOTLPEnvForRapid(t)
		defer restore()

		jsOpts := gen.ValidConfigureOpts().Draw(t, "js_opts")
		root := New()
		instance := &ModuleInstance{root: root, vu: newRapidVU(1, context.Background())}
		if err := instance.Configure(jsOpts); err != nil {
			t.Fatalf("Configure() error = %v", err)
		}

		jsCfg, err := optsToConfig(jsOpts)
		if err != nil {
			t.Fatalf("optsToConfig() error = %v", err)
		}
		expected := exporter.Config{}.MergeWith(exporter.ConfigFromEnv()).MergeWith(jsCfg)
		if !reflect.DeepEqual(root.config, expected) {
			t.Fatalf("root.config = %#v, want %#v", root.config, expected)
		}
	})
}

func TestRunJourney_CtxPassed_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		type ctxKey struct{}
		ctxValue := rapid.StringMatching(`^[a-z0-9-]{1,16}$`).Draw(t, "ctx_value")
		ctx := context.WithValue(context.Background(), ctxKey{}, ctxValue)
		mock := newMockSynth()
		handle := newRapidHandle(ctx, mock)
		handle.RunJourney("checkout")
		for _, got := range mock.recordedContexts() {
			if got != ctx {
				t.Fatalf("recorded ctx = %p, want %p", got, ctx)
			}
		}
	})
}

func setOTLPEnvForRapid(t *rapid.T) func() {
	t.Helper()

	timeoutMS := rapid.IntRange(1, 30_000).Draw(t, "env_timeout_ms")
	values := map[string]string{
		"ENDPOINT":    fmt.Sprintf("env-%d.example.com:%d", timeoutMS, rapid.IntRange(1, 65535).Draw(t, "env_port")),
		"PROTOCOL":    rapid.SampledFrom([]string{"grpc", "http"}).Draw(t, "env_protocol"),
		"TIMEOUT":     strconv.Itoa(timeoutMS),
		"HEADERS":     "Env=1",
		"INSECURE":    strconv.FormatBool(rapid.Bool().Draw(t, "env_insecure")),
		"COMPRESSION": rapid.SampledFrom([]string{"", "gzip"}).Draw(t, "env_compression"),
	}

	keys := make([]string, 0, len(values)*4)
	for suffix := range values {
		keys = append(keys,
			"OTEL_EXPORTER_OTLP_"+suffix,
			"OTEL_EXPORTER_OTLP_TRACES_"+suffix,
			"OTEL_EXPORTER_OTLP_METRICS_"+suffix,
			"OTEL_EXPORTER_OTLP_LOGS_"+suffix,
		)
	}
	old := make(map[string]*string, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			copy := value
			old[key] = &copy
		} else {
			old[key] = nil
		}
	}
	for suffix, value := range values {
		for _, prefix := range []string{
			"OTEL_EXPORTER_OTLP_",
			"OTEL_EXPORTER_OTLP_TRACES_",
			"OTEL_EXPORTER_OTLP_METRICS_",
			"OTEL_EXPORTER_OTLP_LOGS_",
		} {
			if err := os.Setenv(prefix+suffix, value); err != nil {
				t.Fatalf("Setenv(%s) error = %v", prefix+suffix, err)
			}
		}
	}

	return func() {
		for key, value := range old {
			if value == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *value)
		}
	}
}

func newRapidVU(id uint64, ctx context.Context) *fakeVU {
	return &fakeVU{
		ctx:     ctx,
		runtime: sobek.New(),
		state:   &lib.State{VUID: id},
	}
}

func newRapidHandle(ctx context.Context, syn *mockSynth) *TopologyHandle {
	root := New()
	root.schema = testModuleSchema()
	root.overlay = root.schema.ApplyFaults()
	root.loadedPath = "topology.yaml"
	engine := journey.NewEngineWithSeed(root.schema, root.overlay, syn, 1)
	instance := &ModuleInstance{
		root:   root,
		vu:     newRapidVU(1, ctx),
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
