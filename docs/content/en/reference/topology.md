---
title: Topology YAML Reference
weight: 1
---

A topology file declares the synthetic microservice topology that `xk6-otel-gen`
uses to generate OpenTelemetry signals. This document lists every field you can
set in YAML.

## Top level

| Key | Type | Required | Default | Description |
|---|---|---|---|---|
| `namespace` | string | no | `xk6-otel-gen` | Default `service.namespace` for all services; overridable per service. |
| `services` | map | **yes** | — | Map of service identifier → service declaration. At least one required. |
| `journeys` | map | **yes** | — | Map of journey name → user-action sequence. At least one required. |
| `faults` | list | no | `[]` | Ordered array of fault injection specs. |

```yaml
namespace: shop            # optional; defaults to xk6-otel-gen
services: { ... }          # required
journeys: { ... }          # required
faults: [ ... ]            # optional
```

Selected validation rules:

- `services` and `journeys` must each contain at least one entry.
- The operation call graph must be a **DAG (acyclic)**; cycles are a validation error.
- Every call target, journey step, and fault target must reference a service /
  operation / edge that actually exists in the schema.

Export the JSON Schema for editor integration:

```bash
go run ./cmd/xk6-otel-gen-schema > topology.schema.json
go run ./cmd/xk6-otel-gen-schema -output topology.schema.json
```

---

## services

`services` maps a service identifier (the map key) to a service declaration.
Each service owns one or more operations, and each operation may make outgoing
calls (edges) to other services.

### Configurable fields (overview)

**Service (`services.<id>`)**

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `kind` | enum | **yes** | — | Service category. `application` / `database` / `external_api` / `cache` / `queue` |
| `operations` | list | **yes** | — | Operations owned by the service (at least one) |
| `namespace` | string | no | top-level `namespace` | `service.namespace` override for this service |
| `replicas` | int | no | `1` | Number of instances to synthesize (>= 1) |
| `language` | string | no | — | Implementation language metadata |
| `framework` | string | no | — | Framework metadata |
| `version` | string | no | — | Version metadata |

**Operation (`operations[]`)**

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | **yes** | — | Name unique within the service (1–120 bytes) |
| `calls` | list | no | `[]` | Ordered outgoing calls made by this operation (CallNode) |
| `log_events` | list | no | `[]` | Structured log events emitted when this operation completes (LogEvent) |
| `metrics` | list | no | `[]` | Custom metric data points recorded when this operation completes (Metric) |
| `profile` | object | no | — | Synthetic flamegraph pushed to Pyroscope (Profile) |

**Call (CallNode — items of `calls[]` / `parallel[]`)**

Each item is either a single edge or a parallel group (mutually exclusive).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `to` | object | **yes** for an edge | — | Call target `{ service, operation }` |
| `protocol` | enum | **yes** | — | Transport protocol. `http` / `grpc` / `messaging` |
| `latency` | object | no | see LatencyDist below | Latency distribution |
| `error_rate` | number | no | `0.0` | Failure probability `[0,1]` |
| `timeout` | duration | no | `0` (unlimited) | Per-attempt timeout |
| `retries` | int | no | `0` | Retry count (>= 0) |
| `retry_backoff` | enum | no | `exponential` | Retry delay strategy. `exponential` / `linear` / `constant` |
| `retry_base_delay` | duration | no | `100ms` | Base retry delay |
| `on_failure` | object | no | — | Fallback policy on failure (RecoveryPolicy) |
| `parallel` | list | **yes** for a group | — | Child CallNodes that run concurrently (at least one) |

**LatencyDist (`latency`)**

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `distribution` | enum | no | `constant` | `constant` / `lognormal` / `normal` / `exponential` |
| `p50` | duration | no | `0` | Median (50th percentile) |
| `p95` | duration | no | same as `p50` | 95th percentile (must be >= `p50`) |

