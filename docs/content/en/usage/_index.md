---
title: Usage
weight: 2
---

Import the JS module, configure OTLP, load a topology, and run journeys:

```javascript
import otelgen from "k6/x/otel-gen";

export function setup() {
  otelgen.configure({
    endpoint: "localhost:4317",
    protocol: "grpc",
    insecure: true,
  });
}

export default function () {
  const topology = otelgen.load("./topology.yaml");
  topology.runRandomJourney();
}

export function teardown() {
  otelgen.flush();
}
```

Call `load()` inside `default()`, not in `setup()`: k6 JSON-serializes
`setup()` return values, which strips the handle's methods. `load()` parses
and validates the YAML only once per test run and returns the cached handle
on every subsequent call, so calling it per iteration adds no overhead.

Call `otelgen.flush()` in `teardown()`. Each trace's root span ends after all
of its children, so it is the last span to enter the batch queue; without a
final flush it is dropped at process exit and backends report
"root span not yet received". `flush()` makes trace, metric, and log delivery
independent of whether the `otel-gen` output is enabled — it force-flushes the
batch processors without closing the exporters, so it is safe to call with or
without `--out otel-gen=...` (when the output is enabled, its `Stop` hook still
performs the final pipeline shutdown).

| API | Purpose |
|---|---|
| `otelgen.configure(opts)` | Configure OTLP endpoint, protocol, TLS, headers, batching |
| `otelgen.load(path)` | Parse and validate one topology YAML file |
| `handle.runJourney(name)` | Execute a named journey |
| `handle.runRandomJourney()` | Pick a journey by YAML weight, execute it, and return its name |
| `handle.journeyWeights()` | Return `{ name: weight }` for custom JS selection |
| `otelgen.flush()` | Force-flush queued telemetry (call in `teardown()` so root spans are delivered) |
| `otelgen.stats()` | Return exporter success/failure counters |
| `otelgen.journeys()` | List journey names after loading |
| `handle.journeys()` | List journey names from a handle |

## Signals and features

Every journey execution produces correlated OpenTelemetry signals that share a
trace context:

- **Traces** — one trace per journey, with a span per operation and call.
  `messaging` edges additionally emit a PRODUCER (publish) and a CONSUMER
  (receive) span connected by a span link.
- **Metrics** — built-in request/duration instruments plus per-operation custom
  metrics (counter / gauge / histogram). Histogram metrics carry exemplars
  (`trace_id` / `span_id`) for metrics→traces drill-down.
- **Logs** — a per-operation log plus declarative structured log events with an
  `event.name`.
- **Profiles** — synthetic pprof flamegraphs pushed to Pyroscope when
  [`profilesEndpoint`]({{< relref "/reference/configuration" >}}) is set.

These are driven entirely from the topology. Each operation can declare:

| Field | Emits |
|---|---|
| `log_events` | structured logs (name, severity, condition, body, attributes) |
| `metrics` | custom counters / gauges / histograms, optionally fault-linked |
| `profile` | a baseline / incident flamegraph for diff profiling, fault-linked |

See the [Topology YAML Reference]({{< relref "/reference/topology" >}}) for the
full syntax. Custom metrics and profiles can react to active faults, so an
incident changes the emitted values and stacks deterministically.

See the [minimal](https://github.com/ymotongpoo/xk6-otel-gen/tree/main/examples/minimal)
and [astroshop](https://github.com/ymotongpoo/xk6-otel-gen/tree/main/examples/astroshop)
examples for complete scripts.
