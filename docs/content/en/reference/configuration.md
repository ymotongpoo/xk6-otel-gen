---
title: Configuration
weight: 2
---

The exporter can be configured from three surfaces: the JS API, the k6 output
`--out` arguments, and environment variables. This document lists every option
you can set.

Configuration is merged by priority. Higher rows override lower rows.

| Priority | Source | Example |
|---:|---|---|
| 1 | JS API | `otelgen.configure({ endpoint: "localhost:4317" })` |
| 2 | `--out` args | `--out otel-gen=endpoint=localhost:4317,protocol=grpc` |
| 3 | Environment | `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318` |
| 4 | Defaults | `localhost:4317`, gRPC, insecure false |

## Configurable options (overview)

Every option mapped to the three configuration surfaces (the `configure()` key,
the `--out otel-gen=` argument key, and the environment variable). `—` means the
option is not configurable on that surface.

| Option | JS API | `--out` arg | Environment | Type | Default | Description |
|---|---|---|---|---|---|---|
| Base endpoint | `endpoint` | `endpoint` | `OTEL_EXPORTER_OTLP_ENDPOINT` | string | `localhost:4317` | OTLP endpoint shared by all signals |
| Traces endpoint | `tracesEndpoint` | — | `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | string | — | Traces-only override |
| Metrics endpoint | `metricsEndpoint` | `metricsEndpoint` | `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | string | — | Metrics-only override |
| Logs endpoint | `logsEndpoint` | — | `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | string | — | Logs-only override |
| Protocol | `protocol` | `protocol` | `OTEL_EXPORTER_OTLP_PROTOCOL` | enum | `grpc` | `grpc` / `http` (env also accepts `http/protobuf`) |
| insecure (disable TLS) | `insecure` | `insecure` | `OTEL_EXPORTER_OTLP_INSECURE` | bool | `false` | Disable TLS |
| CA certificate | `caCert` | `caCert` | `OTEL_EXPORTER_OTLP_CERTIFICATE` | string (path) | — | CA certificate for server verification |
| Client certificate | `clientCert` | `clientCert` | `OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE` | string (path) | — | mTLS client certificate |
| Client key | `clientKey` | `clientKey` | `OTEL_EXPORTER_OTLP_CLIENT_KEY` | string (path) | — | mTLS client key |
| Headers | `headers` | `headers` | `OTEL_EXPORTER_OTLP_HEADERS` | map | — | Extra OTLP headers (syntax differs per surface) |
| Compression | `compression` | `compression` | `OTEL_EXPORTER_OTLP_COMPRESSION` | enum | `""` (none) | `""` or `gzip` |
| Timeout | `timeout` | `timeout` | `OTEL_EXPORTER_OTLP_TIMEOUT` | duration | `10s` | Export timeout |
| Batch size | `batchSize` | `batchSize` | — | int | `512` | Max spans per export request |
| Batch timeout | `batchTimeout` | `batchTimeout` | — | duration | `1s` | Max wait before a batch is flushed |
| Max queue size | `maxQueueSize` | `maxQueueSize` | — | int | `2048` | Spans buffered before drops begin |
| Sampler | `sampler` | — | `OTEL_TRACES_SAMPLER` | enum | `always_on` | `always_on` / `always_off` / `traceidratio` |
| Sampler arg | `samplerArg` | — | `OTEL_TRACES_SAMPLER_ARG` | number | `1` | Ratio for `traceidratio` `[0,1]` |
| Resource overrides | `resourceOverrides` | — | — | map | — | Override / add resource attributes |
| Output queue size | — | `queueSize` | — | int | `100` | k6 output internal queue (`[10, 10000]`, output-only) |

{{< callout type="info" >}}
The environment variables `HEADERS`, `PROTOCOL`, `COMPRESSION`, `TIMEOUT`,
`INSECURE`, `CERTIFICATE`, `CLIENT_CERTIFICATE`, and `CLIENT_KEY` also have
per-signal variants (`OTEL_EXPORTER_OTLP_TRACES_*`, `OTEL_EXPORTER_OTLP_METRICS_*`,
`OTEL_EXPORTER_OTLP_LOGS_*`), and the per-signal variant takes precedence over the
base `OTEL_EXPORTER_OTLP_*`.
{{< /callout >}}

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
  compression: "gzip",
  timeout: "10s",
  // Batch/queue headroom. Defaults in parentheses. See "Throughput, batching,
  // and dropped root spans" below.
  batchSize: 2048,        // (default 512)
  batchTimeout: "1s",     // (default 1s)
  maxQueueSize: 16384,    // (default 2048)
  sampler: "traceidratio",
  samplerArg: 0.1,
});
```

## Field details

### Endpoints