**RecoveryPolicy (`on_failure`)**

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `fallback` | list | no | `[]` | Ordered fallback calls to try (CallNode) |
| `on_exhausted` | enum | no | `propagate` | Action after all fallbacks fail. `propagate` / `return_default` / `succeed_silently` |
| `default_response` | object | no | — | Synthetic response returned with `return_default` (arbitrary keys) |

### Field details

#### `kind` (required)

The semantic service category. Allowed values: `application`, `database`,
`external_api`, `cache`, `queue`. Reflected in generated span kinds and resource
attributes.

#### `operations` (required)

The array of callable units the service exposes (endpoints, RPC methods, message
handlers). At least one is required.

#### `namespace`

Overrides this service's `service.namespace`, taking precedence over the
top-level default.

#### `replicas`

Number of service instances to synthesize. Must be >= 1; defaults to `1`.

#### `language` / `framework` / `version`

Metadata (implementation language, framework, version) attached as resource
attributes and used to classify the generated telemetry.

#### `operations[].name` (required)

An operation name unique within the service. Must be a non-empty string of
1–120 bytes.

#### `operations[].calls`

The ordered list of outgoing calls this operation makes. Each item is a CallNode:
either an edge or a parallel group.

#### CallNode: edge

A directed call to another operation. `to` is required and is mutually exclusive
with `parallel`.

```yaml
calls:
  - to: { service: payment, operation: authorize_card }
    protocol: grpc
    latency: { distribution: lognormal, p50: 20ms, p95: 200ms }
    error_rate: 0.02
    timeout: 750ms
    retries: 2
    retry_backoff: exponential
    retry_base_delay: 100ms
```

- **`to`** — the target `{ service, operation }`. Both required; must point to an existing operation.
- **`protocol`** — one of `http` / `grpc` / `messaging`. Must be specified. For
  `messaging`, the sender emits a PRODUCER (publish) span and the receiver a
  CONSUMER (receive) span that is linked back to it with a span link, within the
  same journey trace.
- **`latency`** — latency distribution (see below).
- **`error_rate`** — failure probability of this call. `[0,1]`. Default `0.0`.
- **`timeout`** — upper bound for one attempt; if simulated latency exceeds it, the
  attempt is treated as a timeout failure. `0` (default) means unlimited.
- **`retries`** — retry count on failure. >= 0. Default `0`.
- **`retry_backoff`** — how the retry interval grows. `exponential` (default) / `linear` / `constant`.
- **`retry_base_delay`** — base retry delay. Default `100ms`.
- **`on_failure`** — fallback policy (RecoveryPolicy, below).

#### CallNode: parallel group

Runs child CallNodes concurrently. `parallel` is required and mutually exclusive
with `to`. Nestable.

```yaml
calls:
  - parallel:
      - to: { service: inventory, operation: check_stock }
        protocol: grpc
      - to: { service: pricing, operation: get_price }
        protocol: grpc
```

#### LatencyDist (`latency`)

Describes a call's latency distribution.

- **`distribution`** — `constant` (default) / `lognormal` / `normal` / `exponential`.
- **`p50`** — median. Default `0`.
- **`p95`** — 95th percentile. Defaults to `p50`. Must be >= `p50`.

A `duration` may be a Go-style string (e.g. `10ms`, `1s`) or a nanosecond integer.

#### RecoveryPolicy (`on_failure`)

Defines fallback behavior when an edge fails.

- **`fallback`** — ordered alternative calls to try (a list of CallNodes). Each
  fallback must belong to the same caller (`from`) as the original edge.
- **`on_exhausted`** — action after all fallbacks fail.
  - `propagate` (default) — propagate the error to the caller.
  - `return_default` — return `default_response`.
  - `succeed_silently` — suppress the error and treat as success.
- **`default_response`** — synthetic response returned with `return_default` (an object with arbitrary keys).

```yaml
calls:
  - to: { service: payment, operation: authorize_card }
    protocol: grpc
    on_failure:
      fallback:
        - to: { service: payment-backup, operation: authorize_card }
          protocol: grpc
      on_exhausted: return_default
      default_response: { status: "queued" }
```

