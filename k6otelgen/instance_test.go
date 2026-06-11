// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

func TestExports_Names(t *testing.T) {
	t.Parallel()

	instance := &ModuleInstance{root: newTestRootModule(t), vu: newFakeVU(t, 1)}
	exports := instance.Exports()
	for _, name := range []string{"configure", "load", "stats", "journeys"} {
		if exports.Named[name] == nil {
			t.Fatalf("Exports().Named[%q] missing in %#v", name, exports.Named)
		}
	}
	if len(exports.Named) != 4 {
		t.Fatalf("Exports().Named len = %d, want 4", len(exports.Named))
	}
}

func TestLoad_HappyPath(t *testing.T) {
	t.Parallel()

	rt := newTestRuntime(t)
	_ = loadTestSchema(t, rt, minimalTopologyYAML)
}

func TestLoad_PathMismatch_ReturnsError(t *testing.T) {
	t.Parallel()

	instance := &ModuleInstance{root: newTestRootModule(t), vu: newFakeVU(t, 1)}
	first := writeTempYAML(t, minimalTopologyYAML)
	second := writeTempYAMLNamed(t, "other.yaml", minimalTopologyYAML)
	if _, err := instance.Load(first); err != nil {
		t.Fatalf("Load(first) error = %v", err)
	}
	_, err := instance.Load(second)
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("Load(second) error = %T, want *ConfigError", err)
	}
	if cfgErr.Kind != "path_mismatch" {
		t.Fatalf("ConfigError.Kind = %q, want path_mismatch", cfgErr.Kind)
	}
}

func TestLoad_InvalidYAML_ReturnsError(t *testing.T) {
	t.Parallel()

	instance := &ModuleInstance{root: newTestRootModule(t), vu: newFakeVU(t, 1)}
	path := writeTempYAML(t, "services:\n  frontend: [")
	_, err := instance.Load(path)
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("Load() error = %T, want *ConfigError", err)
	}
	if cfgErr.Kind != "parse_error" {
		t.Fatalf("ConfigError.Kind = %q, want parse_error", cfgErr.Kind)
	}
}

func TestConfigure_HappyPath(t *testing.T) {
	t.Parallel()

	root := newTestRootModule(t)
	instance := &ModuleInstance{root: root, vu: newFakeVU(t, 1)}
	err := instance.Configure(map[string]any{
		"endpoint":     "localhost:4317",
		"protocol":     "http",
		"timeout":      "2s",
		"batchSize":    128,
		"maxQueueSize": 256,
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	if !root.configured {
		t.Fatal("root.configured = false, want true")
	}
}

func TestConfigure_AlreadyConfigured_Error(t *testing.T) {
	t.Parallel()

	instance := &ModuleInstance{root: newTestRootModule(t), vu: newFakeVU(t, 1)}
	if err := instance.Configure(map[string]any{"endpoint": "localhost:4317"}); err != nil {
		t.Fatalf("first Configure() error = %v", err)
	}
	err := instance.Configure(map[string]any{"endpoint": "localhost:4318"})
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("second Configure() error = %T, want *ConfigError", err)
	}
	if cfgErr.Kind != "already_configured" {
		t.Fatalf("ConfigError.Kind = %q, want already_configured", cfgErr.Kind)
	}
}

func TestConfigure_Merge_JSOverridesEnv(t *testing.T) {
	setOTLPEnv(t)

	root := newTestRootModule(t)
	instance := &ModuleInstance{root: root, vu: newFakeVU(t, 1)}
	opts := map[string]any{
		"endpoint":     "js.example.com:4318",
		"protocol":     "http",
		"timeout":      "3s",
		"headers":      map[string]any{"Js": "2"},
		"batchSize":    64,
		"maxQueueSize": 128,
	}
	if err := instance.Configure(opts); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	jsCfg, err := optsToConfig(opts)
	if err != nil {
		t.Fatalf("optsToConfig() error = %v", err)
	}
	expected := exporter.Config{}.MergeWith(exporter.ConfigFromEnv()).MergeWith(jsCfg)
	if !reflect.DeepEqual(root.config, expected) {
		t.Fatalf("root.config = %#v, want %#v", root.config, expected)
	}
}

func TestStats_HappyPath(t *testing.T) {
	t.Parallel()

	instance := &ModuleInstance{root: newTestRootModule(t), vu: newFakeVU(t, 1)}
	if _, err := instance.Stats(); err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
}

func TestJourneys_BeforeLoad_Empty(t *testing.T) {
	t.Parallel()

	instance := &ModuleInstance{root: newTestRootModule(t), vu: newFakeVU(t, 1)}
	if got := instance.Journeys(); len(got) != 0 {
		t.Fatalf("Journeys() = %v, want empty", got)
	}
}

func TestJourneys_AfterLoad_Sorted(t *testing.T) {
	t.Parallel()

	instance := &ModuleInstance{root: newTestRootModule(t), vu: newFakeVU(t, 1)}
	path := writeTempYAML(t, multiJourneyTopologyYAML)
	if _, err := instance.Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []string{"checkout", "home"}
	if got := instance.Journeys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Journeys() = %v, want %v", got, want)
	}
}

func writeTempYAMLNamed(t *testing.T, name, yaml string) string {
	t.Helper()
	path := t.TempDir() + "/" + name
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func setOTLPEnv(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"ENDPOINT":    "env.example.com:4317",
		"PROTOCOL":    "http",
		"TIMEOUT":     "2000",
		"HEADERS":     "Env=1",
		"INSECURE":    "true",
		"COMPRESSION": "",
	}
	for suffix, value := range values {
		t.Setenv("OTEL_EXPORTER_OTLP_"+suffix, value)
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_"+suffix, value)
		t.Setenv("OTEL_EXPORTER_OTLP_METRICS_"+suffix, value)
		t.Setenv("OTEL_EXPORTER_OTLP_LOGS_"+suffix, value)
	}
}
