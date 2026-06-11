# xk6-otel-gen

## Table of Contents

- [Project Description](#project-description)
- [Badges](#badges)
- [Quick Start](#quick-start)
- [Features](#features)
- [Building](#building)
- [Usage](#usage)
- [Topology YAML Reference](#topology-yaml-reference)
- [Configuration](#configuration)
- [Examples](#examples)
- [Security](#security)
- [Contributing](#contributing)
- [License](#license)
- [Compatibility](#compatibility)

## Project Description

`xk6-otel-gen` is a k6 extension that synthesizes OpenTelemetry traces, metrics, and logs from a declarative YAML topology. It lets you model microservice graphs, journeys, and faults without building real services.

```yaml
journeys:
  checkout:
    weight: 1.0
    steps:
      - service: frontend
        operation: get_index
```

The extension can send OTLP/gRPC and OTLP/HTTP telemetry to collectors and can also forward k6 output metrics through the `otel-gen` output.

## Badges

[![Go 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8)](https://go.dev/)
[![License Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue)](./LICENSE)
[![CI](https://github.com/ymotongpoo/xk6-otel-gen/actions/workflows/ci.yml/badge.svg)](https://github.com/ymotongpoo/xk6-otel-gen/actions/workflows/ci.yml)

| Badge | Meaning |
|---|---|
| Go 1.25+ | Minimum supported Go toolchain |
| Apache-2.0 | Project license |
| CI | GitHub Actions build status |

## Quick Start

Build k6, deploy the minimal observability stack, and run synthetic traffic:

```bash
# 1. Build a local k6 binary with this extension.
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.

# 2. Create a local Kubernetes cluster.
kind create cluster --name xk6-otel-gen

# 3. Deploy Collector, Tempo, Prometheus, Loki, and Grafana.
kubectl apply -k examples/minimal/k8s/

# 4. Forward OTLP/gRPC into the Collector.
kubectl -n xk6-otel-gen-demo port-forward svc/otel-collector 4317:4317

# 5. Run the minimal journey and export telemetry.
./k6 run examples/minimal/script.js --out otel-gen=endpoint=localhost:4317,protocol=grpc,insecure=true
```

Open Grafana in another terminal:

```bash
kubectl -n xk6-otel-gen-demo port-forward svc/grafana 3000:3000
```

## Features

| Feature | Concrete example |
|---|---|
| Topology DSL | `services.frontend.operations[].calls[]` models service edges |
| Journey execution | `runJourney("checkout")` creates one synthetic trace |
| Fault injection | `error_rate_override`, `latency_inflation`, `disconnect`, `crash` |
| OTLP egress | gRPC on `localhost:4317` or HTTP on `localhost:4318` |
| k6 output integration | `--out otel-gen=endpoint=localhost:4317` forwards k6 output |
| JSON Schema export | `go run ./cmd/xk6-otel-gen-schema > topology.schema.json` |

Example fault:

```yaml
faults:
  - target: operation:payment.authorize_card
    kind: error_rate_override
    severity:
      probability: 1.0
      value: 0.10
```

## Building

Install xk6 and build a custom k6 binary:

```bash
go install go.k6.io/xk6/cmd/xk6@latest
xk6 build --with github.com/ymotongpoo/xk6-otel-gen
```

For local development, point xk6 at this checkout:

```bash
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.
./k6 version
```

| Build mode | Command |
|---|---|
| Remote module | `xk6 build --with github.com/ymotongpoo/xk6-otel-gen` |
| Local checkout | `xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.` |

## Usage

Import the JS module, configure OTLP, load a topology, and run journeys:

```javascript
import otelgen from "k6/x/otel-gen";

export function setup() {
  otelgen.configure({
    endpoint: "localhost:4317",
    protocol: "grpc",
    insecure: true,
  });
  return { topology: otelgen.load("./topology.yaml") };
}

export default function (data) {
  data.topology.runRandomJourney();
}
```

| API | Purpose |
|---|---|
| `otelgen.configure(opts)` | Configure OTLP endpoint, protocol, TLS, headers, batching |
| `otelgen.load(path)` | Parse and validate one topology YAML file |
| `handle.runJourney(name)` | Execute a named journey |
| `handle.runRandomJourney()` | Pick a journey by YAML weight, execute it, and return its name |
| `handle.journeyWeights()` | Return `{ name: weight }` for custom JS selection |
| `otelgen.stats()` | Return exporter success/failure counters |
| `otelgen.journeys()` | List journey names after loading |
| `handle.journeys()` | List journey names from a handle |

See [examples/minimal](./examples/minimal/) and [examples/astroshop](./examples/astroshop/) for complete scripts.

## Topology YAML Reference

A topology file has three top-level sections:

| Section | Required | Example |
|---|---|---|
| `services` | yes | `frontend`, `backend`, `database` |
| `journeys` | yes | `checkout`, `browse`, `place-order` |
| `faults` | no | `latency_inflation` on `operation:shipping.quote_shipping` |

Optional top-level `namespace` sets the default OpenTelemetry `service.namespace`
resource attribute for all services. Services can override it with
`services.<name>.namespace`. When omitted, `xk6-otel-gen` is used.

Minimal service declaration:

```yaml
services:
  backend:
    namespace: checkout
    kind: application
    replicas: 3
    language: java
    framework: spring-boot
    version: 2.5.0
    operations:
      - name: get_user
```

Journeys are selected by `weight` when using `runRandomJourney()`. Omitted
weights default to `1.0`; weights must be positive in validated topology files.

```yaml
journeys:
  browse:
    weight: 4.0
    steps:
      - service: frontend
        operation: browse_home
  checkout:
    weight: 1.0
    steps:
      - service: frontend
        operation: checkout
```

Edges support retry timing with `retries`, `retry_backoff`, and
`retry_base_delay`. Use `timeout` to cap one edge attempt and mark it as a
timeout failure when simulated latency exceeds the budget:

```yaml
calls:
  - to: { service: payment, operation: authorize_card }
    protocol: grpc
    timeout: 750ms
    retries: 2
    retry_backoff: exponential
    retry_base_delay: 100ms
```

### Faults

Faults target a service node, one operation, or one concrete edge:

| Target syntax | Scope |
|---|---|
| `node:<svc>` | all operations on one service |
| `operation:<svc>.<op>` | one service operation |
| `edge:<from_svc>.<from_op>-><to_svc>.<to_op>` | one call edge |

Supported fault kinds:

| Kind | Severity fields |
|---|---|
| `latency_inflation` | `probability`, `multiplier`, optional `add` |
| `error_rate_override` | `probability`, `value` |
| `disconnect` | `probability` |
| `crash` | `probability` |

```yaml
faults:
  - target: node:payment
    kind: latency_inflation
    severity: { probability: 0.20, multiplier: 3.0 }
  - target: operation:checkout.place_order
    kind: error_rate_override
    severity: { probability: 1.0, value: 0.05 }
  - target: edge:frontend.checkout->payment.authorize_card
    kind: disconnect
    severity: { probability: 0.01 }
  - target: operation:cart.get_cart
    kind: crash
    severity: { probability: 0.005 }
```

Export the JSON Schema for editor integration:

```bash
go run ./cmd/xk6-otel-gen-schema > topology.schema.json
go run ./cmd/xk6-otel-gen-schema -output topology.schema.json
```

## Configuration

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
  batchSize: 512,
  batchTimeout: "1s",
  maxQueueSize: 2048,
  sampler: "traceidratio",
  samplerArg: 0.1,
});
```

`sampler` accepts `always_on`, `always_off`, or `traceidratio`.
`samplerArg` is used by `traceidratio` and must be in `[0,1]`. Invalid sampler
environment values fail pipeline validation with the original
`OTEL_TRACES_SAMPLER` value and the allowed set in the error message.

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

### Built-in Metrics

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

### Sending to SaaS OTLP endpoints

The same `configure(...)` / `--out otel-gen=...` mechanism works against managed OpenTelemetry endpoints. See [examples/saas-endpoints.md](examples/saas-endpoints.md) for full per-vendor instructions.

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

**Google Cloud Observability (via a sidecar Collector)** — Google's OTLP intake requires OAuth2 / ADC, so the recommended pattern is to keep xk6-otel-gen pointed at a local Collector that handles authentication and re-exports to `telemetry.googleapis.com`. The k6 side stays unchanged (`endpoint: "localhost:4317"`).

A copy-pasteable Collector config for each vendor is in [examples/saas-endpoints.md](examples/saas-endpoints.md).

## Examples

| Example | Size | Use case |
|---|---:|---|
| [minimal](./examples/minimal/) | 3 services | First run, topology basics, local smoke test |
| [astroshop](./examples/astroshop/) | 18 services | Larger commerce graph modeled after OTel Demo v2.2.0 |

Run only the topology validation tests:

```bash
go test ./test/examples/...
```

Build the Kubernetes manifests:

```bash
kustomize build examples/minimal/k8s/ > /tmp/minimal.yaml
kustomize build examples/astroshop/k8s/ > /tmp/astroshop.yaml
```

## Security

This project distributes source code and examples, not prebuilt k6 binaries. Build your own k6 binary with xk6 so the final artifact is produced in your environment from audited inputs.

| Security choice | Rationale |
|---|---|
| No prebuilt binary | Avoids asking users to trust an opaque load-testing executable |
| Pinned demo images | Kubernetes examples use explicit image tags |
| Synthetic data only | Examples do not require production credentials or user data |
| OTLP TLS options | Configure secure endpoints through JS options or environment variables |

Example production-style endpoint:

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

The certificate files are read during pipeline validation and startup so
missing files, malformed PEM data, incomplete client certificate/key pairs, and
certificate options combined with `insecure: true` fail before traffic starts.
Header values are never included in JS-module configuration logs.

## Contributing

Keep changes scoped to the package or example being modified, run the relevant tests, and use Conventional Commits for commit messages.

```bash
go test ./...
go test -race -count=1 ./...
golangci-lint run
```

| Change type | Expected check |
|---|---|
| Topology parser | `go test ./topology/...` |
| Journey engine | `go test ./journey/...` |
| Examples | `go test ./test/examples/...` and `kustomize build examples/<name>/k8s/` |
| k6 integration | `xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.` |

## License

`xk6-otel-gen` is licensed under Apache-2.0.

```text
SPDX-License-Identifier: Apache-2.0
```

| File | Purpose |
|---|---|
| [LICENSE](./LICENSE) | Apache License 2.0 full text |
| `.go` files | SPDX header enforced by lint |

## Compatibility

| Tool | Minimum version | Purpose |
|---|---:|---|
| Go | 1.25+ | Module build and tests |
| xk6 | latest | Custom k6 binary build |
| k6 | built by xk6 | Load-test runtime |
| kubectl | 1.27+ | Apply and inspect manifests |
| kind | 0.20+ | Local Kubernetes cluster |
| Docker | latest stable | kind node runtime |

Check local versions:

```bash
go version
xk6 version
kubectl version --client
kind version
docker version
```
