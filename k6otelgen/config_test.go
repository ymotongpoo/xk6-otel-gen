// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

func TestOptsToConfig_AllFields_HappyPath(t *testing.T) {
	t.Parallel()

	got, err := optsToConfig(map[string]any{
		"endpoint":          "otel.example.com:4317",
		"protocol":          "http",
		"insecure":          true,
		"caCert":            "ca.pem",
		"clientCert":        "client.pem",
		"clientKey":         "client-key.pem",
		"headers":           map[string]any{"Authorization": "Bearer token", "X-Retry": int64(3)},
		"compression":       "gzip",
		"timeout":           "2.5s",
		"batchSize":         int64(64),
		"batchTimeout":      float64(1250),
		"maxQueueSize":      256,
		"resourceOverrides": map[string]any{"service.namespace": "checkout", "replica": float64(2)},
		"sampler":           "traceidratio",
		"samplerArg":        0.25,
	})
	if err != nil {
		t.Fatalf("optsToConfig() error = %v", err)
	}

	want := exporter.Config{
		Endpoint:          "otel.example.com:4317",
		Protocol:          exporter.ProtocolHTTP,
		Insecure:          true,
		InsecureSet:       true,
		Certificate:       "ca.pem",
		ClientCertificate: "client.pem",
		ClientKey:         "client-key.pem",
		Headers:           map[string]string{"Authorization": "Bearer token", "X-Retry": "3"},
		Compression:       "gzip",
		Timeout:           2500 * time.Millisecond,
		BatchSize:         64,
		BatchTimeout:      1250 * time.Millisecond,
		MaxQueueSize:      256,
		ResourceOverrides: map[string]string{"service.namespace": "checkout", "replica": "2"},
		Sampler:           "traceidratio",
		SamplerArg:        0.25,
		SamplerArgSet:     true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("optsToConfig() = %#v, want %#v", got, want)
	}
}

func TestOptsToConfig_TypeMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		value any
	}{
		{name: "endpoint", field: "endpoint", value: 123},
		{name: "protocol", field: "protocol", value: true},
		{name: "insecure", field: "insecure", value: "true"},
		{name: "caCert", field: "caCert", value: 123},
		{name: "clientCert", field: "clientCert", value: true},
		{name: "clientKey", field: "clientKey", value: []byte("key")},
		{name: "headers", field: "headers", value: "Authorization=token"},
		{name: "compression", field: "compression", value: 1},
		{name: "timeout", field: "timeout", value: struct{}{}},
		{name: "batchSize", field: "batchSize", value: 1.5},
		{name: "batchTimeout", field: "batchTimeout", value: []string{"1s"}},
		{name: "maxQueueSize", field: "maxQueueSize", value: "1024"},
		{name: "resourceOverrides", field: "resourceOverrides", value: map[string]any{"empty": []int{1}}},
		{name: "sampler", field: "sampler", value: true},
		{name: "samplerArg", field: "samplerArg", value: "0.5"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := optsToConfig(map[string]any{tt.field: tt.value})
			var cfgErr *ConfigError
			if !errors.As(err, &cfgErr) {
				t.Fatalf("optsToConfig() error = %T, want *ConfigError", err)
			}
			if cfgErr.Kind != "type_mismatch" || cfgErr.Path != tt.field {
				t.Fatalf("ConfigError = %#v, want kind type_mismatch path %q", cfgErr, tt.field)
			}
		})
	}
}

func TestOptsToConfig_InvalidSampler(t *testing.T) {
	t.Parallel()

	_, err := optsToConfig(map[string]any{"sampler": "parentbased_always_on"})
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("optsToConfig() error = %T, want *ConfigError", err)
	}
	if cfgErr.Kind != "invalid_sampler" {
		t.Fatalf("ConfigError = %#v, want invalid_sampler", cfgErr)
	}
}

func TestOptsToConfig_InvalidSamplerArg(t *testing.T) {
	t.Parallel()

	_, err := optsToConfig(map[string]any{"sampler": "traceidratio", "samplerArg": 1.5})
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("optsToConfig() error = %T, want *ConfigError", err)
	}
	if cfgErr.Kind != "invalid_sampler_arg" {
		t.Fatalf("ConfigError = %#v, want invalid_sampler_arg", cfgErr)
	}
}

func TestOptsToConfig_InvalidProtocol(t *testing.T) {
	t.Parallel()

	_, err := optsToConfig(map[string]any{"protocol": "udp"})
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("optsToConfig() error = %T, want *ConfigError", err)
	}
	if cfgErr.Kind != "invalid_protocol" || cfgErr.Path != "udp" {
		t.Fatalf("ConfigError = %#v, want invalid_protocol udp", cfgErr)
	}
}

func TestOptsToConfig_UnknownKey_Ignored(t *testing.T) {
	t.Parallel()

	got, err := optsToConfig(map[string]any{
		"endpoint": "localhost:4317",
		"future":   struct{ Value string }{Value: "ignored"},
	})
	if err != nil {
		t.Fatalf("optsToConfig() error = %v", err)
	}
	if got.Endpoint != "localhost:4317" {
		t.Fatalf("Endpoint = %q, want localhost:4317", got.Endpoint)
	}
}

func TestToDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    time.Duration
		wantErr bool
	}{
		{name: "int", value: 1500, want: 1500 * time.Millisecond},
		{name: "int64", value: int64(750), want: 750 * time.Millisecond},
		{name: "float64", value: 1.5, want: 1500 * time.Microsecond},
		{name: "string", value: "3s", want: 3 * time.Second},
		{name: "bad string", value: "soon", wantErr: true},
		{name: "bad type", value: false, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := toDuration(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("toDuration() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("toDuration() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("toDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToStringMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "basic",
			value: map[string]any{"a": "b"},
			want:  map[string]string{"a": "b"},
		},
		{
			name:  "numeric coercion",
			value: map[string]any{"int": 1, "int64": int64(2), "float": 3.25},
			want:  map[string]string{"int": "1", "int64": "2", "float": "3.25"},
		},
		{
			name:    "not object",
			value:   "a=b",
			wantErr: true,
		},
		{
			name:    "bad value",
			value:   map[string]any{"slice": []string{"x"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := toStringMap(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("toStringMap() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("toStringMap() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("toStringMap() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
