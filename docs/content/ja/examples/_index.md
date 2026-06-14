---
title: サンプル
weight: 4
---

| サンプル | 規模 | ユースケース |
|---|---:|---|
| [minimal](https://github.com/ymotongpoo/xk6-otel-gen/tree/main/examples/minimal) | 3 サービス | 初回実行、トポロジの基本、ローカルのスモークテスト |
| [astroshop](https://github.com/ymotongpoo/xk6-otel-gen/tree/main/examples/astroshop) | 18 サービス | OTel Demo v2.2.0 を模した大規模なコマースグラフ |

トポロジ検証のテストのみを実行します。

```bash
go test ./test/examples/...
```

Kubernetes マニフェストをビルドします。

```bash
kustomize build examples/minimal/k8s/ > /tmp/minimal.yaml
kustomize build examples/astroshop/k8s/ > /tmp/astroshop.yaml
```
