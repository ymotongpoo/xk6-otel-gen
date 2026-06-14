---
title: Configuration
weight: 2
---

Configuration is merged by priority. Higher rows override lower rows.

| Priority | Source | Example |
|---:|---|---|
| 1 | JS API | `otelgen.configure({ endpoint: "localhost:4317" })` |
| 2 | `--out` args | `--out otel-gen=endpoint=localhost:4317,protocol=grpc` |
| 3 | Environment | `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318` |
| 4 | Defaults | `localhost:4317`, gRPC, insecure false |

Common JS configuration:

```javascript
otelgen.configure({
  endpoint: "localhost:4317",
  protocol: "grpc",
  insecure: true,
  caCert: "/etc/otel/ca.pem",
  clientCert: "/etc/otel/client.pem",
  clientKey: "/etc/otel/client-key.pem",
  headers: { "x-demo": "minimal" },
  timeout: "10s",
  // Batch/queue headroom. Defaults shown in parentheses; the values below are
  // generous, suited to sustained load. See "Throughput, batching, and
  // dropped root spans" below.
  batchSize: 2048,        // (default 512)
  batchTimeout: "1s",     // (default 1s)
  maxQueueSize: 16384,    // (default 2048)
  sampler: "traceidratio",
  samplerArg: 0.1,
});
```

`sampler` accepts `always_on`, `always_off`, or `traceidratio`.
`samplerArg` is used by `traceidratio` and must be in `[0,1]`. Invalid sampler
environment values fail pipeline validation with the original
`OTEL_TRACES_SAMPLER` value and the allowed set in the error message.

## Throughput, batching, and dropped root spans

The synthesizer emits one trace per journey iteration, as fast as k6 runs
iterations. With a `constant-vus` executor and no think time, a single VU can
drive **10,000+ iterations/s**, far more telemetry than most backends — or the
OTLP exporter — can absorb.

When generation outpaces export, the trace `BatchSpanProcessor` queue fills and
**spans are dropped**. These drops happen *before* the exporter, so they are
**not** counted in `otelgen.stats().tracesFailed`. Crucially, a trace's **root
span ends after all of its children**, so it is enqueued last and is the first
casualty when the queue overflows. The backend then receives child spans but no
root, and Grafana Tempo shows `<root span not yet received>` in the Service
column of the trace list.

Two independent controls address this:

**1. Cap the generation rate.** Use a rate-based executor so journeys are
produced at a fixed, ingestable rate instead of at full CPU speed:

```javascript
export const options = {
  scenarios: {
    checkout: {
      executor: "constant-arrival-rate",
      rate: 300,            // journeys/s; × spans-per-journey ≈ backend span rate
      timeUnit: "1s",
      duration: "30s",
      preAllocatedVUs: 20,
      maxVUs: 100,
    },
  },
};
```

Pick `rate` so that `rate × spans-per-journey` stays within your backend's
ingest budget. The bundled examples target roughly **1,000 spans/s**: the
minimal journey emits 3 spans, so it runs at `rate: 300`.

**2. Size the exporter queue and batches.** Give the batch processor enough
headroom to absorb bursts without dropping:

| Option | Default | Generous | Effect |
|---|---:|---:|---|
| `maxQueueSize` | 2048 | 16384 | Spans buffered before drops begin. Raise this first to stop drops. |
| `batchSize` | 512 | 2048 | Max spans per OTLP export request. Must be ≤ `maxQueueSize`. |
| `batchTimeout` | 1s | 1s | Max time a span waits before its batch is flushed. Lower values reduce how long a root span lags its children. |

Always call `otelgen.flush()` in `teardown()` (see [Usage]({{< relref "/usage" >}})) so the final
batch — which contains the most recent root spans — is delivered before the
process exits.

## Endpoint resolution

