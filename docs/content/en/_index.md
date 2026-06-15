---
title: xk6-otel-gen
# type: docs makes this home page render with Hextra's docs layout (sidebar +
# TOC), and roots the sidebar at the site home so every top-level section is
# listed on every page.
type: docs
# cascade type:docs to all descendants so every page uses the docs layout.
cascade:
  type: docs
---

`xk6-otel-gen` is a [k6](https://k6.io/) extension that synthesizes OpenTelemetry
traces, metrics, and logs from a declarative YAML topology. Instead of building
and deploying real microservices, you describe the service graph, user journeys,
and faults in YAML, and it generates correlated OpenTelemetry signals and sends
them to any collector or backend.

## The problem it solves

Validating an observability pipeline — backends, dashboards, alerts — usually
requires real services that emit telemetry. But standing up a fleet of
microservices, driving load through them, and reproducing realistic failures
just to generate that telemetry is slow and expensive.

`xk6-otel-gen` replaces the *producer* of telemetry with a synthetic one. You
declare a service graph and user journeys in YAML, and k6 executes them to emit
correlated traces, metrics, and logs — no real services needed.

## Highlights

- **Declarative topology** — model service edges, journeys, and faults in YAML. No real backends required.
- **Correlated signals** — emit traces, metrics, logs, and profiles that share trace context, with metric exemplars (`trace_id` / `span_id`) and producer↔consumer span links for messaging.
- **Per-operation telemetry** — declare structured `log_events`, custom `metrics`, and Pyroscope `profile` flamegraphs directly on operations, with fault-linked values for realistic incidents.
- **Fault injection** — reproduce latency inflation, error-rate overrides, disconnects, and crashes probabilistically.
- **OTLP egress** — send over OTLP/gRPC or OTLP/HTTP to any collector or SaaS endpoint (e.g. Grafana Cloud).
- **k6-native** — control generation rate and scale with k6 executors, and forward k6 output metrics too.

## What it is good for

- Validating and demoing observability backends (Tempo, Prometheus, Loki, Grafana, …).
- Exercising collector pipelines and sampling configurations.
- Building dashboards and alerts against realistic data without real services.
- Benchmarking backend ingest capacity and scale.

## How it works

1. Describe services, journeys, and faults in a single YAML topology.
2. Build a k6 binary that includes this extension.
3. Load the topology from a k6 script and run journeys. Each journey execution
   becomes one trace (plus related metrics and logs).
4. Telemetry is exported over OTLP to your collector or backend.

## Next steps

- [Getting Started]({{< relref "/getting-started" >}}) — features and your first run.
- [Quick Start]({{< relref "/getting-started/quick-start" >}}) — build k6 and send synthetic traffic.
- [Topology YAML Reference]({{< relref "/reference/topology" >}}) — every configurable field.
- [Configuration]({{< relref "/reference/configuration" >}}) — all exporter / k6 options.
