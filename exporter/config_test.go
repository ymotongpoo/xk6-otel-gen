package exporter

import (
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

var envTestMu sync.Mutex

func TestProtocol_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		proto Protocol
		want  string
	}{
		{name: "grpc", proto: ProtocolGRPC, want: "grpc"},
		{name: "http", proto: ProtocolHTTP, want: "http"},
		{name: "unknown", proto: Protocol(99), want: "unknown"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.proto.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfig_Validate_OK(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Protocol:     ProtocolHTTP,
		Endpoint:     "https://otel.example.com:4318",
		Headers:      map[string]string{"Authorization": "Bearer token", "X_Tenant": "tenant-a"},
		Compression:  "gzip",
		Timeout:      time.Second,
		BatchSize:    128,
		BatchTimeout: 500 * time.Millisecond,
		MaxQueueSize: 256,
		ResourceOverrides: map[string]string{
			string(semconv.ServiceNameKey): "checkout",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestConfig_Validate_Errors(t *testing.T) {
	t.Parallel()

	valid := Config{
		Protocol:     ProtocolGRPC,
		Endpoint:     "localhost:4317",
		Timeout:      time.Second,
		BatchSize:    128,
		BatchTimeout: time.Second,
		MaxQueueSize: 256,
	}
	tests := []struct {
		name  string
		cfg   Config
		field string
	}{
		{name: "protocol", cfg: withConfig(valid, func(c *Config) { c.Protocol = Protocol(99) }), field: "Protocol"},
		{name: "endpoint empty", cfg: withConfig(valid, func(c *Config) { c.Endpoint = "" }), field: "Endpoint"},
		{name: "endpoint malformed", cfg: withConfig(valid, func(c *Config) { c.Endpoint = "localhost" }), field: "Endpoint"},
		{name: "timeout", cfg: withConfig(valid, func(c *Config) { c.Timeout = 0 }), field: "Timeout"},
		{name: "batch size", cfg: withConfig(valid, func(c *Config) { c.BatchSize = 0 }), field: "BatchSize"},
		{name: "batch timeout", cfg: withConfig(valid, func(c *Config) { c.BatchTimeout = 0 }), field: "BatchTimeout"},
		{name: "max queue", cfg: withConfig(valid, func(c *Config) { c.MaxQueueSize = 0 }), field: "MaxQueueSize"},
		{name: "max queue below batch", cfg: withConfig(valid, func(c *Config) { c.MaxQueueSize = 64 }), field: "MaxQueueSize"},
		{name: "compression", cfg: withConfig(valid, func(c *Config) { c.Compression = "zstd" }), field: "Compression"},
		{name: "header key empty", cfg: withConfig(valid, func(c *Config) { c.Headers = map[string]string{"": "v"} }), field: "Headers"},
		{name: "header key invalid", cfg: withConfig(valid, func(c *Config) { c.Headers = map[string]string{"bad key": "v"} }), field: "Headers"},
		{name: "header value empty", cfg: withConfig(valid, func(c *Config) { c.Headers = map[string]string{"X-Test": ""} }), field: "Headers"},
		{name: "resource key empty", cfg: withConfig(valid, func(c *Config) { c.ResourceOverrides = map[string]string{"": "v"} }), field: "ResourceOverrides"},
		{name: "resource value empty", cfg: withConfig(valid, func(c *Config) { c.ResourceOverrides = map[string]string{string(semconv.ServiceNameKey): ""} }), field: "ResourceOverrides"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want ConfigError")
			}
			var cfgErr *ConfigError
			if !errors.As(err, &cfgErr) {
				t.Fatalf("Validate() error type = %T, want *ConfigError", err)
			}
			if !joinedErrorHasField(err, tt.field) {
				t.Fatalf("Validate() error = %v, want field %s", err, tt.field)
			}
		})
	}
}

func TestConfig_MergeWith_Examples(t *testing.T) {
	t.Parallel()

	base := Config{
		Protocol:          ProtocolGRPC,
		Endpoint:          "base:4317",
		Headers:           map[string]string{"base": "kept"},
		Timeout:           time.Second,
		BatchSize:         128,
		BatchTimeout:      time.Second,
		MaxQueueSize:      256,
		ResourceOverrides: map[string]string{string(semconv.ServiceNameKey): "base"},
	}
	override := Config{
		Protocol:          ProtocolHTTP,
		Endpoint:          "override:4318",
		Headers:           map[string]string{"override": "wins"},
		Insecure:          true,
		Compression:       "gzip",
		Timeout:           2 * time.Second,
		BatchSize:         256,
		BatchTimeout:      3 * time.Second,
		MaxQueueSize:      512,
		ResourceOverrides: map[string]string{string(semconv.ServiceNameKey): "override"},
	}

	merged := base.MergeWith(override)
	if merged.Protocol != ProtocolHTTP || merged.Endpoint != "override:4318" || !merged.Insecure {
		t.Fatalf("MergeWith() = %#v, override scalar fields did not win", merged)
	}
	if !reflect.DeepEqual(merged.Headers, map[string]string{"override": "wins"}) {
		t.Fatalf("Headers = %#v, want replacement map", merged.Headers)
	}
	if !reflect.DeepEqual(merged.ResourceOverrides, map[string]string{string(semconv.ServiceNameKey): "override"}) {
		t.Fatalf("ResourceOverrides = %#v, want replacement map", merged.ResourceOverrides)
	}
	if merged.Timeout != 2*time.Second || merged.BatchSize != 256 || merged.BatchTimeout != 3*time.Second || merged.MaxQueueSize != 512 {
		t.Fatalf("MergeWith() = %#v, override batch fields did not win", merged)
	}

	emptyMaps := base.MergeWith(Config{
		Headers:           map[string]string{},
		ResourceOverrides: map[string]string{},
	})
	if len(emptyMaps.Headers) != 0 || len(emptyMaps.ResourceOverrides) != 0 {
		t.Fatalf("empty map override did not replace maps: %#v", emptyMaps)
	}

	falseInsecure := Config{Insecure: true}.MergeWith(Config{Insecure: false})
	if !falseInsecure.Insecure {
		t.Fatal("Insecure=false override cleared true, want one-way merge")
	}
}

func TestConfig_fillDefaults_Examples(t *testing.T) {
	t.Parallel()

	cfg := Config{}.fillDefaults()
	if cfg.Protocol != ProtocolGRPC {
		t.Fatalf("Protocol = %v, want ProtocolGRPC", cfg.Protocol)
	}
	if cfg.Endpoint != defaultEndpoint {
		t.Fatalf("Endpoint = %q, want %q", cfg.Endpoint, defaultEndpoint)
	}
	if cfg.Timeout != defaultTimeout || cfg.BatchSize != defaultBatchSize || cfg.BatchTimeout != defaultBatchTimeout || cfg.MaxQueueSize != defaultMaxQueueSize {
		t.Fatalf("fillDefaults() = %#v, want built-in defaults", cfg)
	}

	custom := Config{Endpoint: "custom:4317", Timeout: time.Second, BatchSize: 10, BatchTimeout: time.Second, MaxQueueSize: 20}.fillDefaults()
	if custom.Endpoint != "custom:4317" || custom.Timeout != time.Second || custom.BatchSize != 10 || custom.BatchTimeout != time.Second || custom.MaxQueueSize != 20 {
		t.Fatalf("fillDefaults() overwrote custom values: %#v", custom)
	}
}

func TestConfigFromEnv_Generic(t *testing.T) {
	t.Parallel()

	withOTLPEnv(t, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT":    "env:4317",
		"OTEL_EXPORTER_OTLP_HEADERS":     "Authorization=Bearer%20token,X-Tenant=tenant-a",
		"OTEL_EXPORTER_OTLP_PROTOCOL":    "http/protobuf",
		"OTEL_EXPORTER_OTLP_COMPRESSION": "gzip",
		"OTEL_EXPORTER_OTLP_TIMEOUT":     "1500",
		"OTEL_EXPORTER_OTLP_INSECURE":    "true",
	})

	cfg := ConfigFromEnv()
	if cfg.Endpoint != "env:4317" {
		t.Fatalf("Endpoint = %q, want env:4317", cfg.Endpoint)
	}
	if cfg.Protocol != ProtocolHTTP {
		t.Fatalf("Protocol = %v, want ProtocolHTTP", cfg.Protocol)
	}
	if cfg.Compression != "gzip" || cfg.Timeout != 1500*time.Millisecond || !cfg.Insecure {
		t.Fatalf("ConfigFromEnv() = %#v, want compression/timeout/insecure from env", cfg)
	}
	wantHeaders := map[string]string{"Authorization": "Bearer token", "X-Tenant": "tenant-a"}
	if !reflect.DeepEqual(cfg.Headers, wantHeaders) {
		t.Fatalf("Headers = %#v, want %#v", cfg.Headers, wantHeaders)
	}
}

func TestConfigFromEnv_SignalSpecificPriority(t *testing.T) {
	t.Parallel()

	withOTLPEnv(t, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT":         "generic:4317",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT":  "traces:4317",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": "metrics:4317",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT":    "logs:4317",
	})

	cfg := ConfigFromEnv()
	if cfg.Endpoint != "traces:4317" {
		t.Fatalf("Endpoint = %q, want traces:4317", cfg.Endpoint)
	}
}

