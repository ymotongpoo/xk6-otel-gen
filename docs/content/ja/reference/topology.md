---
title: トポロジ YAML リファレンス
weight: 1
---

トポロジファイルは、`xk6-otel-gen` が OpenTelemetry シグナルを合成するための宣言的な
マイクロサービス構成を記述します。この文書では YAML で設定できる項目をすべて記載します。

## トップレベル

| キー | 型 | 必須 | 既定値 | 説明 |
|---|---|---|---|---|
| `namespace` | string | いいえ | `xk6-otel-gen` | 全サービス共通の `service.namespace` 既定値。各サービスで上書き可能。 |
| `services` | map | **はい** | — | サービス識別子 → サービス定義のマップ。1 つ以上必須。 |
| `journeys` | map | **はい** | — | ジャーニー名 → ユーザー操作シーケンスのマップ。1 つ以上必須。 |
| `faults` | list | いいえ | `[]` | 障害注入指定の配列（順序付き）。 |

```yaml
namespace: shop            # 任意。省略時は xk6-otel-gen
services: { ... }          # 必須
journeys: { ... }          # 必須
faults: [ ... ]            # 任意
```

検証ルールの一部:

- `services` と `journeys` はそれぞれ最低 1 要素が必要です。
- オペレーション呼び出しのグラフは **DAG（非循環）** でなければなりません。循環は検証エラーになります。
- すべての呼び出し先・ジャーニーのステップ・障害ターゲットは、スキーマ内に実在するサービス／オペレーション／エッジを参照する必要があります。

エディタ補完用に JSON Schema を出力できます。

```bash
go run ./cmd/xk6-otel-gen-schema > topology.schema.json
go run ./cmd/xk6-otel-gen-schema -output topology.schema.json
```

---

## services

`services` は、サービス識別子（マップのキー）をサービス定義に対応づけます。各サービスは
1 つ以上のオペレーションを持ち、各オペレーションは他サービスへの呼び出し（エッジ）を
持てます。

### 設定可能な項目（一覧）

**サービス（`services.<id>`）**

| フィールド | 型 | 必須 | 既定値 | 説明 |
|---|---|---|---|---|
| `kind` | enum | **はい** | — | サービス種別。`application` / `database` / `external_api` / `cache` / `queue` |
| `operations` | list | **はい** | — | サービスが持つオペレーション（1 つ以上） |
| `namespace` | string | いいえ | トップレベル `namespace` | このサービスの `service.namespace` 上書き |
| `replicas` | int | いいえ | `1` | 合成するインスタンス数（1 以上） |
| `language` | string | いいえ | — | 実装言語のメタデータ |
| `framework` | string | いいえ | — | フレームワークのメタデータ |
| `version` | string | いいえ | — | バージョンのメタデータ |

**オペレーション（`operations[]`）**

| フィールド | 型 | 必須 | 既定値 | 説明 |
|---|---|---|---|---|
| `name` | string | **はい** | — | サービス内で一意な名前（1〜120 バイト） |
| `calls` | list | いいえ | `[]` | このオペレーションが行う送信呼び出し（順序付き、CallNode） |

**呼び出し（CallNode = `calls[]` / `parallel[]` の各要素）**

各要素は「エッジ 1 本」または「並列グループ」のいずれか一方です（排他）。

| フィールド | 型 | 必須 | 既定値 | 説明 |
|---|---|---|---|---|
| `to` | object | エッジでは**はい** | — | 呼び出し先 `{ service, operation }` |
| `protocol` | enum | **はい** | — | 転送プロトコル。`http` / `grpc` / `messaging` |
| `latency` | object | いいえ | 下記 LatencyDist 参照 | レイテンシ分布 |
| `error_rate` | number | いいえ | `0.0` | 失敗確率 `[0,1]` |
| `timeout` | duration | いいえ | `0`（無制限） | 1 回の試行のタイムアウト |
| `retries` | int | いいえ | `0` | リトライ回数（0 以上） |
| `retry_backoff` | enum | いいえ | `exponential` | リトライ遅延戦略。`exponential` / `linear` / `constant` |
| `retry_base_delay` | duration | いいえ | `100ms` | リトライの基準遅延 |
| `on_failure` | object | いいえ | — | 失敗時のフォールバック方針（RecoveryPolicy） |
| `parallel` | list | 並列では**はい** | — | 並行実行する子 CallNode（1 つ以上） |

**LatencyDist（`latency`）**

| フィールド | 型 | 必須 | 既定値 | 説明 |
|---|---|---|---|---|
| `distribution` | enum | いいえ | `constant` | 分布。`constant` / `lognormal` / `normal` / `exponential` |
| `p50` | duration | いいえ | `0` | 中央値（50 パーセンタイル） |
| `p95` | duration | いいえ | `p50` と同値 | 95 パーセンタイル（`p50` 以上である必要あり） |

