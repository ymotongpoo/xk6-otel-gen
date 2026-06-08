# Topology YAML Schema — xk6-otel-gen

本書は xk6-otel-gen が入力として受け取る **トポロジー YAML の構造** を、完全な例を交えて定義します。設計判断: **operation を第一級概念にする** (`components.md` Q4 の解釈確定)。

---

## まず把握すべき 2 つの抽象レベル

| レベル | 何を表現するか | 観測上の単位 |
|---|---|---|
| **Operation tree** (intra-trace) | 「1 つの operation が呼ばれた時、その中でバックエンドの何 (複数 service / 複数 operation) を呼ぶか」 | **1 つの trace** (深い span ツリー) |
| **Journey** (inter-trace) | 「ユーザーがセッション中に行う一連のアクション」 | **複数の trace** (各 step が 1 trace ルート) |

つまり:
- Operation tree は YAML の **`services:` 配下の `operations[].calls`** に書く → 1 trace の中身を決める
- Journey は YAML の **`journeys:` 配下の `steps`** に書く → ユーザーがどんな順序で複数 trace を生むかを決める

ジャーニー側にバックエンドの内部呼び出しを列挙する必要はない — それは topology 側で `calls:` が自動展開する。

### 例: フロントエンド 1 アクションが複数サービス / 複数 operation を呼ぶ

ユーザーが商品ページを開いたとき、フロントエンドが裏で `catalog-service.GetProduct` と `catalog-service.ListRelated` (同一サービスの別 operation) と `search-service.LogQuery` (別サービス) を呼ぶケース。

```yaml
services:
  frontend:
    kind: application
    operations:
      - name: GET /products/{id}
        calls:                                # ← 1 trace 内で順次/並列に呼ぶリスト
          - to: { service: catalog-service, operation: GetProduct }
            protocol: http
            latency: { p50: 10ms, p95: 100ms }
          - to: { service: catalog-service, operation: ListRelated }   # 同じサービスの別 op
            protocol: http
            latency: { p50: 8ms }
          - to: { service: search-service, operation: LogQuery }       # 別サービス
            protocol: http
            latency: { p50: 3ms }
            on_failure:
              fallback: []
              on_exhausted: succeed_silently  # ログ失敗は無視

  catalog-service:
    kind: application
    operations:
      - name: GetProduct
        calls:
          - to: { service: product-db, operation: GetProduct }
            protocol: grpc
            latency: { p50: 30ms, p95: 200ms }
      - name: ListRelated
        calls:
          - to: { service: product-db, operation: ListByCategory }
            protocol: grpc

  search-service:
    kind: application
    operations:
      - name: LogQuery
        calls:
          - to: { service: search-log, operation: Append }
            protocol: grpc

  product-db:
    kind: database
    operations:
      - { name: GetProduct, calls: [] }
      - { name: ListByCategory, calls: [] }
  search-log:
    kind: database
    operations:
      - { name: Append, calls: [] }

journeys:
  view-product:           # ← user-perspective: ユーザーは「商品ページを開く」だけ
    weight: 1.0
    steps:
      - service: frontend
        operation: GET /products/{id}
```

この YAML で `view-product` ジャーニーを 1 回実行すると、**1 つの trace** が生成され、次の span ツリーが組み上がる:

```text
trace_id=<unique-per-iteration>
└─ Span: frontend.GET /products/{id}
   ├─ Span: catalog-service.GetProduct
   │  └─ Span: product-db.GetProduct
   ├─ Span: catalog-service.ListRelated
   │  └─ Span: product-db.ListByCategory
   └─ Span: search-service.LogQuery
      └─ Span: search-log.Append
```

これらすべて topology 側 (`services:` の各 operation の `calls:`) から自動展開されたもの。**Journey 定義は 1 step だけ**。

### 例: 複数 step のジャーニー (= 複数 trace)

「ユーザーが商品を見て → カートに入れて → 決済する」のような複数アクションの flow を表すには、journey に複数 step を書く:

```yaml
journeys:
  full-checkout:
    weight: 0.3
    steps:
      - service: frontend
        operation: GET /products/{id}    # → 1 つ目の trace (商品ページ)
      - service: frontend
        operation: POST /cart            # → 2 つ目の trace (カート追加)
      - service: frontend
        operation: POST /checkout        # → 3 つ目の trace (決済)
```

各 step は **独立した trace** (それぞれ unique な trace_id) を生成。各 trace は当該 frontend operation の operation tree に従って span を展開する。

---

## トップレベル構造

```yaml
services:    # サービス定義 (マップ、キーは ServiceID)
  ...
journeys:    # ユーザージャーニー定義 (マップ、キーはジャーニー名)
  ...
faults:      # 障害注入定義 (配列)
  ...
```

すべて 1 つの YAML ファイルに記述する (Q2=A シンプル単一ファイル方針)。

---

## `services:` セクション

各サービスは **1 つ以上の operation** を持つ。`operation` は外部から呼ばれる単位 (HTTP エンドポイント、RPC メソッド、メッセージハンドラ等)。サービス自身は operation の集合体として定義され、**個別の outgoing edge はすべて operation 配下** に置く。

```yaml
services:
  frontend:                                # ← ServiceID
    kind: application                      # application | database | external_api | cache | queue
    replicas: 3                            # optional, default 1 (service.instance.id を replicas 分だけ生成)
    language: javascript                   # optional
    framework: react                       # optional
    version: 1.4.0                         # optional
    operations:
      - name: GET /products/{id}           # operation 名 (HTTP path / RPC method / message topic 等)
        calls:                             # 内部で順序通り実行する呼び出しのリスト
          - to:                            # 呼び出し先 = 別 operation
              service: catalog-service
              operation: GetProduct
            protocol: http
            latency:
              distribution: lognormal
              p50: 10ms
              p95: 100ms
            error_rate: 0.001
            timeout: 1s
            retries: 2
            retry_backoff: exponential

  catalog-service:
    kind: application
    replicas: 4
    language: go
    operations:
      - name: GetProduct
        calls:
          - to: { service: product-db, operation: GetProduct }
            protocol: grpc
            latency: { distribution: lognormal, p50: 30ms, p95: 200ms }
            error_rate: 0.001
            timeout: 1s
            on_failure:                    # ← リカバリーポリシー (条件付きカスケード)
              fallback:
                - to: { service: product-cache, operation: GetProduct }
                  protocol: grpc
                  latency: { p50: 1ms, p95: 5ms }
                  error_rate: 0.0001
              on_exhausted: propagate      # propagate | return_default | succeed_silently

      - name: GetProductWithRecommendations
        calls:
          - parallel:                      # fan-out グループ (Q7 elaboration)
              - to: { service: product-db, operation: GetProduct }
                protocol: grpc
                latency: { p50: 30ms }
              - to: { service: recommender, operation: Recommend }
                protocol: grpc
                latency: { p50: 80ms }
          - to: { service: audit-log, operation: Append }     # 並列ブロック完了後の sequential 呼び出し
            protocol: grpc
            latency: { p50: 5ms }

  product-db:
    kind: database
    operations:
      - name: GetProduct
        calls: []                          # ← leaf service (下流呼び出しなし)

  product-cache:
    kind: cache
    operations:
      - name: GetProduct
        calls: []

  recommender:
    kind: application
    operations:
      - name: Recommend
        calls: []

  audit-log:
    kind: external_api
    operations:
      - name: Append
        calls: []
```

### `calls:` の要素 (CallNode)

各 `calls` 配列の要素は **2 つの形** のうちいずれか:

#### 形 1: 単一の呼び出し (leaf)

```yaml
- to: { service: <svc>, operation: <op> }
  protocol: http | grpc | messaging
  latency: { distribution: ..., p50: ..., p95: ... }
  error_rate: 0.001
  timeout: 1s
  retries: 2
  retry_backoff: exponential | linear | constant
  on_failure: { ... }                      # optional
```

#### 形 2: 並列グループ (fan-out)

