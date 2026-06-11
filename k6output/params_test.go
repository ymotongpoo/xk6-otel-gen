// SPDX-License-Identifier: Apache-2.0

package k6output

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

func TestDefaultParams_Values(t *testing.T) {
	t.Parallel()

	got := defaultParams()
	if got.Protocol != exporter.ProtocolGRPC {
		t.Fatalf("Protocol = %v, want %v", got.Protocol, exporter.ProtocolGRPC)
	}
	if got.Endpoint != "localhost:4317" {
		t.Fatalf("Endpoint = %q, want localhost:4317", got.Endpoint)
	}
	if got.Timeout != 10*time.Second || got.BatchSize != 512 || got.BatchTimeout != time.Second || got.MaxQueueSize != 2048 {
		t.Fatalf("OTLP defaults = timeout:%s batch:%d batchTimeout:%s maxQueue:%d", got.Timeout, got.BatchSize, got.BatchTimeout, got.MaxQueueSize)
	}
	if got.QueueSize != 100 || got.FlushInterval != time.Second {
		t.Fatalf("U6 defaults = queue:%d flush:%s, want 100 and 1s", got.QueueSize, got.FlushInterval)
	}
}

func TestParseOutArgs_Empty(t *testing.T) {
	t.Parallel()

	got, err := parseOutArgs("")
	if err != nil {
		t.Fatalf("parseOutArgs(empty) error = %v, want nil", err)
	}
	if got.Endpoint != "localhost:4317" || got.QueueSize != 100 {
		t.Fatalf("parseOutArgs(empty) = %#v, want defaults", got)
	}
	if len(got.provided) != 0 {
		t.Fatalf("provided keys = %v, want empty", got.provided)
	}
}

func TestParseOutArgs_AllKeys_HappyPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		arg   string
		check func(*testing.T, Params)
	}{
		{name: "endpoint", arg: "endpoint=https://otel.example.com:4317", check: func(t *testing.T, p Params) {
			if p.Endpoint != "https://otel.example.com:4317" {
				t.Fatalf("Endpoint = %q", p.Endpoint)
			}
		}},
		{name: "protocol", arg: "protocol=http", check: func(t *testing.T, p Params) {
			if p.Protocol != exporter.ProtocolHTTP {
				t.Fatalf("Protocol = %v, want HTTP", p.Protocol)
			}
		}},
		{name: "insecure", arg: "insecure=true", check: func(t *testing.T, p Params) {
			if !p.Insecure {
				t.Fatal("Insecure = false, want true")
			}
		}},
		{name: "headers", arg: "headers=api-key:abc;x-tenant:foo", check: func(t *testing.T, p Params) {
			want := map[string]string{"api-key": "abc", "x-tenant": "foo"}
			if !reflect.DeepEqual(p.Headers, want) {
				t.Fatalf("Headers = %#v, want %#v", p.Headers, want)
			}
		}},
		{name: "compression", arg: "compression=gzip", check: func(t *testing.T, p Params) {
			if p.Compression != "gzip" {
				t.Fatalf("Compression = %q, want gzip", p.Compression)
			}
		}},
		{name: "timeout", arg: "timeout=5s", check: func(t *testing.T, p Params) {
			if p.Timeout != 5*time.Second {
				t.Fatalf("Timeout = %s, want 5s", p.Timeout)
			}
		}},
		{name: "batchSize", arg: "batchSize=128", check: func(t *testing.T, p Params) {
			if p.BatchSize != 128 {
				t.Fatalf("BatchSize = %d, want 128", p.BatchSize)
			}
		}},
		{name: "batchTimeout", arg: "batchTimeout=250ms", check: func(t *testing.T, p Params) {
			if p.BatchTimeout != 250*time.Millisecond {
				t.Fatalf("BatchTimeout = %s, want 250ms", p.BatchTimeout)
			}
		}},
		{name: "maxQueueSize", arg: "maxQueueSize=4096", check: func(t *testing.T, p Params) {
			if p.MaxQueueSize != 4096 {
				t.Fatalf("MaxQueueSize = %d, want 4096", p.MaxQueueSize)
			}
		}},
		{name: "queueSize", arg: "queueSize=1000", check: func(t *testing.T, p Params) {
			if p.QueueSize != 1000 {
				t.Fatalf("QueueSize = %d, want 1000", p.QueueSize)
			}
		}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseOutArgs(tt.arg)
			if err != nil {
				t.Fatalf("parseOutArgs(%q) error = %v, want nil", tt.arg, err)
			}
			if !got.wasProvided(tt.name) {
				t.Fatalf("provided[%q] = false, want true", tt.name)
			}
			tt.check(t, got)
		})
	}
}

func TestParseOutArgs_InvalidProtocol(t *testing.T) {
	t.Parallel()

	_, err := parseOutArgs("protocol=udp")
	assertConfigError(t, err, ConfigErrorKindInvalidProtocol, "protocol", "udp")
}

func TestParseOutArgs_InvalidURL(t *testing.T) {
	t.Parallel()

	_, err := parseOutArgs("endpoint=://")
	assertConfigError(t, err, ConfigErrorKindInvalidURL, "endpoint", "://")
}

func TestParseOutArgs_TypeMismatch_Insecure(t *testing.T) {
	t.Parallel()

	_, err := parseOutArgs("insecure=yes")
	assertConfigError(t, err, ConfigErrorKindTypeMismatch, "insecure", "yes")
}

