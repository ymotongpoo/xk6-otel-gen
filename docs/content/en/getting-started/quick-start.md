---
title: Quick Start
weight: 1
---

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
