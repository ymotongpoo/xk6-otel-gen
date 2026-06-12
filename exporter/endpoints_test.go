// SPDX-License-Identifier: Apache-2.0

package exporter_test

import (
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

func TestResolveEndpoints_Examples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                              string
		cfg                               exporter.Config
		wantTraces, wantMetrics, wantLogs string
	}{
		{
			name: "grafana cloud base url appends per-signal path",
			cfg: exporter.Config{
				Protocol: exporter.ProtocolHTTP,
				Endpoint: "https://otlp-gateway-prod-ap-northeast-0.grafana.net/otlp",
			},
			wantTraces:  "https://otlp-gateway-prod-ap-northeast-0.grafana.net/otlp/v1/traces",
			wantMetrics: "https://otlp-gateway-prod-ap-northeast-0.grafana.net/otlp/v1/metrics",
			wantLogs:    "https://otlp-gateway-prod-ap-northeast-0.grafana.net/otlp/v1/logs",
		},
		{
			name: "base url with trailing slash",
			cfg: exporter.Config{
				Protocol: exporter.ProtocolHTTP,
				Endpoint: "https://host.example.com/otlp/",
			},
			wantTraces:  "https://host.example.com/otlp/v1/traces",
			wantMetrics: "https://host.example.com/otlp/v1/metrics",
			wantLogs:    "https://host.example.com/otlp/v1/logs",
		},
		{
			name: "base url with no path",
			cfg: exporter.Config{
				Protocol: exporter.ProtocolHTTP,
				Endpoint: "https://host.example.com:4318",
			},
			wantTraces:  "https://host.example.com:4318/v1/traces",
			wantMetrics: "https://host.example.com:4318/v1/metrics",
			wantLogs:    "https://host.example.com:4318/v1/logs",
		},
		{
			name: "base url already ending in v1/traces still appends (no dedupe)",
			cfg: exporter.Config{
				Protocol: exporter.ProtocolHTTP,
				Endpoint: "https://host.example.com/v1/traces",
			},
			wantTraces:  "https://host.example.com/v1/traces/v1/traces",
			wantMetrics: "https://host.example.com/v1/traces/v1/metrics",
			wantLogs:    "https://host.example.com/v1/traces/v1/logs",
		},
		{
			name: "base url with query and fragment preserved",
			cfg: exporter.Config{
				Protocol: exporter.ProtocolHTTP,
				Endpoint: "https://host.example.com/otlp?token=abc#frag",
			},
			wantTraces:  "https://host.example.com/otlp/v1/traces?token=abc#frag",
			wantMetrics: "https://host.example.com/otlp/v1/metrics?token=abc#frag",
			wantLogs:    "https://host.example.com/otlp/v1/logs?token=abc#frag",
		},
		{
			name: "http host:port base left as-is for sdk default path",
			cfg: exporter.Config{
				Protocol: exporter.ProtocolHTTP,
				Endpoint: "localhost:4318",
			},
			wantTraces:  "localhost:4318",
			wantMetrics: "localhost:4318",
			wantLogs:    "localhost:4318",
		},
		{
			name: "grpc url base left as-is",
			cfg: exporter.Config{
				Protocol: exporter.ProtocolGRPC,
				Endpoint: "https://host.example.com/otlp",
			},
			wantTraces:  "https://host.example.com/otlp",
			wantMetrics: "https://host.example.com/otlp",
			wantLogs:    "https://host.example.com/otlp",
		},
		{
			name: "per-signal overrides used as-is and independently",
			cfg: exporter.Config{
				Protocol:        exporter.ProtocolHTTP,
				Endpoint:        "https://base.example.com/otlp",
				TracesEndpoint:  "https://traces.example.com/custom",
				MetricsEndpoint: "metrics.example.com:4318",
			},
			wantTraces:  "https://traces.example.com/custom",
			wantMetrics: "metrics.example.com:4318",
			wantLogs:    "https://base.example.com/otlp/v1/logs",
		},
		{
			name:        "empty config falls back to default endpoint",
			cfg:         exporter.Config{Protocol: exporter.ProtocolGRPC},
			wantTraces:  "localhost:4317",
			wantMetrics: "localhost:4317",
			wantLogs:    "localhost:4317",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			traces, metrics, logs := tt.cfg.ResolveEndpoints()
			if traces != tt.wantTraces {
				t.Errorf("traces = %q, want %q", traces, tt.wantTraces)
			}
			if metrics != tt.wantMetrics {
				t.Errorf("metrics = %q, want %q", metrics, tt.wantMetrics)
			}
			if logs != tt.wantLogs {
				t.Errorf("logs = %q, want %q", logs, tt.wantLogs)
			}
		})
	}
}

func TestConfig_Validate_PerSignalEndpoints(t *testing.T) {
	t.Parallel()

	base := exporter.Config{
		Protocol:     exporter.ProtocolHTTP,
		Endpoint:     "https://host.example.com/otlp",
		Timeout:      time.Second,
		BatchSize:    512,
		BatchTimeout: time.Second,
		MaxQueueSize: 2048,
		Sampler:      "always_on",
		SamplerArg:   1,
	}

	t.Run("valid per-signal endpoints accepted", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.TracesEndpoint = "https://traces.example.com/otlp/v1/traces"
		cfg.MetricsEndpoint = "metrics.example.com:4318"
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})

	t.Run("invalid per-signal endpoint rejected", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.LogsEndpoint = "://missing-scheme"
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() = nil, want error for malformed LogsEndpoint")
		}
	})
}
