# Astroshop Example

## Description

This example is modeled after the OpenTelemetry Demo astronomy shop v2.2.0. It keeps the same kind of commerce graph while staying intentionally synthetic and compact.

```text
frontend -> product-catalog, cart, checkout
checkout -> payment, fraud-detection, shipping, email, accounting, kafka
support -> ad, recommendation, image-provider
dependencies -> redis-cache, postgres, kafka, flagd
```

The topology includes 18 services, 5 weighted journeys, retry backoff on payment authorization, and four subtle faults: payment errors, shipping latency, recommendation crashes, and email disconnects.

Operations may declare `log_events` in the topology YAML to emit structured OTLP logs with `event.name` when the operation completes (for example, `{service_name="payment"} | event_name="provider_call.timeout"` in LogQL).

## Prerequisites

| Tool | Minimum | Purpose |
|---|---:|---|
| Go | 1.25 | Build the k6 extension |
| xk6 | latest | Build a custom k6 binary |
| Docker | latest stable | Run kind nodes |
| kind | 0.20 | Local Kubernetes cluster |
| kubectl | 1.27 | Apply the demo manifests |

```bash
go install go.k6.io/xk6/cmd/xk6@latest
kind version
kubectl version --client
```

## Setup

Create a local cluster and deploy the larger LGTM-lite stack:

```bash
kind create cluster --name xk6-otel-gen
kubectl apply -k examples/astroshop/k8s/
kubectl -n xk6-otel-gen-demo rollout status deployment/otel-collector
kubectl -n xk6-otel-gen-demo rollout status deployment/grafana
```

Build k6 with this extension:

```bash
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.
```

## Run

Port-forward the Collector and run the two-scenario script. The browse scenario uses `runRandomJourney()` to demonstrate weighted journey selection.

```bash
kubectl -n xk6-otel-gen-demo port-forward svc/otel-collector 4317:4317
./k6 run examples/astroshop/script.js --out otel-gen=endpoint=localhost:4317,protocol=grpc,insecure=true
```

## View Results

Open Grafana:

```bash
kubectl -n xk6-otel-gen-demo port-forward svc/grafana 3000:3000
```

Then browse to `http://localhost:3000`. The dashboard focuses on the commerce path:

| Signal | Backend | Example view |
|---|---|---|
| Traces | Tempo | Recent astroshop traces |
| Metrics | Prometheus | Request rate by service |
| Metrics | Prometheus | Journey mix |
| Logs | Loki | Commerce path logs |

## Cleanup

```bash
kubectl delete namespace xk6-otel-gen-demo
kind delete cluster --name xk6-otel-gen
```

## Customize

Edit `examples/astroshop/topology.yaml` and rerun k6. To bias traffic toward order placement, adjust journey weights:

```yaml
journeys:
  place-order:
    weight: 0.25
    steps:
      - service: frontend
        operation: place_order
```

For upstream maintenance, compare this example against `open-telemetry/opentelemetry-demo` release `2.2.0` before changing service names or journey shapes.

The fault declarations include YAML `schedule` blocks that script a
burn→recover timeline from engine start: healthy at `0s`, incident at `1m`, and
recovered at `3m`. If a scenario needs to override one fault manually, call
`topology.setFaultIntensity("operation:payment.authorize_card", x)` before the
journey; target-specific overrides take precedence over the declarative
schedule.

`checkout.place_order` declares a counter metric (`orders.settlement.amount.total`)
that adds 80 on each successful order; with OTLP cumulative temporality this
becomes a settlement-total time series. `shipping.quote_shipping` declares a
fault-linked gauge (`shipping.quote.backlog`) that jumps from 5 to 45 while the
existing `latency_inflation` fault is active on that operation.
`kafka` declares a service-scoped observable gauge (`kafka.consumer.lag`) backed
by `state_updates` on `kafka.publish_order`, so queue lag is recalculated during
OTel collection even though the metric itself is not emitted by an operation.

Messaging edges (for example `checkout.place_order` → `kafka.publish_order`)
emit a PRODUCER (publish) span on the sender and a CONSUMER (receive) span on
the receiver within the same journey trace; the consumer span carries a span
link back to the producer span so Grafana can follow publish↔receive hops.
Histogram metrics include exemplars (trace_id / span_id) when spans are sampled,
enabling Grafana metrics→traces drill-down.

Synthetic flamegraphs declared in the topology are pushed to Pyroscope when
`profilesEndpoint` is configured (Pyroscope or Grafana Cloud Profiles). The
linked fault kind selects an incident stack variant for diff flamegraphs, and
spans carry `pyroscope.profile.id` (= span_id) so Grafana can link Span→Profiles.
