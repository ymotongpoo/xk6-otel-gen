// SPDX-License-Identifier: Apache-2.0

package exporter_test

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"pgregory.net/rapid"
)

// urlBaseEndpoint draws a URL-form base endpoint with an optional path, query
// and fragment, exercising appendSignalPath's path-handling branches.
func urlBaseEndpoint(t *rapid.T, label string) string {
	scheme := rapid.SampledFrom([]string{"http", "https"}).Draw(t, label+"_scheme")
	host := rapid.StringMatching(`^[a-z][a-z0-9-]{2,20}\.example\.com$`).Draw(t, label+"_host")
	port := rapid.IntRange(1, 65535).Draw(t, label+"_port")
	path := rapid.SampledFrom([]string{
		"", "/", "/otlp", "/otlp/", "/v1/traces", "/a/b/c",
	}).Draw(t, label+"_path")
	suffix := ""
	if rapid.Bool().Draw(t, label+"_query") {
		suffix += "?token=abc&x=1"
	}
	if rapid.Bool().Draw(t, label+"_fragment") {
		suffix += "#frag"
	}
	return fmt.Sprintf("%s://%s:%d%s%s", scheme, host, port, path, suffix)
}

// TP-U4-5: appendSignalPath structure preservation (verified through the public
// ResolveEndpoints contract over HTTP URL-form base endpoints).
func TestResolveEndpoints_PathStructure_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		base := urlBaseEndpoint(t, "base")
		cfg := exporter.Config{Protocol: exporter.ProtocolHTTP, Endpoint: base}
		resolved := map[string]string{}
		resolved["traces"], resolved["metrics"], resolved["logs"] = cfg.ResolveEndpoints()

		baseURL, err := url.Parse(base)
		if err != nil {
			t.Fatalf("base %q failed to parse: %v", base, err)
		}
		basePathNorm := strings.TrimSuffix(baseURL.Path, "/")

		for signal, got := range resolved {
			gotURL, err := url.Parse(got)
			if err != nil {
				t.Fatalf("%s: resolved %q failed to parse: %v", signal, got, err)
			}
			suffix := "/v1/" + signal
			// P1: path ends with /v1/{signal}.
			if !strings.HasSuffix(gotURL.Path, suffix) {
				t.Fatalf("%s: path %q does not end with %q", signal, gotURL.Path, suffix)
			}
			// P2: scheme/host/query/fragment preserved.
			if gotURL.Scheme != baseURL.Scheme || gotURL.Host != baseURL.Host ||
				gotURL.RawQuery != baseURL.RawQuery || gotURL.Fragment != baseURL.Fragment {
				t.Fatalf("%s: non-path components changed: base=%q got=%q", signal, base, got)
			}
			// P3: normalized base path is a prefix of the resolved path.
			if !strings.HasPrefix(gotURL.Path, basePathNorm) {
				t.Fatalf("%s: base path %q is not a prefix of %q", signal, basePathNorm, gotURL.Path)
			}
			// P4: appended exactly once.
			if gotURL.Path != basePathNorm+suffix {
				t.Fatalf("%s: path %q != %q (single append)", signal, gotURL.Path, basePathNorm+suffix)
			}
		}
	})
}

// TP-U4-6: resolution precedence and per-signal independence.
func TestResolveEndpoints_Precedence_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		cfg := generators.ValidConfig().Draw(t, "cfg")
		traces, metrics, logs := cfg.ResolveEndpoints()
		got := map[string]string{"traces": traces, "metrics": metrics, "logs": logs}
		perSignal := map[string]string{
			"traces":  cfg.TracesEndpoint,
			"metrics": cfg.MetricsEndpoint,
			"logs":    cfg.LogsEndpoint,
		}
		base := cfg.Endpoint

		for signal, resolved := range got {
			if ps := perSignal[signal]; ps != "" {
				// P1: per-signal override used verbatim.
				if resolved != ps {
					t.Fatalf("%s: resolved %q, want per-signal %q", signal, resolved, ps)
				}
				continue
			}
			if cfg.Protocol == exporter.ProtocolHTTP && strings.Contains(base, "://") {
				// P2: HTTP + URL base => v1/{signal} appended.
				if !strings.HasSuffix(mustPath(t, resolved), "/v1/"+signal) {
					t.Fatalf("%s: resolved %q lacks /v1/%s suffix", signal, resolved, signal)
				}
			} else {
				// P3: gRPC or host:port base => base used unchanged.
				if resolved != base {
					t.Fatalf("%s: resolved %q, want base %q", signal, resolved, base)
				}
			}
		}

		// P4: independence — blanking the other signals' overrides must not
		// change a given signal's resolution.
		isolated := cfg
		isolated.MetricsEndpoint = ""
		isolated.LogsEndpoint = ""
		tracesOnly, _, _ := isolated.ResolveEndpoints()
		if tracesOnly != traces {
			t.Fatalf("traces resolution changed after blanking other signals: %q vs %q", tracesOnly, traces)
		}
	})
}

func mustPath(t *rapid.T, raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u.Path
}

// TP-U4-7: ConfigFromEnv maps per-signal ENDPOINT env vars to per-signal fields
// and the base ENDPOINT to Endpoint, without cross-contamination.
func TestConfigFromEnv_PerSignalEndpoints_Property(t *testing.T) {
	endpointEnvVars := []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
	}

	rapid.Check(t, func(t *rapid.T) {
		// Clear all endpoint env vars for a clean slate each iteration.
		for _, name := range endpointEnvVars {
			os.Unsetenv(name)
		}

		want := map[string]string{}
		for _, name := range endpointEnvVars {
			if rapid.Bool().Draw(t, name+"_set") {
				value := validExporterEndpointStr(t, name)
				os.Setenv(name, value)
				want[name] = value
			}
		}

		cfg := exporter.ConfigFromEnv()

		// P1+P2: each field reflects exactly its own env var.
		if cfg.Endpoint != want["OTEL_EXPORTER_OTLP_ENDPOINT"] {
			t.Fatalf("Endpoint = %q, want %q", cfg.Endpoint, want["OTEL_EXPORTER_OTLP_ENDPOINT"])
		}
		if cfg.TracesEndpoint != want["OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"] {
			t.Fatalf("TracesEndpoint = %q, want %q", cfg.TracesEndpoint, want["OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"])
		}
		if cfg.MetricsEndpoint != want["OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"] {
			t.Fatalf("MetricsEndpoint = %q, want %q", cfg.MetricsEndpoint, want["OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"])
		}
		if cfg.LogsEndpoint != want["OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"] {
			t.Fatalf("LogsEndpoint = %q, want %q", cfg.LogsEndpoint, want["OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"])
		}
	})

	for _, name := range endpointEnvVars {
		os.Unsetenv(name)
	}
}

func validExporterEndpointStr(t *rapid.T, label string) string {
	scheme := rapid.SampledFrom([]string{"http", "https"}).Draw(t, label+"_scheme")
	host := rapid.StringMatching(`^[a-z][a-z0-9-]{2,20}\.example\.com$`).Draw(t, label+"_host")
	port := rapid.IntRange(1, 65535).Draw(t, label+"_port")
	return fmt.Sprintf("%s://%s:%d/otlp", scheme, host, port)
}