#### `operations[].log_events`

Structured log events emitted when the operation completes, in addition to the
generic per-operation log. Each item becomes one OTLP log record whose
`event.name` is set (and also attached as an `event.name` attribute).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | **yes** | — | Event name; emitted as `event.name` |
| `severity` | enum | no | `info` | `trace` / `debug` / `info` / `warn` / `error` / `fatal` |
| `condition` | enum | no | `always` | When to emit. `always` / `on_success` / `on_error` |
| `body` | string | no | — | Log record body |
| `attributes` | map | no | — | Extra structured attributes (arbitrary keys) |

```yaml
operations:
  - name: authorize_card
    log_events:
      - name: provider_call.timeout
        severity: error
        condition: on_error
        body: "payment provider call timed out"
        attributes: { provider: stripe, retryable: true }
```

This powers LogQL such as
`{service_name="payment"} | event_name="provider_call.timeout"`.

#### `operations[].metrics`

Custom metric data points recorded when the operation completes. Optionally
**fault-linked**: while the referenced fault kind is active on this operation,
the recorded value becomes `baseline + delta` (or is overridden by `value`).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | **yes** | — | Instrument name |
| `type` | enum | **yes** | — | `counter` / `gauge` / `histogram` |
| `unit` | string | no | — | UCUM-style unit (e.g. `{request}`) |
| `baseline` | number | no | `0` | Value recorded (counter: amount added; gauge/histogram: value) |
| `condition` | enum | no | `always` | When to record. `always` / `on_success` / `on_error` |
| `attributes` | map | no | — | Extra data-point attributes |
| `when_fault` | object | no | — | Fault linkage (below) |

**when_fault**

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `kind` | enum | **yes** | — | Fault kind to react to. `latency_inflation` / `error_rate_override` / `disconnect` / `crash` |
| `delta` | number | no | `0` | Added to `baseline` while the fault is active on this operation |
| `value` | number | no | — | Overrides the value (instead of `delta`) while the fault is active |

```yaml
operations:
  - name: quote_shipping
    metrics:
      - name: shipping.quote.backlog
        type: gauge
        unit: "{request}"
        baseline: 5
        when_fault:
          kind: latency_inflation
          delta: 40
```

The fault linkage is evaluated against the same fault state applied to the
operation, so the value moves deterministically when the fault fires. A counter
fed on `condition: on_success` plus OTLP cumulative temporality yields a
continuously growing total (e.g. a settlement amount).

#### `operations[].profile`

A synthetic flamegraph for the operation, pushed to Pyroscope as pprof when
[`profilesEndpoint`]({{< relref "/reference/configuration" >}}) is set (otherwise
it is a no-op). Two stack-set variants enable **diff flamegraphs**: while the
linked fault kind is active, the `incident` stacks are emitted instead of
`baseline`. The operation span carries a `pyroscope.profile.id` attribute (equal
to its span id) for Span→Profiles correlation.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `enabled` | bool | no | `false` | Whether to emit a profile for this operation |
| `sample_rate` | int | no | `100` | Sample rate in Hz |
| `baseline` | list | **yes** when enabled | — | Stack samples for normal runs (StackSample) |
| `incident` | list | required with `when_fault` | — | Stack samples emitted while the linked fault is active |
| `when_fault` | object | no | — | `{ kind }` — fault kind that selects the `incident` variant |

**StackSample (`baseline[]` / `incident[]` items)**

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `frames` | list of string | **yes** | — | Call stack frames, ordered root → leaf |
| `weight` | number | no | `0` | Self weight of the stack (e.g. sample count) |

```yaml
operations:
  - name: quote_shipping
    profile:
      enabled: true
      sample_rate: 100
      baseline:
        - frames: ["handleQuoteShipping", "calcBaseRate"]
          weight: 120
      incident:
        - frames: ["handleQuoteShipping", "calcBaseRate", "geoLookup", "matrixSolve"]
          weight: 900
      when_fault:
        kind: latency_inflation
```