There are two ways to point the exporter at a destination, following the
[OTLP exporter specification](https://opentelemetry.io/docs/specs/otel/protocol/exporter/#endpoint-urls-for-otlphttp):

1. **Base endpoint** — set a single `endpoint`. For HTTP, the per-signal path
   is appended automatically: `v1/traces`, `v1/metrics`, `v1/logs`. For
   example, `https://otlp-gateway.example.com/otlp` sends traces to
   `https://otlp-gateway.example.com/otlp/v1/traces`. gRPC and `host:port`
   endpoints are used unchanged (the SDK applies its own per-signal path).
2. **Per-signal endpoints** — set `tracesEndpoint`, `metricsEndpoint` and/or
   `logsEndpoint`. These are used **as-is** with no path completion and take
   precedence over the base `endpoint` for the matching signal.

| Surface | Base | Per-signal |
|---|---|---|
| JS API | `endpoint` | `tracesEndpoint`, `metricsEndpoint`, `logsEndpoint` |
| `--out` args | `endpoint` | `metricsEndpoint` (this output emits metrics only) |
| Environment | `OTEL_EXPORTER_OTLP_ENDPOINT` | `OTEL_EXPORTER_OTLP_{TRACES,METRICS,LOGS}_ENDPOINT` |

```javascript
otelgen.configure({
  // Base endpoint: v1/{signal} is appended for HTTP.
  endpoint: "https://otlp-gateway-prod-us-central-0.grafana.net/otlp",
  protocol: "http",
  // Optional per-signal overrides (used as-is, no path completion):
  // tracesEndpoint: "https://traces.example.com/v1/traces",
  // metricsEndpoint: "https://metrics.example.com/v1/metrics",
  // logsEndpoint: "https://logs.example.com/v1/logs",
});
```

{{< callout type="warning" >}}
**Breaking change (per-signal endpoint support):** URL-form base endpoints
(those with a `scheme://`) now have `v1/{signal}` appended for HTTP. Previously
the URL path was sent as-is. If you relied on the old behavior — e.g. setting
`endpoint: "https://host:4318/v1/traces"` — move that value to the matching
per-signal key (`tracesEndpoint`), which is used as-is.
{{< /callout >}}

TLS certificate options can be supplied through JS (`caCert`, `clientCert`,
`clientKey`), `--out` args with the same keys, or OTEL environment variables:
`OTEL_EXPORTER_OTLP_CERTIFICATE`,
`OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE`, and `OTEL_EXPORTER_OTLP_CLIENT_KEY`
including signal-specific variants such as
`OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE`. `clientCert` and `clientKey` must be
configured together. Certificate options cannot be combined with
`insecure: true`.

Sampling applies to traces only. Metrics and logs are still emitted; logs keep
the active trace context even when the trace sampler drops spans.

## Built-in Metrics

The JS module publishes exporter counters as native k6 metrics after journey
runs: `otel_gen_traces_exported`, `otel_gen_traces_failed`,
`otel_gen_metrics_exported`, `otel_gen_metrics_failed`,
`otel_gen_logs_exported`, `otel_gen_logs_failed`, and
`otel_gen_queue_drops`. Queue drops are scoped to the JS-module pipeline metric;
the `otel-gen` k6 output logs its final queue drop count on `Stop()`.

Common output configuration:

```bash
./k6 run script.js --out otel-gen=endpoint=localhost:4317,protocol=grpc,insecure=true,queueSize=100
./k6 run script.js --out otel-gen=endpoint=otel.example.com:4317,protocol=grpc,caCert=/etc/otel/ca.pem,clientCert=/etc/otel/client.pem,clientKey=/etc/otel/client-key.pem
```

## Sending to SaaS OTLP endpoints

The same `configure(...)` / `--out otel-gen=...` mechanism works against managed
OpenTelemetry endpoints. See
[examples/saas-endpoints.md](https://github.com/ymotongpoo/xk6-otel-gen/blob/main/examples/saas-endpoints.md)
for full per-vendor instructions.

**Grafana Cloud (OTLP gateway, HTTP/protobuf)**:

```javascript
otelgen.configure({
  endpoint: "https://otlp-gateway-prod-us-central-0.grafana.net/otlp",
  protocol: "http",
  insecure: false,
  headers: {
    // base64("<instance_id>:<api_token>")
    Authorization: `Basic ${__ENV.GRAFANA_CLOUD_OTLP_TOKEN}`,
  },
});
```

**Google Cloud Observability (via a sidecar Collector)** — Google's OTLP intake
requires OAuth2 / ADC, so the recommended pattern is to keep xk6-otel-gen pointed
at a local Collector that handles authentication and re-exports to
`telemetry.googleapis.com`. The k6 side stays unchanged (`endpoint: "localhost:4317"`).

A copy-pasteable Collector config for each vendor is in
[examples/saas-endpoints.md](https://github.com/ymotongpoo/xk6-otel-gen/blob/main/examples/saas-endpoints.md).