func TestParseOutArgs_TypeMismatch_Timeout(t *testing.T) {
	t.Parallel()

	_, err := parseOutArgs("timeout=soon")
	assertConfigError(t, err, ConfigErrorKindTypeMismatch, "timeout", "soon")
}

func TestParseOutArgs_TypeMismatch_BatchSize(t *testing.T) {
	t.Parallel()

	_, err := parseOutArgs("batchSize=one")
	assertConfigError(t, err, ConfigErrorKindTypeMismatch, "batchSize", "one")
}

func TestParseOutArgs_QueueSizeOutOfRange(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{"queueSize=9", "queueSize=10001"} {
		arg := arg
		t.Run(arg, func(t *testing.T) {
			t.Parallel()

			_, err := parseOutArgs(arg)
			var cfgErr *ConfigError
			if !errors.As(err, &cfgErr) || cfgErr.Kind != ConfigErrorKindInvalidArgs || cfgErr.Field != "queueSize" {
				t.Fatalf("parseOutArgs(%q) error = %v, want queueSize invalid_args", arg, err)
			}
		})
	}
}

func TestParseOutArgs_UnknownKey_Ignored(t *testing.T) {
	t.Parallel()

	got, err := parseOutArgs("future=value,queueSize=10")
	if err != nil {
		t.Fatalf("parseOutArgs unknown key error = %v, want nil", err)
	}
	if got.QueueSize != 10 {
		t.Fatalf("QueueSize = %d, want 10", got.QueueSize)
	}
	if got.wasProvided("future") {
		t.Fatal("unknown key marked as provided")
	}
}

func TestParseOutArgs_MalformedToken(t *testing.T) {
	t.Parallel()

	_, err := parseOutArgs("endpoint")
	assertConfigError(t, err, ConfigErrorKindInvalidArgs, "endpoint", "(missing =)")
}

func TestParseHeaders_Basic(t *testing.T) {
	t.Parallel()

	got, err := parseHeaders("api-key:abc")
	if err != nil {
		t.Fatalf("parseHeaders() error = %v, want nil", err)
	}
	want := map[string]string{"api-key": "abc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseHeaders() = %#v, want %#v", got, want)
	}
}

func TestParseHeaders_Multiple(t *testing.T) {
	t.Parallel()

	got, err := parseHeaders("api-key:abc; x-tenant: foo")
	if err != nil {
		t.Fatalf("parseHeaders() error = %v, want nil", err)
	}
	want := map[string]string{"api-key": "abc", "x-tenant": "foo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseHeaders() = %#v, want %#v", got, want)
	}
}

func TestParseHeaders_Malformed(t *testing.T) {
	t.Parallel()

	if _, err := parseHeaders("api-key"); err == nil {
		t.Fatal("parseHeaders(malformed) error = nil, want error")
	}
	if _, err := parseHeaders(":value"); err == nil {
		t.Fatal("parseHeaders(empty key) error = nil, want error")
	}
}

func TestExporterConfig_OnlyProvidedFieldsAndCopiesHeaders(t *testing.T) {
	t.Parallel()

	params, err := parseOutArgs("endpoint=localhost:4318,headers=api-key:abc,batchSize=64")
	if err != nil {
		t.Fatalf("parseOutArgs() error = %v, want nil", err)
	}
	cfg := params.exporterConfig()
	if cfg.Endpoint != "localhost:4318" || cfg.BatchSize != 64 {
		t.Fatalf("exporterConfig() = %#v, want provided endpoint and batchSize", cfg)
	}
	if cfg.Timeout != 0 || cfg.MaxQueueSize != 0 {
		t.Fatalf("exporterConfig() filled unprovided defaults: %#v", cfg)
	}
	params.Headers["api-key"] = "mutated"
	if cfg.Headers["api-key"] != "abc" {
		t.Fatalf("Headers alias source map, got %q", cfg.Headers["api-key"])
	}
}

func TestBuildPipelineConfig_AllProvidedFields(t *testing.T) {
	t.Parallel()

	params, err := parseOutArgs("endpoint=localhost:4318,protocol=http,insecure=false,headers=api-key:abc,compression=,timeout=2s,batchSize=64,batchTimeout=200ms,maxQueueSize=128,queueSize=20")
	if err != nil {
		t.Fatalf("parseOutArgs() error = %v, want nil", err)
	}
	cfg := buildPipelineConfig(params)
	if cfg.Endpoint != "localhost:4318" ||
		cfg.Protocol != exporter.ProtocolHTTP ||
		cfg.Insecure ||
		cfg.Compression != "" ||
		cfg.Timeout != 2*time.Second ||
		cfg.BatchSize != 64 ||
		cfg.BatchTimeout != 200*time.Millisecond ||
		cfg.MaxQueueSize != 128 {
		t.Fatalf("buildPipelineConfig() = %#v, want provided fields", cfg)
	}
	if !reflect.DeepEqual(cfg.Headers, map[string]string{"api-key": "abc"}) {
		t.Fatalf("Headers = %#v, want api-key", cfg.Headers)
	}
}

func assertConfigError(t *testing.T, err error, kind, field, value string) {
	t.Helper()

	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error = %v, want *ConfigError", err)
	}
	if cfgErr.Kind != kind || cfgErr.Field != field || cfgErr.Value != value {
		t.Fatalf("ConfigError = %#v, want kind=%q field=%q value=%q", cfgErr, kind, field, value)
	}
}
