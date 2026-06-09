# U7 testutil/generators — Domain Entities (Initial Generator Catalog)

本書は **U7 の初期リリースで提供するジェネレータの完全なカタログ** を示す (Q1=A、最小骨格 `Schema()` / `Service()` ペア + 共有プリミティブ)。後続ユニット (U1 以降) の FD で追加されるジェネレータは、本カタログを拡張する形で同パッケージに追加される (Q8=A)。

---

## 1. 共有プリミティブ (atomic / 全ジェネレータの基礎)

### 1.1 文字列・識別子系

```go
package generators

// ValidServiceID returns a generator producing a topology.ServiceID
// matching ^[a-z][a-z0-9-]{2,30}$ (kebab-case ASCII, length 3-31).
func ValidServiceID() *rapid.Generator[topology.ServiceID]

// AnyServiceID returns a generator producing any string cast to ServiceID,
// including invalid ones (uppercase, spaces, empty, too long).
func AnyServiceID() *rapid.Generator[topology.ServiceID]

// ValidOperationName returns a generator producing operation names like
// "GET /products/{id}", "GetProduct", "process-message" — UTF-8 strings
// of length 1-120 with optional "/", "{", "}" tokens.
func ValidOperationName() *rapid.Generator[string]

// AnyOperationName returns a generator possibly producing empty or
// over-length strings.
func AnyOperationName() *rapid.Generator[string]
```

### 1.2 数値・確率系

```go
// ValidProbability returns a generator producing a float64 in [0.0, 1.0].
func ValidProbability() *rapid.Generator[float64]

// AnyProbability returns a generator possibly producing negative,
// NaN, +Inf, or values > 1.0.
func AnyProbability() *rapid.Generator[float64]

// ValidReplicaCount returns a generator producing an int in [1, 100].
func ValidReplicaCount() *rapid.Generator[int]

// AnyReplicaCount returns a generator possibly producing 0 or negative.
func AnyReplicaCount() *rapid.Generator[int]
```

### 1.3 時間・分布系

```go
// ValidLatencyPair returns a generator producing (p50, p95) durations
// in realistic ranges with p95 >= p50 (R-DOM-1).
//   p50 ∈ [1ms, 5s], p95 ∈ [p50, 30s]
func ValidLatencyPair() *rapid.Generator[LatencyPair]

// AnyLatencyPair may produce p95 < p50 (violates R-DOM-1) or negative.
func AnyLatencyPair() *rapid.Generator[LatencyPair]

type LatencyPair struct {
    P50 time.Duration
    P95 time.Duration
}
// LatencyPair is a small helper type used for testing; it is NOT exported
// from topology/. The mapping to topology.LatencyDist is done by callers
// (e.g., ValidEdge() inside the generator).

// ValidTimeout returns a generator producing a time.Duration in [100ms, 60s].
func ValidTimeout() *rapid.Generator[time.Duration]

// AnyTimeout may produce 0 or negative durations.
func AnyTimeout() *rapid.Generator[time.Duration]
```

### 1.4 enum 系 (Go の型システムで invalid 値が作れないため、Valid のみ)

```go
// ValidServiceKind returns a generator drawing from
// {application, database, external_api, cache, queue}.
func ValidServiceKind() *rapid.Generator[topology.ServiceKind]

// ValidProtocol returns a generator drawing from {http, grpc, messaging}.
func ValidProtocol() *rapid.Generator[topology.Protocol]
```

---

## 2. 初期リリースのトップレベルジェネレータ (Q1=A)

### 2.1 ValidService / AnyService

```go
// ServiceOption is a functional option for Service generator tuning.
type ServiceOption func(*serviceOptions)

func MaxOpsPerService(n int) ServiceOption       { ... }
func WithKind(k topology.ServiceKind) ServiceOption { ... }
// ... future options

// ValidService returns a generator producing a *topology.Service satisfying:
//   - R-STR-2: Operations の back-pointer 整合 (Operation.Service == 親 *Service)
//   - Name は ValidServiceID() の出力
//   - Replicas は ValidReplicaCount() の出力
//   - Operations は空ではない (>=1 個)、各 Operation の Calls は空でも OK
//     (leaf service の表現)
// ただし Edges (Operation.Calls) は **このジェネレータ単独では設定しない**
// (cross-service エッジは ValidSchema が組み立て時に注入)。
func ValidService(opts ...ServiceOption) *rapid.Generator[*topology.Service]

// AnyService may produce services with:
//   - 空の Operations
//   - 不整合な back-pointer
//   - invalid ServiceID
func AnyService(opts ...ServiceOption) *rapid.Generator[*topology.Service]
```

### 2.2 ValidSchema / AnySchema