There are two ways to point the exporter at a destination, following the
[OTLP exporter specification](https://opentelemetry.io/docs/specs/otel/protocol/exporter/#endpoint-urls-for-otlphttp):

1. **Base endpoint** — set a single `endpoint`. For HTTP, the per-signal path is
   appended automatically: `v1/traces`, `v1/metrics`, `v1/logs`. For example,
   `https://otlp-gateway.example.com/otlp` sends traces to
   `https://otlp-gateway.example.com/otlp/v1/traces`. gRPC and `host:port`
   endpoints are used unchanged.
2. **Per-signal endpoints** — set `tracesEndpoint`, `metricsEndpoint` and/or
   `logsEndpoint`. These are used **as-is** with no path completion and take
   precedence over the base `endpoint` for the matching signal.

| Surface | Base | Per-signal |
|---|---|---|
| JS API | `endpoint` | `tracesEndpoint`, `metricsEndpoint`, `logsEndpoint` |
| `--out` args | `endpoint` | `metricsEndpoint` (this output emits metrics only) |
| Environment | `OTEL_EXPORTER_OTLP_ENDPOINT` | `OTEL_EXPORTER_OTLP_{TRACES,METRICS,LOGS}_ENDPOINT` |

An endpoint must be in `host:port` form or `scheme://host[:port]` form.

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

**`protocol`** is `grpc` (default) or `http` (OTLP/HTTP/protobuf). The environment
variable also accepts `http/protobuf`.

{{< callout type="warning" >}}
**Breaking change (per-signal endpoint support):** URL-form base endpoints (those
with a `scheme://`) now have `v1/{signal}` appended for HTTP. Previously the URL
path was sent as-is. If you relied on the old behavior — e.g. setting
`endpoint: "https://host:4318/v1/traces"` — move that value to the matching
per-signal key (`tracesEndpoint`), which is used as-is.
{{< /callout >}}

### Authentication and TLS

- **`insecure`** — `true` disables TLS (plaintext). Cannot be combined with any
  certificate option.
- **`caCert`** — path to a CA certificate (PEM) used to verify the server
  certificate. Appended to the system certificate pool.
- **`clientCert` / `clientKey`** — paths to the mTLS client certificate and key.
  Must be configured **together**; setting only one is a validation error.
- **`headers`** — extra headers added to OTLP requests. Keys must match
  `[A-Za-z0-9_-]+` and values must be non-empty. The syntax differs per surface:
  - JS API: object `{ "x-key": "value" }`
  - `--out` args: `headers=key1:value1;key2:value2` (`;`-separated, `:` between key and value)
  - Environment: `OTEL_EXPORTER_OTLP_HEADERS=key1=value1,key2=value2` (`,`-separated, `=` delimiter, values are URL-decoded)
- **`compression`** — `""` (none, default) or `gzip`.

Certificate files are read during pipeline validation and startup, so missing
files, malformed PEM data, incomplete client certificate/key pairs, and
certificate options combined with `insecure: true` fail before traffic starts.
The minimum TLS version is 1.2. Header values are never included in JS-module
configuration logs.

```javascript
otelgen.configure({
  endpoint: "otel-collector.example.internal:4317",
  protocol: "grpc",
  insecure: false,
  caCert: "/etc/otel/ca.pem",
  clientCert: "/etc/otel/client.pem",
  clientKey: "/etc/otel/client-key.pem",
  headers: { authorization: "Bearer ${TOKEN}" },
});
```

### Batching and queueing

- **`timeout`** — timeout for one OTLP export call. Default `10s`. In the JS API a
  number is milliseconds, or a Go duration string (e.g. `"10s"`); the `--out` arg
  is a duration string; the env var `OTEL_EXPORTER_OTLP_TIMEOUT` is milliseconds.
- **`batchSize`** — max spans per OTLP export request. Must be <= `maxQueueSize`.
  Default `512`.
- **`batchTimeout`** — max time a span waits before its batch is flushed. Default `1s`.
- **`maxQueueSize`** — spans buffered before drops begin. Default `2048`.

#### Throughput, batching, and dropped root spans

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

### Sampling

- **`sampler`** — one of `always_on` (default) / `always_off` / `traceidratio`.
- **`samplerArg`** — used by `traceidratio`, must be in `[0,1]`. Default `1`.

Invalid sampler environment values fail pipeline validation with the original
`OTEL_TRACES_SAMPLER` value and the allowed set in the error message.

Sampling applies to traces only. Metrics and logs are still emitted; logs keep
the active trace context even when the trace sampler drops spans.

### Resource attributes

- **`resourceOverrides`** — a map that overrides or adds resource attributes
  (JS API only). Keys must be non-empty and values must be string-coercible.

```javascript
otelgen.configure({
  endpoint: "localhost:4317",
  resourceOverrides: {
    "deployment.environment": "staging",
    "service.version": "1.2.3",
  },
});
```

### k6 output specific

- **`queueSize`** — internal queue size of the `otel-gen` k6 output. Range
  `[10, 10000]`, default `100`. Configurable only via `--out` args.

```bash
./k6 run script.js --out otel-gen=endpoint=localhost:4317,protocol=grpc,insecure=true,queueSize=100
./k6 run script.js --out otel-gen=endpoint=otel.example.com:4317,protocol=grpc,caCert=/etc/otel/ca.pem,clientCert=/etc/otel/client.pem,clientKey=/etc/otel/client-key.pem
```

## Built-in Metrics

The JS module publishes exporter counters as native k6 metrics after journey
runs: `otel_gen_traces_exported`, `otel_gen_traces_failed`,
`otel_gen_metrics_exported`, `otel_gen_metrics_failed`,
`otel_gen_logs_exported`, `otel_gen_logs_failed`, and
`otel_gen_queue_drops`. Queue drops are scoped to the JS-module pipeline metric;
the `otel-gen` k6 output logs its final queue drop count on `Stop()`.

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