---

## journeys

`journeys` maps a journey name to a sequence of user actions. Each journey
execution produces one synthetic trace.

### Configurable fields (overview)

**Journey (`journeys.<name>`)**

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `steps` | list | **yes** | — | Ordered steps (at least one) |
| `weight` | number | no | `1` | Relative selection weight for `runRandomJourney()` (> 0) |

**Step (items of `steps[]` / `parallel[]`)**

Each item is either a single operation or a parallel group (mutually exclusive).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `service` | string | **yes** for a single step | — | Entry service |
| `operation` | string | **yes** for a single step | — | Entry operation |
| `parallel` | list | **yes** for a group | — | Child steps that run concurrently (at least one) |

### Field details

#### `steps` (required)

The ordered list of steps that make up the journey. At least one is required.
Each step is either a single operation invocation or a parallel group.

#### `weight`

The relative weight used when `runRandomJourney()` selects a journey. Must be
> 0; defaults to `1.0`.

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
        operation: view_cart
      - service: frontend
        operation: checkout
```

#### Step: single operation

`service` and `operation` specify the entry point. Both required; mutually
exclusive with `parallel`. Must point to an existing operation.

#### Step: parallel group

`parallel` runs multiple child steps concurrently. Mutually exclusive with
`service` / `operation`; nestable.

```yaml
steps:
  - parallel:
      - service: frontend
        operation: load_recommendations
      - service: frontend
        operation: load_banner
```

---

## faults

`faults` declares faults to inject during synthesis as an ordered array. Each
fault has a target, a kind, and severity parameters.

### Configurable fields (overview)

**Fault (`faults[]`)**

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `target` | string | **yes** | — | Target in `node:` / `operation:` / `edge:` form |
| `kind` | enum | **yes** | — | Fault kind. `latency_inflation` / `error_rate_override` / `disconnect` / `crash` |
| `severity` | object | no | — | Severity parameters (below) |

**SeverityParams (`severity`)**

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `probability` | number | no | `0` | Probability the fault fires `[0,1]` |
| `multiplier` | number | **required** for `latency_inflation` | `0` | Latency multiplier (> 0) |
| `add` | duration | no | `0` | Fixed delay to add (`latency_inflation`) |
| `value` | number | used by `error_rate_override` | `0` | Overriding error rate `[0,1]` |

### Field details

#### `target` (required)

The fault target, as a string in one of three forms.

| Target syntax | Scope |
|---|---|
| `node:<svc>` | all operations on one service |
| `operation:<svc>.<op>` | one service operation |
| `edge:<from_svc>.<from_op>-><to_svc>.<to_op>` | one call edge |

The referenced service / operation / edge must exist in the schema.

#### `kind` (required)

The type of fault to inject.

- **`latency_inflation`** — increases latency. With `add` (fixed) and `multiplier`,
  it adds `add + (multiplier - 1) × base latency`. `multiplier` must be > 0.
- **`error_rate_override`** — overrides the target's error rate with `value`
  (clamped to `[0,1]`).
- **`disconnect`** — injects a connection error (disconnect).
- **`crash`** — injects a crash.

#### `severity`

Severity parameters. Which fields apply depends on `kind`.

| kind | severity fields used |
|---|---|
| `latency_inflation` | `probability`, `multiplier` (required, > 0), `add` (optional) |
| `error_rate_override` | `probability`, `value` |
| `disconnect` | `probability` |
| `crash` | `probability` |

- **`probability`** — probability the fault fires per call. `[0,1]`.
- **`multiplier`** — latency multiplier (`latency_inflation`). > 0.
- **`add`** — fixed delay to add (`latency_inflation`).
- **`value`** — overriding error rate (`error_rate_override`); clamped to `[0,1]`.

```yaml
faults:
  - target: node:payment
    kind: latency_inflation
    severity: { probability: 0.20, multiplier: 3.0, add: 50ms }
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