**RecoveryPolicy（`on_failure`）**

| フィールド | 型 | 必須 | 既定値 | 説明 |
|---|---|---|---|---|
| `fallback` | list | いいえ | `[]` | 順に試す代替の呼び出し（CallNode） |
| `on_exhausted` | enum | いいえ | `propagate` | 全フォールバック失敗後の動作。`propagate` / `return_default` / `succeed_silently` |
| `default_response` | object | いいえ | — | `return_default` 時に返す合成レスポンス（任意のキー） |

### 各設定の詳細

#### `kind`（必須）

サービスの意味的な種別です。許可値は `application`、`database`、`external_api`、`cache`、
`queue`。生成されるスパンの種別やリソース属性に反映されます。

#### `operations`（必須）

サービスが公開する呼び出し可能な単位（エンドポイント、RPC メソッド、メッセージハンドラ）
の配列です。最低 1 つ必要です。

#### `namespace`

このサービスの `service.namespace` をトップレベルの既定値から上書きします。

#### `replicas`

合成するサービスインスタンス数です。1 以上である必要があり、既定は `1` です。

#### `language` / `framework` / `version`

リソース属性に付与されるメタデータ（実装言語、フレームワーク、バージョン）です。生成
されるテレメトリの分類に使われます。

#### `operations[].name`（必須）

サービス内で一意なオペレーション名です。1〜120 バイトの非空文字列である必要があります。

#### `operations[].calls`

このオペレーションが実行する送信呼び出しの順序付きリストです。各要素は CallNode で、
「エッジ」か「並列グループ」のいずれかです。

#### CallNode: エッジ

別オペレーションへの有向呼び出しです。`to` が必須で、`parallel` とは排他です。

```yaml
calls:
  - to: { service: payment, operation: authorize_card }
    protocol: grpc
    latency: { distribution: lognormal, p50: 20ms, p95: 200ms }
    error_rate: 0.02
    timeout: 750ms
    retries: 2
    retry_backoff: exponential
    retry_base_delay: 100ms
```

- **`to`** — 呼び出し先の `{ service, operation }`。両方必須で、実在するオペレーションを指す必要があります。
- **`protocol`** — `http` / `grpc` / `messaging` のいずれか。指定が必要です。
- **`latency`** — レイテンシ分布（下記）。
- **`error_rate`** — この呼び出しの失敗確率。`[0,1]`。既定 `0.0`。
- **`timeout`** — 1 回の試行に対する上限。シミュレートされたレイテンシがこれを超えると
  タイムアウト失敗として扱われます。`0`（既定）は無制限。
- **`retries`** — 失敗時のリトライ回数。0 以上。既定 `0`。
- **`retry_backoff`** — リトライ間隔の増え方。`exponential`（既定）/ `linear` / `constant`。
- **`retry_base_delay`** — リトライの基準遅延。既定 `100ms`。
- **`on_failure`** — フォールバック方針（下記 RecoveryPolicy）。

#### CallNode: 並列グループ

子 CallNode を並行して実行します。`parallel` が必須で、`to` とは排他です。ネスト可能です。

```yaml
calls:
  - parallel:
      - to: { service: inventory, operation: check_stock }
        protocol: grpc
      - to: { service: pricing, operation: get_price }
        protocol: grpc
```

#### LatencyDist（`latency`）

呼び出しのレイテンシ分布を表します。

- **`distribution`** — `constant`（既定）/ `lognormal` / `normal` / `exponential`。
- **`p50`** — 中央値。既定 `0`。
- **`p95`** — 95 パーセンタイル。既定は `p50` と同値。`p50` 以上である必要があります。

`duration` は Go 形式の文字列（例: `10ms`、`1s`）またはナノ秒の整数で指定できます。

#### RecoveryPolicy（`on_failure`）

エッジが失敗したときのフォールバック動作を定義します。

- **`fallback`** — 順に試す代替の呼び出し（CallNode のリスト）。各フォールバックは元の
  エッジと同じ呼び出し元（`from`）に属する必要があります。
- **`on_exhausted`** — すべてのフォールバックが失敗した後の動作。
  - `propagate`（既定）— エラーを呼び出し元へ伝播する。
  - `return_default` — `default_response` を返す。
  - `succeed_silently` — エラーを抑制して成功扱いにする。
- **`default_response`** — `return_default` 時に返す合成レスポンス（任意のキーを持つオブジェクト）。

```yaml
calls:
  - to: { service: payment, operation: authorize_card }
    protocol: grpc
    on_failure:
      fallback:
        - to: { service: payment-backup, operation: authorize_card }
          protocol: grpc
      on_exhausted: return_default
      default_response: { status: "queued" }
```

---

## journeys

`journeys` は、ジャーニー名をユーザー操作のシーケンスに対応づけます。各ジャーニーの
実行が 1 本の合成トレースを生成します。

### 設定可能な項目（一覧）