func TestConfigFromEnv_InvalidValuesRemainInvalid(t *testing.T) {
	t.Parallel()

	withOTLPEnv(t, map[string]string{
		"OTEL_EXPORTER_OTLP_PROTOCOL": "not-a-protocol",
		"OTEL_EXPORTER_OTLP_TIMEOUT":  "not-a-timeout",
		"OTEL_EXPORTER_OTLP_HEADERS":  "bad-header",
	})

	cfg := ConfigFromEnv()
	if cfg.Protocol != Protocol(-1) {
		t.Fatalf("Protocol = %v, want invalid protocol sentinel", cfg.Protocol)
	}
	if cfg.Timeout != -1 {
		t.Fatalf("Timeout = %v, want invalid timeout sentinel", cfg.Timeout)
	}
	if cfg.Headers["bad-header"] != "" {
		t.Fatalf("Headers = %#v, want invalid header with empty value", cfg.Headers)
	}
}

func withConfig(base Config, mutate func(*Config)) Config {
	mutate(&base)
	return base
}

func joinedErrorHasField(err error, field string) bool {
	for _, joined := range flattenJoined(err) {
		var cfgErr *ConfigError
		if errors.As(joined, &cfgErr) && cfgErr.Field == field {
			return true
		}
	}
	return false
}

