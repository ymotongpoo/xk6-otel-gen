---
title: はじめに
weight: 1
prev: /
next: /getting-started/quick-start
---

`xk6-otel-gen` は、宣言的な YAML トポロジから OpenTelemetry のトレース・メトリクス・
ログを合成する k6 拡張機能です。実際のサービスを構築することなく、マイクロサービスの
グラフ・ジャーニー・障害をモデル化できます。

```yaml
journeys:
  checkout:
    weight: 1.0
    steps:
      - service: frontend
        operation: get_index
```

本拡張機能は OTLP/gRPC および OTLP/HTTP のテレメトリを Collector へ送信できるほか、
`otel-gen` 出力を通じて k6 の出力メトリクスを転送することもできます。

## 機能

| 機能 | 具体例 |
|---|---|
| トポロジ DSL | `services.frontend.operations[].calls[]` でサービス間のエッジを表現 |
| ジャーニー実行 | `runJourney("checkout")` が 1 本の合成トレースを生成 |
| 障害注入 | `error_rate_override`、`latency_inflation`、`disconnect`、`crash` |
| オペレーション単位のシグナル | `log_events`・`metrics`・`profile` で構造化ログ・カスタムメトリクス・フレームグラフを出力 |
| スパンリンクとエグゼンプラー | `messaging` エッジで producer↔consumer スパンを連結。メトリクスに `trace_id` / `span_id` |
| Pyroscope プロファイル | fault 連動の incident 変種を持つ合成フレームグラフで diff プロファイリング |
| OTLP 送信 | `localhost:4317`(gRPC) または `localhost:4318`(HTTP)。プロファイルは `profilesEndpoint` 経由 |
| k6 出力連携 | `--out otel-gen=endpoint=localhost:4317` で k6 出力を転送 |
| JSON Schema 出力 | `go run ./cmd/xk6-otel-gen-schema > topology.schema.json` |
| トポロジ可視化 | `go run ./cmd/xk6-otel-gen-viz -input topology.yaml -output topology.html` |

障害の例:

```yaml
faults:
  - target: operation:payment.authorize_card
    kind: error_rate_override
    severity:
      probability: 1.0
      value: 0.10
```

## 次のステップ

- [クイックスタート]({{< relref "/getting-started/quick-start" >}}) — k6 をビルドして合成トラフィックを実行します。
- [ビルド]({{< relref "/getting-started/building" >}}) — この拡張機能を組み込んだ k6 バイナリをビルドします。
- [使い方]({{< relref "/usage" >}}) — JavaScript API。
