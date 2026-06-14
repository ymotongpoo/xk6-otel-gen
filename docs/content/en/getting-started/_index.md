---
title: Getting Started
weight: 1
prev: /
next: /getting-started/quick-start
---

`xk6-otel-gen` is a k6 extension that synthesizes OpenTelemetry traces, metrics,
and logs from a declarative YAML topology. It lets you model microservice graphs,
journeys, and faults without building real services.

```yaml
journeys:
  checkout:
    weight: 1.0
    steps:
      - service: frontend
        operation: get_index
```

The extension can send OTLP/gRPC and OTLP/HTTP telemetry to collectors and can
also forward k6 output metrics through the `otel-gen` output.

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

## Next steps

- [Quick Start]({{< relref "/getting-started/quick-start" >}}) — build k6 and run synthetic traffic.
- [Building]({{< relref "/getting-started/building" >}}) — build a custom k6 binary with this extension.
- [Usage]({{< relref "/usage" >}}) — the JavaScript API.