```go
type SchemaOption func(*schemaOptions)

func MaxServices(n int) SchemaOption          { ... }
func MaxOpsPerService(n int) SchemaOption     { ... }
func MaxCallsPerOp(n int) SchemaOption        { ... }
func MaxFaults(n int) SchemaOption            { ... }
func BiasValid(p float64) SchemaOption        { ... } // Any 系で valid 比率を歪める

// ValidSchema returns a generator producing a *topology.Schema satisfying:
//   - R-STR-1 〜 R-STR-8 すべて
//   - topology.Validate(schema) == nil
//   - Services 数 ∈ [1, MaxServices] (default 10)
//   - Operations.Calls はクロスサービスの edge を含み、必ず DAG
//   - Journeys は 1 件以上、各 entry operation が実在
//   - Faults は 0〜MaxFaults 件、すべて実在のターゲットを指す
func ValidSchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema]

// AnySchema may produce:
//   - Operations.Calls に未解決参照 (R-STR-3 違反)
//   - DAG 違反の循環 (R-STR-4 違反)
//   - Journeys に存在しない operation を参照 (R-STR-5 違反)
//   - Faults に存在しないターゲットを指す spec (R-STR-6 違反)
//   - 構造的整合は保つ (Go 型レベルで組み立て可能、nil access なし)
// BiasValid(p) で valid 出力比率を調整 (default p=0.5)。
func AnySchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema]
```

---

## 3. 参考実装スケッチ

### 3.1 ValidSchema の組み立てアルゴリズム概略

```go
func ValidSchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema] {
    o := defaultSchemaOptions()
    for _, opt := range opts { opt(&o) }
    return rapid.Custom(func(t *rapid.T) *topology.Schema {
        // 1. サービス数を決定し、空の Service マップを構築
        n := rapid.IntRange(1, o.maxServices).Draw(t, "n_services")
        services := make(map[topology.ServiceID]*topology.Service, n)
        names := rapid.SliceOfNDistinct(ValidServiceID(), n, n, func(s topology.ServiceID) topology.ServiceID { return s }).Draw(t, "svc_names")
        for _, name := range names {
            kind := ValidServiceKind().Draw(t, fmt.Sprintf("%s.kind", name))
            replicas := ValidReplicaCount().Draw(t, fmt.Sprintf("%s.replicas", name))
            services[name] = &topology.Service{
                Name: name, Kind: kind, Replicas: replicas,
                Operations: make(map[string]*topology.Operation),
            }
        }

        // 2. 各サービスに operations を生成 (back-pointer も同時設定)
        for _, svc := range services {
            opCount := rapid.IntRange(1, o.maxOpsPerService).Draw(t, ...)
            opNames := rapid.SliceOfNDistinct(ValidOperationName(), opCount, opCount, ...).Draw(t, ...)
            for _, opName := range opNames {
                op := &topology.Operation{Name: opName, Service: svc, Calls: nil}
                svc.Operations[opName] = op
            }
        }

        // 3. DAG 順序を決定し、各 operation の Calls を順序内で構築
        //    (上流のサービス → 下流のサービスのみエッジを張る、循環防止)
        topoOrder := computeTopoOrder(services, t)
        for i, svc := range topoOrder {
            for _, op := range svc.Operations {
                callCount := rapid.IntRange(0, o.maxCallsPerOp).Draw(t, ...)
                downstream := topoOrder[i+1:]  // この後の順位のサービスのみ
                if len(downstream) == 0 || callCount == 0 {
                    continue
                }
                for c := 0; c < callCount; c++ {
                    targetSvc := rapid.SampledFrom(downstream).Draw(t, ...)
                    targetOpName := rapid.SampledFrom(keys(targetSvc.Operations)).Draw(t, ...)
                    edge := &topology.Edge{
                        From: op,
                        To:   targetSvc.Operations[targetOpName],
                        Protocol: ValidProtocol().Draw(t, ...),
                        // ... latency, error_rate 等
                    }
                    op.Calls = append(op.Calls, &topology.CallNode{Edge: edge})
                }
            }
        }

        // 4. Journeys を生成 (各 step は実在 operation を指す)
        // 5. Faults を生成 (実在ターゲットを指す)

        schema := &topology.Schema{Services: services, Journeys: ..., Faults: ...}
        // 6. 念のためバリデーション (失敗したら shrinker が自動的に縮める)
        if err := topology.Validate(schema); err != nil {
            t.Fatalf("ValidSchema produced invalid schema: %v", err)
        }
        return schema
    })
}
```

(実装の最終形は Code Generation で確定。上記はあくまで設計スケッチ)

### 3.2 AnySchema の劣化注入

```go
func AnySchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema] {
    // 確率 BiasValid (default 0.5) で ValidSchema をそのまま返す
    // 確率 1 - BiasValid で、ValidSchema の出力を 1 箇所だけ "破壊" する:
    //   - 既存 edge の To を未登録 operation に置換 (R-STR-3 違反)
    //   - 2 つの operation の Calls を相互参照させて循環を作る (R-STR-4)
    //   - journey の step.Op を nil 化、または別 schema の op に置換 (R-STR-5)
    //   - fault.Target を未登録に置換 (R-STR-6)
    // 各破壊種類は均等に選ばれる
    ...
}
```

---

## 4. 初期リリース範囲外 (= U1 FD で追加予定のジェネレータ)

これらは **U7 の初期リリースには含めない**。各ユニットの FD でそのユニットが必要とするものを追加する (Q8=A):