func flattenJoined(err error) []error {
	if err == nil {
		return nil
	}
	type unwrapper interface {
		Unwrap() []error
	}
	if multi, ok := err.(unwrapper); ok {
		var out []error
		for _, child := range multi.Unwrap() {
			out = append(out, flattenJoined(child)...)
		}
		return out
	}
	return []error{err}
}

func withOTLPEnv(t *testing.T, values map[string]string) {
	t.Helper()
	envTestMu.Lock()

	keys := []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_HEADERS",
		"OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_EXPORTER_OTLP_COMPRESSION",
		"OTEL_EXPORTER_OTLP_TIMEOUT",
		"OTEL_EXPORTER_OTLP_INSECURE",
	}
	signalPrefixes := []string{
		"OTEL_EXPORTER_OTLP_TRACES_",
		"OTEL_EXPORTER_OTLP_METRICS_",
		"OTEL_EXPORTER_OTLP_LOGS_",
	}
	suffixes := []string{"ENDPOINT", "HEADERS", "PROTOCOL", "COMPRESSION", "TIMEOUT", "INSECURE"}
	for _, prefix := range signalPrefixes {
		for _, suffix := range suffixes {
			keys = append(keys, prefix+suffix)
		}
	}

	type oldValue struct {
		value string
		set   bool
	}
	old := make(map[string]oldValue, len(keys))
	for _, key := range keys {
		value, set := os.LookupEnv(key)
		old[key] = oldValue{value: value, set: set}
	}
	t.Cleanup(func() {
		for key, state := range old {
			if state.set {
				if err := os.Setenv(key, state.value); err != nil {
					t.Errorf("restore %s: %v", key, err)
				}
				continue
			}
			if err := os.Unsetenv(key); err != nil {
				t.Errorf("unset %s: %v", key, err)
			}
		}
		envTestMu.Unlock()
	})

	for _, key := range keys {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
	for key, value := range values {
		if err := os.Setenv(key, value); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
}
