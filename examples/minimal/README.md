# Minimal Example

## Description

This example generates synthetic OpenTelemetry traces, metrics, and logs for a three-tier topology:

```text
frontend -> backend -> database
```

The `checkout` journey enters `frontend.get_index`, calls `backend.get_user`, and then calls `database.select_user`. A small `error_rate_override` fault is attached to the frontend-to-backend edge.

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

Create a local cluster and deploy the LGTM-lite stack:

```bash
kind create cluster --name xk6-otel-gen
kubectl apply -k examples/minimal/k8s/
kubectl -n xk6-otel-gen-demo rollout status deployment/otel-collector
kubectl -n xk6-otel-gen-demo rollout status deployment/grafana
```

Build k6 with this extension:

```bash
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.
```

## Run

Port-forward the Collector OTLP/gRPC endpoint and run the script:

```bash
kubectl -n xk6-otel-gen-demo port-forward svc/otel-collector 4317:4317
./k6 run examples/minimal/script.js --out otel-gen=endpoint=localhost:4317,protocol=grpc,insecure=true
```

## View Results

Open Grafana:

```bash
kubectl -n xk6-otel-gen-demo port-forward svc/grafana 3000:3000
```

Then browse to `http://localhost:3000`. The default home dashboard shows:

| Signal | Backend | Example view |
|---|---|---|
| Traces | Tempo | Recent synthetic traces |
| Metrics | Prometheus | Request rate by service |
| Logs | Loki | Synthetic logs |

## Cleanup

Remove the demo namespace or delete the whole kind cluster:

```bash
kubectl delete namespace xk6-otel-gen-demo
kind delete cluster --name xk6-otel-gen
```

## Customize

Edit `examples/minimal/topology.yaml`, then rerun the same k6 command. A common first change is to add latency to the backend-to-database edge:

```yaml
faults:
  - target: edge:backend.get_user->database.select_user
    kind: latency_inflation
    severity:
      probability: 1.0
      multiplier: 2.0
      add: 25ms
```