| 後で追加するジェネレータ | 追加タイミング (どのユニット FD で出てくる予定か) |
|---|---|
| `ValidOperation`, `AnyOperation` (single-Operation) | U1 FD |
| `ValidCallNode`, `AnyCallNode` (Edge / Parallel variant) | U1 FD |
| `ValidEdge`, `AnyEdge` (single-Edge) | U1 FD |
| `ValidRecoveryPolicy`, `AnyRecoveryPolicy` | U1 FD |
| `ValidJourney`, `AnyJourney`, `ValidStep`, `AnyStep` | U1 FD |
| `ValidFaultSpec`, `AnyFaultSpec`, `ValidFaultTarget`, `AnyFaultTarget` | U1 FD |
| `ValidFaultOverlay` | U1 FD |
| `ValidConfig` (exporter.Config), `AnyConfig` | U4 FD |
| `ValidPlan`, `ValidNode`, `ValidOutcome` (journey) | U2 FD |
| `ValidSpanInput`, `ValidMetricInput`, `ValidLogInput` (synth) | U3 FD |

(U1 FD では Operation/Edge を atomic として独立に draw できる必要が出てくる。たとえば「同じ operation を 2 回 marshal → parse して invariant 検証」など、Schema 全体まで作る必要がないテストのため)

---

## 5. 関数シグネチャ一覧 (初期リリース)

```text
ValidServiceID    () *rapid.Generator[topology.ServiceID]
AnyServiceID      () *rapid.Generator[topology.ServiceID]
ValidOperationName() *rapid.Generator[string]
AnyOperationName  () *rapid.Generator[string]
ValidProbability  () *rapid.Generator[float64]
AnyProbability    () *rapid.Generator[float64]
ValidReplicaCount () *rapid.Generator[int]
AnyReplicaCount   () *rapid.Generator[int]
ValidLatencyPair  () *rapid.Generator[LatencyPair]
AnyLatencyPair    () *rapid.Generator[LatencyPair]
ValidTimeout      () *rapid.Generator[time.Duration]
AnyTimeout        () *rapid.Generator[time.Duration]
ValidServiceKind  () *rapid.Generator[topology.ServiceKind]
ValidProtocol     () *rapid.Generator[topology.Protocol]
ValidService      (opts ...ServiceOption) *rapid.Generator[*topology.Service]
AnyService        (opts ...ServiceOption) *rapid.Generator[*topology.Service]
ValidSchema       (opts ...SchemaOption)  *rapid.Generator[*topology.Schema]
AnySchema         (opts ...SchemaOption)  *rapid.Generator[*topology.Schema]
```

オプション関数 (初期):

```text
SchemaOption:
  MaxServices(n int)
  MaxOpsPerService(n int)
  MaxCallsPerOp(n int)
  MaxFaults(n int)
  BiasValid(p float64)

ServiceOption:
  MaxOpsPerService(n int)
  WithKind(k topology.ServiceKind)
```

---

## 6. U7 自身のテスト (PBT-01 適用先)

`business-rules.md` §10 で定義した TP-U7-1 〜 TP-U7-6 を、`testutil/generators/*_test.go` で:

- `TestValidSchemaPassesValidate` (example-based, sanity) — 5 個 draw 試行
- `TestValidSchemaProperty_PassesValidate` (PBT) — `rapid.Check` 100 回
- `TestAnySchemaProperty_ContainsInvalid` (statistical PBT) — 100 draw 中 invalid が >= 1
- `TestValidSchemaRespectsMaxServices` (PBT) — オプション尊重
- `TestValidLatencyPairProperty_P95GeqP50` (PBT) — R-DOM-1 不変条件

実装は U7 Code Generation で。

---

## 7. ドキュメンテーション (GoDoc)

各 public ジェネレータの GoDoc は以下を含むこと:

- 短い 1 行概要 (`// ValidSchema returns a generator producing a valid *topology.Schema.`)
- 不変条件の参照 (`// See business-rules.md §5 R-STR-1..R-STR-8.`)
- レンジ (`// Defaults: max 10 services, max 5 ops per service, ...`)
- オプション一覧 (`// Use MaxServices(n) to bound the service count.`)
- 使用例 (1〜2 行のスニペット)

---

## 8. U7 に必要な追加ジェネレータ (各ユニット FD から U7 へのリクエスト一覧)

このセクションは **U1 以降の各ユニット FD で書かれる "U7 への追加リクエスト" を統合する場所**。各ユニット FD は自分が必要とするジェネレータを以下に追記する責任を持つ (Q8=A 規約)。

### Request from U1 FD (COMPLETED in U1 Code Generation Phase 13)
- ValidOperation, AnyOperation
- ValidEdge, AnyEdge
- ValidCallNode, AnyCallNode
- ValidRecoveryPolicy, AnyRecoveryPolicy
- ValidJourney, AnyJourney
- ValidStep, AnyStep
- ValidFaultSpec, AnyFaultSpec
- ValidFaultTarget, AnyFaultTarget
- ValidFaultOverlay, AnyFaultOverlay
- Files added: testutil/generators/{operation,edge,callnode,recovery,journey,fault}.go
