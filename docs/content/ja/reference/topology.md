---
title: トポロジ YAML リファレンス
weight: 1
---

トポロジファイルには 3 つのトップレベルセクションがあります。

| セクション | 必須 | 例 |
|---|---|---|
| `services` | はい | `frontend`、`backend`、`database` |
| `journeys` | はい | `checkout`、`browse`、`place-order` |
| `faults` | いいえ | `operation:shipping.quote_shipping` への `latency_inflation` |

トップレベルの `namespace`(任意)は、すべてのサービスの既定の OpenTelemetry
`service.namespace` リソース属性を設定します。各サービスは `services.<name>.namespace`
で上書きできます。省略時は `xk6-otel-gen` が使われます。

最小限のサービス宣言:

```yaml
services:
  backend:
    namespace: checkout
    kind: application
    replicas: 3
    language: java
    framework: spring-boot
    version: 2.5.0
    operations:
      - name: get_user
```

ジャーニーは `runRandomJourney()` 使用時に `weight` で選択されます。`weight` を省略すると
`1.0` が既定値になります。検証済みのトポロジファイルでは weight は正の値でなければ
なりません。

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
        operation: checkout
```

エッジは `retries`、`retry_backoff`、`retry_base_delay` で再試行のタイミングを設定
できます。`timeout` を使うと 1 回のエッジ試行に上限を設け、シミュレートされたレイテンシ
がその予算を超えた場合にタイムアウト失敗としてマークします。

```yaml
calls:
  - to: { service: payment, operation: authorize_card }
    protocol: grpc
    timeout: 750ms
    retries: 2
    retry_backoff: exponential
    retry_base_delay: 100ms
```

## 障害(faults)

障害はサービスノード、1 つのオペレーション、または 1 つの具体的なエッジを対象とします。

| ターゲット構文 | 範囲 |
|---|---|
| `node:<svc>` | 1 つのサービス上のすべてのオペレーション |
| `operation:<svc>.<op>` | 1 つのサービスオペレーション |
| `edge:<from_svc>.<from_op>-><to_svc>.<to_op>` | 1 つの呼び出しエッジ |

サポートされる障害の種類:

| 種類 | severity フィールド |
|---|---|
| `latency_inflation` | `probability`、`multiplier`、`add`(任意) |
| `error_rate_override` | `probability`、`value` |
| `disconnect` | `probability` |
| `crash` | `probability` |

```yaml
faults:
  - target: node:payment
    kind: latency_inflation
    severity: { probability: 0.20, multiplier: 3.0 }
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

エディタ連携用に JSON Schema を出力します。

```bash
go run ./cmd/xk6-otel-gen-schema > topology.schema.json
go run ./cmd/xk6-otel-gen-schema -output topology.schema.json
```
