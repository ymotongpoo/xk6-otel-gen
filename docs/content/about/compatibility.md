---
title: Compatibility
weight: 3
---

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

## License

`xk6-otel-gen` is licensed under Apache-2.0.

```text
SPDX-License-Identifier: Apache-2.0
```

| File | Purpose |
|---|---|
| [LICENSE](https://github.com/ymotongpoo/xk6-otel-gen/blob/main/LICENSE) | Apache License 2.0 full text |
| `.go` files | SPDX header enforced by lint |
