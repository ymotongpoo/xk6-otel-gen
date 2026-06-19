// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

// Package k6otelgen registers the "k6/x/otel-gen" k6 extension module.
//
// JavaScript usage:
//
//	import otelgen from "k6/x/otel-gen";
//
//	export function setup() {
//	    otelgen.configure({
//	        endpoint: "localhost:4317",
//	        protocol: "grpc",
//	        insecure: true,
//	    });
//	}
//
//	export default function () {
//	    const topology = otelgen.load("./topology.yaml");
//	    topology.runJourney("checkout");
//	}
//
//	export function teardown() {
//	    otelgen.flush();
//	}
//
// IMPORTANT: run k6 with --out otel-gen=... so the exporter Pipeline is shut
// down by the k6 output lifecycle. Without the output module, unsent OTLP
// batches may be lost when the k6 process exits:
//
//	k6 run --out otel-gen=endpoint=localhost:4317 script.js
//
// Endpoint resolution:
//
//   - endpoint is the base endpoint. For HTTP it follows the OTLP exporter
//     spec: the per-signal path v1/{signal} is appended (for example
//     "https://host/otlp" sends traces to "https://host/otlp/v1/traces").
//     gRPC and host:port endpoints are used unchanged.
//   - tracesEndpoint, metricsEndpoint and logsEndpoint optionally override a
//     single signal. They are used as-is (no path completion) and take
//     precedence over endpoint for that signal.
//
// State model:
//
//   - Process singleton: topology schema, fault overlay, and shared exporter
//     Pipeline. The schema is loaded once through otelgen.load.
//   - Per VU: journey Engine, Synthesizer, and TopologyHandle. These are bound
//     to the VU context used by topology.runJourney.
package k6otelgen