**ジャーニー（`journeys.<name>`）**

| フィールド | 型 | 必須 | 既定値 | 説明 |
|---|---|---|---|---|
| `steps` | list | **はい** | — | 順序付きのステップ（1 つ以上） |
| `weight` | number | いいえ | `1` | `runRandomJourney()` での相対選択重み（0 より大） |

**ステップ（Step = `steps[]` / `parallel[]` の各要素）**

各要素は「単一オペレーション」または「並列グループ」のいずれか一方です（排他）。

| フィールド | 型 | 必須 | 既定値 | 説明 |
|---|---|---|---|---|
| `service` | string | 単一では**はい** | — | 開始サービス |
| `operation` | string | 単一では**はい** | — | 開始オペレーション |
| `parallel` | list | 並列では**はい** | — | 並行実行する子ステップ（1 つ以上） |

### 各設定の詳細

#### `steps`（必須）

ジャーニーを構成するステップの順序付きリストです。最低 1 つ必要です。各ステップは
単一オペレーションの呼び出しか、並列グループです。

#### `weight`

`runRandomJourney()` がジャーニーを選択する際の相対的な重みです。0 より大きい必要があり、
省略時は `1.0` です。

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
        operation: view_cart
      - service: frontend
        operation: checkout
```

#### Step: 単一オペレーション

`service` と `operation` で開始点を指定します。両方必須で、`parallel` とは排他です。
実在するオペレーションを指す必要があります。

#### Step: 並列グループ

`parallel` で複数の子ステップを並行実行します。`service` / `operation` とは排他で、
ネスト可能です。

```yaml
steps:
  - parallel:
      - service: frontend
        operation: load_recommendations
      - service: frontend
        operation: load_banner
```

---

## faults

`faults` は、合成時に注入する障害を順序付きの配列で宣言します。各障害は対象（ターゲット）、
種類、深刻度（severity）を持ちます。

### 設定可能な項目（一覧）

**障害（`faults[]`）**

| フィールド | 型 | 必須 | 既定値 | 説明 |
|---|---|---|---|---|
| `target` | string | **はい** | — | 対象。`node:` / `operation:` / `edge:` のいずれかの形式 |
| `kind` | enum | **はい** | — | 障害種別。`latency_inflation` / `error_rate_override` / `disconnect` / `crash` |
| `severity` | object | いいえ | — | 深刻度パラメータ（下記） |

**SeverityParams（`severity`）**

| フィールド | 型 | 必須 | 既定値 | 説明 |
|---|---|---|---|---|
| `probability` | number | いいえ | `0` | 障害が発動する確率 `[0,1]` |
| `multiplier` | number | `latency_inflation` で**必須** | `0` | レイテンシ倍率（0 より大） |
| `add` | duration | いいえ | `0` | 加算する固定遅延（`latency_inflation`） |
| `value` | number | `error_rate_override` で使用 | `0` | 上書きするエラー率 `[0,1]` |

### 各設定の詳細

#### `target`（必須）

障害の対象を文字列で指定します。3 つの形式があります。

| ターゲット構文 | 範囲 |
|---|---|
| `node:<svc>` | 1 つのサービス上のすべてのオペレーション |
| `operation:<svc>.<op>` | 1 つのサービスオペレーション |
| `edge:<from_svc>.<from_op>-><to_svc>.<to_op>` | 1 つの呼び出しエッジ |

指定したサービス／オペレーション／エッジはスキーマ内に実在する必要があります。

#### `kind`（必須）

注入する障害の種類です。

- **`latency_inflation`** — レイテンシを増やします。`add`（固定加算）と `multiplier`
  （倍率）で、`add + (multiplier - 1) × 基準レイテンシ` だけ増加させます。`multiplier` は
  0 より大きい必要があります。
- **`error_rate_override`** — 対象のエラー率を `value`（`[0,1]` にクランプ）で上書きします。
- **`disconnect`** — 接続断（コネクションエラー）を発生させます。
- **`crash`** — クラッシュを発生させます。

#### `severity`

障害の深刻度パラメータです。どのフィールドが使われるかは `kind` によります。

| kind | 使用する severity フィールド |
|---|---|
| `latency_inflation` | `probability`、`multiplier`（必須・>0）、`add`（任意） |
| `error_rate_override` | `probability`、`value` |
| `disconnect` | `probability` |
| `crash` | `probability` |

- **`probability`** — その呼び出しごとに障害が発動する確率。`[0,1]`。
- **`multiplier`** — レイテンシ倍率（`latency_inflation`）。0 より大。
- **`add`** — 加算する固定遅延（`latency_inflation`）。
- **`value`** — 上書きするエラー率（`error_rate_override`）。`[0,1]` にクランプされます。

```yaml
faults:
  - target: node:payment
    kind: latency_inflation
    severity: { probability: 0.20, multiplier: 3.0, add: 50ms }
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
