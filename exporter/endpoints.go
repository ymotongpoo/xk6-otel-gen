// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"net/url"
	"strings"
)

// Per-signal OTLP/HTTP path suffixes appended to a base endpoint.
const (
	tracesSignalPath  = "v1/traces"
	metricsSignalPath = "v1/metrics"
	logsSignalPath    = "v1/logs"
)

// ResolveEndpoints returns the effective destination endpoint for each signal.
//
// Resolution is independent per signal and follows the OTLP exporter spec
// (https://opentelemetry.io/docs/specs/otel/protocol/exporter/#endpoint-urls-for-otlphttp):
//
//   - A non-empty per-signal endpoint (TracesEndpoint/MetricsEndpoint/
//     LogsEndpoint) is used as-is with no path completion.
//   - Otherwise the base Endpoint is used. For HTTP with a URL-form base
//     endpoint, the per-signal path v1/{signal} is appended to the base path.
//     For gRPC, or a host:port base endpoint, the base is returned unchanged
//     (the SDK applies its own default path for host:port HTTP endpoints).
//
// The returned values are the single source of truth shared by exporter
// construction and startup logging.
func (c Config) ResolveEndpoints() (traces, metrics, logs string) {
	base := c.Endpoint
	if base == "" {
		base = defaultEndpoint
	}
	traces = resolveSignalEndpoint(c.TracesEndpoint, base, c.Protocol, tracesSignalPath)
	metrics = resolveSignalEndpoint(c.MetricsEndpoint, base, c.Protocol, metricsSignalPath)
	logs = resolveSignalEndpoint(c.LogsEndpoint, base, c.Protocol, logsSignalPath)
	return traces, metrics, logs
}

// resolveSignalEndpoint resolves a single signal's endpoint from its optional
// per-signal override and the shared base endpoint.
func resolveSignalEndpoint(perSignal, base string, protocol Protocol, signalPath string) string {
	if perSignal != "" {
		return perSignal
	}
	if protocol == ProtocolHTTP && endpointIsURL(base) {
		return appendSignalPath(base, signalPath)
	}
	return base
}

// appendSignalPath appends the OTLP per-signal path to a URL-form base endpoint,
// preserving its scheme, host, query and fragment. The signal path is appended
// to the existing path exactly once; no de-duplication is performed, matching
// the OTLP spec's strict base-endpoint semantics.
func appendSignalPath(base, signalPath string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	switch {
	case u.Path == "" || u.Path == "/":
		u.Path = "/" + signalPath
	case strings.HasSuffix(u.Path, "/"):
		u.Path += signalPath
	default:
		u.Path += "/" + signalPath
	}
	return u.String()
}
