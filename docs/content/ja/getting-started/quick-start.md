---
title: クイックスタート
weight: 1
---

k6 をビルドし、最小構成のオブザーバビリティスタックをデプロイして、合成トラフィックを
実行します。

```bash
# 1. この拡張機能を組み込んだローカル k6 バイナリをビルドする。
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.

# 2. ローカルの Kubernetes クラスターを作成する。
kind create cluster --name xk6-otel-gen

# 3. Collector、Tempo、Prometheus、Loki、Grafana をデプロイする。
kubectl apply -k examples/minimal/k8s/

# 4. OTLP/gRPC を Collector へポートフォワードする。
kubectl -n xk6-otel-gen-demo port-forward svc/otel-collector 4317:4317

# 5. 最小構成のジャーニーを実行し、テレメトリをエクスポートする。
./k6 run examples/minimal/script.js --out otel-gen=endpoint=localhost:4317,protocol=grpc,insecure=true
```

別のターミナルで Grafana を開きます。

```bash
kubectl -n xk6-otel-gen-demo port-forward svc/grafana 3000:3000
```