```yaml
- parallel:
    - { to: ..., protocol: ..., ... }      # 単一呼び出し (再帰可能)
    - { to: ..., protocol: ..., ... }
    - parallel: [ ... ]                    # ネスト可
```

`parallel:` 配下のすべての要素を `sync.WaitGroup` で並行実行 → join。

### `on_failure:` (RecoveryPolicy)

```yaml
on_failure:
  fallback:                                # 順序付きフォールバックチェーン
    - to: { service: ..., operation: ... }
      protocol: ...
      latency: ...
      error_rate: ...
    - to: ...                              # さらなる代替先 (任意の段数)
  on_exhausted: propagate                  # 全 fallback 失敗時の挙動
  # OR
  on_exhausted: return_default
  default_response:                        # on_exhausted=return_default のときのデフォルト
    cached_value: stub
    is_stale: true
```

`on_exhausted` の値:
- `propagate` — ステップ失敗扱い → **下流にカスケード伝播** (リカバリー枯渇)
- `return_default` — `default_response` の attribute を使ってステップ成功扱い (`fallback.default_used=true` で観察可能)
- `succeed_silently` — 例: best-effort 書き込み。ステップ成功扱い、特に追加 attribute なし

---

## `journeys:` セクション

ジャーニーは **operation の高レベルなシーケンス**。各 step は「ある service の operation を起動する」だけ。その operation の `calls:` を経由した下流呼び出しは **自動的に展開** される (operation tree traversal)。

```yaml
journeys:
  view-product:                            # ← Journey 名
    weight: 0.7                            # 重み付きジャーニー選択用 (デフォルト 1.0)
    steps:
      - service: frontend
        operation: GET /products/{id}

  list-then-view:
    weight: 0.3
    steps:
      - service: frontend
        operation: GET /products              # まずカテゴリ一覧
      - service: frontend
        operation: GET /products/{id}         # 続いて個別商品 (2 trace になる)
        # 各 step は独立した trace ルートを生む

  intense-flow:
    weight: 0.1
    steps:
      - parallel:                            # journey 自身でも parallel 可能 (たとえば「同時並行で操作する」を表現)
          - service: frontend
            operation: GET /products/{id}
          - service: frontend
            operation: POST /cart
```

各 `step` の意味:
- **エントリ operation を起動 → そこから先は operation tree を自動 traverse**
- 複数 step がある場合、ステップ間は **逐次** (前 step が完了してから次)
- ステップ自身に `parallel:` を使うと、step 同士の並列起動も可能 (一般的ではないが上級者向け)

### Step のミニマル形と将来拡張

最低限は `{service, operation}` のペア。将来必要になったら以下を追加可能:
- iteration ごとのパラメータ (例: `path: /products/42` のような URL パラメータ)
- step 個別の latency override
- step 内のループ (`repeat: 3`)

これらは MVP では含めない (`requirements.md` FR-8.1 の宣言的 API 範囲)。

---

## `faults:` セクション

障害注入は **トポロジーとは独立に宣言** し、Parse 時に overlay として組み込まれる。3 つのターゲット種類:

```yaml
faults:
  # 1. operation 全体への障害 (= 該当 operation がいつも遅い/失敗する)
  - target: operation:product-db.GetProduct
    kind: latency_inflation
    severity:
      multiplier: 10.0                     # 通常レイテンシの 10 倍
      probability: 1.0                     # 常に発生

  # 2. 個別 edge への障害 (= 該当呼び出しが失敗する)
  - target: edge:catalog-service.GetProduct->product-db.GetProduct
    kind: disconnect
    severity: { probability: 1.0 }

  # 3. service 全体への障害 (= サービスのすべての operation が落ちる)
  - target: node:product-db
    kind: crash
    severity: { probability: 0.5 }
```

`kind:` 値:
- `latency_inflation` — レイテンシを増幅 (severity に `multiplier` または `add`)
- `error_rate_override` — error_rate を強制設定 (severity に `value`)
- `disconnect` — 接続失敗 (network error)
- `crash` — サービス停止 (全 operation が失敗)

