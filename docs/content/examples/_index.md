---
title: Examples
weight: 4
---

| Example | Size | Use case |
|---|---:|---|
| [minimal](https://github.com/ymotongpoo/xk6-otel-gen/tree/main/examples/minimal) | 3 services | First run, topology basics, local smoke test |
| [astroshop](https://github.com/ymotongpoo/xk6-otel-gen/tree/main/examples/astroshop) | 18 services | Larger commerce graph modeled after OTel Demo v2.2.0 |

Run only the topology validation tests:

```bash
go test ./test/examples/...
```

Build the Kubernetes manifests:

```bash
kustomize build examples/minimal/k8s/ > /tmp/minimal.yaml
kustomize build examples/astroshop/k8s/ > /tmp/astroshop.yaml
```
