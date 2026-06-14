# xk6-otel-gen

[![Go 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8)](https://go.dev/)
[![License Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue)](./LICENSE)
[![CI](https://github.com/ymotongpoo/xk6-otel-gen/actions/workflows/ci.yml/badge.svg)](https://github.com/ymotongpoo/xk6-otel-gen/actions/workflows/ci.yml)
[![Docs](https://github.com/ymotongpoo/xk6-otel-gen/actions/workflows/docs.yml/badge.svg)](https://ymotongpoo.github.io/xk6-otel-gen/)

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

## 📖 Documentation

Full documentation is published at **<https://ymotongpoo.github.io/xk6-otel-gen/>**:

- [Getting Started](https://ymotongpoo.github.io/xk6-otel-gen/getting-started/) — features and first run
- [Quick Start](https://ymotongpoo.github.io/xk6-otel-gen/getting-started/quick-start/)
- [Building](https://ymotongpoo.github.io/xk6-otel-gen/getting-started/building/) — build a custom k6 binary
- [Usage](https://ymotongpoo.github.io/xk6-otel-gen/usage/) — the JavaScript API
- [Topology YAML Reference](https://ymotongpoo.github.io/xk6-otel-gen/reference/topology/)
- [Configuration](https://ymotongpoo.github.io/xk6-otel-gen/reference/configuration/)
- [Examples](https://ymotongpoo.github.io/xk6-otel-gen/examples/)
- [Security](https://ymotongpoo.github.io/xk6-otel-gen/about/security/),
  [Contributing](https://ymotongpoo.github.io/xk6-otel-gen/about/contributing/),
  [Compatibility](https://ymotongpoo.github.io/xk6-otel-gen/about/compatibility/)

The documentation source lives in [`docs/`](./docs/) (Hugo + Hextra). See
[`docs/README.md`](./docs/README.md) for how to build the site locally.

## Quick Start

Build a local k6 binary with this extension and run the minimal journey:

```bash
# Build a local k6 binary with this extension.
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.

# Run the minimal journey and export telemetry to a local Collector.
./k6 run examples/minimal/script.js --out otel-gen=endpoint=localhost:4317,protocol=grpc,insecure=true
```

See the [Quick Start guide](https://ymotongpoo.github.io/xk6-otel-gen/getting-started/quick-start/)
for the full local observability stack (kind, Collector, Tempo, Prometheus, Loki, Grafana).

## Examples

| Example | Size | Use case |
|---|---:|---|
| [minimal](./examples/minimal/) | 3 services | First run, topology basics, local smoke test |
| [astroshop](./examples/astroshop/) | 18 services | Larger commerce graph modeled after OTel Demo v2.2.0 |

## License

`xk6-otel-gen` is licensed under [Apache-2.0](./LICENSE).

```text
SPDX-License-Identifier: Apache-2.0
```