### カスケード障害は条件付き (Q8 解釈)

faults は **エッジに直接届く**。エッジに `on_failure` が定義されていてフォールバックが成功すれば、下流にはカスケードしない。リカバリー枯渇 + `on_exhausted=propagate` のときのみ、Journey Engine が下流 step に `Cascaded=true` の Outcome を伝播する。

例: 上記の `product-db.GetProduct` への latency_inflation fault があっても、`catalog-service.GetProduct` の `on_failure.fallback` で `product-cache.GetProduct` を試行するので、product-cache が生きていればジャーニーは成功する。

---

## バリデーション (Parse + Validate)

`topology.Parse` は以下を検証 (失敗時は行番号付きエラー):

1. **YAML 構文**: yaml.v3 のデコードエラー
2. **参照解決**: `{service, operation}` ペアが実在する。未解決は Parse 時失敗
3. **DAG 性**: services の operation 間の `calls` グラフに循環がないこと (純粋な循環は false positive を起こす場合があるが、現実的な微サービストポロジーで循環は稀)
4. **journey の到達可能性**: journey steps の各 entry operation が実在し、その operation tree の全 operation が実在
5. **fault target の解決**: `operation:` / `edge:` / `node:` の参照先が実在

`Validate` (Parse の後段で呼ばれる) はさらに:

- service.kind と operations の整合 (例: `kind: database` なのに operations が無いと warning)
- `parallel:` のネスト深さチェック (NFR Design で上限決定)
- recovery chain のネスト深さチェック (同上)

---

## 完全な動作可能サンプル

```yaml
services:
  frontend:
    kind: application
    replicas: 2
    language: javascript
    operations:
      - name: GET /products/{id}
        calls:
          - to: { service: catalog-service, operation: GetProduct }
            protocol: http
            latency: { distribution: lognormal, p50: 10ms, p95: 100ms }
            error_rate: 0.001

  catalog-service:
    kind: application
    replicas: 3
    language: go
    operations:
      - name: GetProduct
        calls:
          - to: { service: product-db, operation: GetProduct }
            protocol: grpc
            latency: { distribution: lognormal, p50: 30ms, p95: 200ms }
            error_rate: 0.001
            timeout: 1s
            on_failure:
              fallback:
                - to: { service: product-cache, operation: GetProduct }
                  protocol: grpc
                  latency: { p50: 1ms, p95: 5ms }
                  error_rate: 0.0001
              on_exhausted: propagate

  product-db:
    kind: database
    operations:
      - name: GetProduct
        calls: []

  product-cache:
    kind: cache
    operations:
      - name: GetProduct
        calls: []

journeys:
  view-product:
    weight: 1.0
    steps:
      - service: frontend
        operation: GET /products/{id}

faults:
  - target: edge:catalog-service.GetProduct->product-db.GetProduct
    kind: disconnect
    severity: { probability: 1.0 }
```

このサンプルで k6 を実行すると:
1. `view-product` ジャーニーが iteration ごとに起動
2. `frontend.GET /products/{id}` が span を発行し `catalog-service.GetProduct` を呼ぶ
3. `catalog-service.GetProduct` が span を発行し `product-db.GetProduct` を呼ぶ
4. fault のため `product-db.GetProduct` への呼び出しは 100% 失敗
5. `on_failure.fallback` により `product-cache.GetProduct` を試行
6. cache の error_rate は 0.0001 なのでほぼ成功
7. **`catalog-service.GetProduct` ステップは成功扱い** (リカバリーが機能した)
8. trace には primary (失敗) と fallback (成功) の両 span が含まれる
9. metrics: `product-db` への RPS は通常通り (障害発生中)、`product-cache` への RPS が **急増** (fallback トラフィック)

これがまさに現実の **cache-aside パターン** の観察と一致する。

---

## JSON Schema エクスポート

`topology.Schema.ExportJSONSchema()` は同等の JSON Schema を返す (NFR-6.2)。エディタ補完用途。実装は Functional Design で確定。
