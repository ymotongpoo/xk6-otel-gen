---
title: Topology YAML Reference
weight: 1
---

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

## Faults

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
