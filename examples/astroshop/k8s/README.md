# Astroshop Kubernetes Manifests

## Description

This directory deploys the astroshop example observability stack:

```text
OpenTelemetry Collector -> Tempo, Prometheus, Loki -> Grafana
```

The resource requests are slightly larger than the minimal example because the topology has more services and higher default VU counts.

## Prerequisites

```bash
kind version
kubectl version --client
docker version
```

## Setup

Create a cluster if one is not already available:

```bash
kind create cluster --name xk6-otel-gen
kubectl cluster-info --context kind-xk6-otel-gen
```

Apply the manifests:

```bash
kubectl apply -k examples/astroshop/k8s/
kubectl -n xk6-otel-gen-demo get pods
```

## Run

Wait for the deployments:

```bash
kubectl -n xk6-otel-gen-demo rollout status deployment/tempo
kubectl -n xk6-otel-gen-demo rollout status deployment/loki
kubectl -n xk6-otel-gen-demo rollout status deployment/prometheus
kubectl -n xk6-otel-gen-demo rollout status deployment/grafana
kubectl -n xk6-otel-gen-demo rollout status deployment/otel-collector
```

## View Results

Forward Grafana and the Collector:

```bash
kubectl -n xk6-otel-gen-demo port-forward svc/grafana 3000:3000
kubectl -n xk6-otel-gen-demo port-forward svc/otel-collector 4317:4317
```

Open `http://localhost:3000` for Grafana. Point k6 at `localhost:4317`.

## Cleanup

```bash
kubectl delete namespace xk6-otel-gen-demo
kind delete cluster --name xk6-otel-gen
```

## Customize

To run minimal and astroshop at the same time, copy this directory and change the namespace in `kustomization.yaml` and `namespace.yaml`:

```yaml
namespace: xk6-otel-gen-astroshop
```
