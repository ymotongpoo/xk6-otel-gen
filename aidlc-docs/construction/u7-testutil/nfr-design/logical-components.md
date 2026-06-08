# U7 testutil/generators — Logical Components

本書は U7 内部の論理コンポーネント (パッケージ内サブモジュール / 関数群) とその責務を確定する。Application Design の物理パッケージ (`testutil/generators/`) は 1 つだが、その内部を論理的に分割して保守性を確保する。

---

## 1. ファイル構成と論理コンポーネントのマッピング

```text
testutil/generators/
├── doc.go                  // [LC-0] パッケージドキュメント
├── options.go              // [LC-1] Options Resolver
├── primitives.go           // [LC-2] Primitive Generators (atomic)
├── service.go              // [LC-3] Service Generator
├── schema.go               // [LC-4] Schema Generator (DAG builder + Any degradation)
├── mutators.go             // [LC-5] Schema Mutators (Any 系の degradation 注入)
├── primitives_test.go      // [LC-T1] テスト: primitives
├── service_test.go         // [LC-T2] テスト: service
├── schema_test.go          // [LC-T3] テスト: schema (Valid/Any 不変条件)
├── options_test.go         // [LC-T4] テスト: options 尊重
└── bench_test.go           // [LC-T5] ベンチマーク
```

Phase 0 で別途書く pre-U1 型骨格 (P-SKEL):

```text
topology/                   // (U7 Code Generation で骨格作成、U1 で本実装)
├── doc.go
├── types.go                // Schema, Service, ServiceID, Operation, CallNode, Edge,
                            //   Journey, Step, FaultSpec, FaultTarget, RecoveryPolicy
├── enums.go                // ServiceKind, Protocol, ExhaustedAction, FaultKind, TargetKind
└── (各メソッドは panic スタブ、AUTOGEN-MARKER-U1 コメント付き)
```

---

## 2. 論理コンポーネント詳細

### [LC-1] Options Resolver (`options.go`)

#### 責務
- `SchemaOption` / `ServiceOption` 型の定義
- 各 Option 関数 (`MaxServices`, `MaxOpsPerService`, ...) の定義と値域 clamp
- `defaultXxxOptions()` の初期値定数
- options struct の `apply(opts []Option)` ヘルパー (内部)

#### 公開 API
```go
type SchemaOption func(*schemaOptions)
type ServiceOption func(*serviceOptions)

func MaxServices(n int) SchemaOption
func MaxOpsPerService(n int) SchemaOption    // SchemaOption と ServiceOption 両方の場面
func MaxCallsPerOp(n int) SchemaOption
func MaxFaults(n int) SchemaOption
func BiasValid(p float64) SchemaOption
func WithKind(k topology.ServiceKind) ServiceOption
```

#### 不変条件
- すべての Option 関数は **値域を正規化** (例: `MaxServices(-3)` は 1 に clamp、`BiasValid(1.5)` は 1.0 に clamp)
- options struct は unexported、外部からフィールド直接設定不可
- スレッドセーフ (Option 関数は副作用なし、options struct は per-draw)

### [LC-2] Primitive Generators (`primitives.go`)

#### 責務
- 個別ドメイン型の rapid generator を提供 (Layer 1 atomic、`nfr-design-patterns.md` §2 P-COMP-1)
- 各 generator は `*rapid.Generator[T]` を返す関数

#### 公開 API (初期スコープ)
```go
// 文字列・識別子
func ValidServiceID() *rapid.Generator[topology.ServiceID]
func AnyServiceID()   *rapid.Generator[topology.ServiceID]
func ValidOperationName() *rapid.Generator[string]
func AnyOperationName()   *rapid.Generator[string]

// 確率・数値
func ValidProbability() *rapid.Generator[float64]
func AnyProbability()   *rapid.Generator[float64]
func ValidReplicaCount() *rapid.Generator[int]
func AnyReplicaCount()   *rapid.Generator[int]

// 時間
func ValidLatencyPair() *rapid.Generator[LatencyPair]
func AnyLatencyPair()   *rapid.Generator[LatencyPair]
func ValidTimeout() *rapid.Generator[time.Duration]
func AnyTimeout()   *rapid.Generator[time.Duration]

// enum (Valid only)
func ValidServiceKind() *rapid.Generator[topology.ServiceKind]
func ValidProtocol()    *rapid.Generator[topology.Protocol]

// helper 型
type LatencyPair struct {
    P50 time.Duration
    P95 time.Duration
}
```

#### 不変条件
- Valid 系: R-V-1..5 (business-rules.md §1)、R-DOM-1..6 (§4)
- Any 系: R-A-1..4 (§2)
- すべて P-PERF-1 (rapid.Custom 中心) で実装

### [LC-3] Service Generator (`service.go`)

#### 責務
- `ValidService(opts ...ServiceOption) *rapid.Generator[*topology.Service]`
- `AnyService(opts ...ServiceOption) *rapid.Generator[*topology.Service]`
- Service 単体の構築 (Operations は持つが、Calls は **空 or 後段で注入される**)

#### 公開 API
```go
func ValidService(opts ...ServiceOption) *rapid.Generator[*topology.Service]
func AnyService(opts ...ServiceOption)   *rapid.Generator[*topology.Service]
```

#### 内部設計
```go
func ValidService(opts ...ServiceOption) *rapid.Generator[*topology.Service] {
    o := defaultServiceOptions()
    for _, opt := range opts { opt(&o) }
    return rapid.Custom(func(t *rapid.T) *topology.Service {
        svc := &topology.Service{
            Name: ValidServiceID().Draw(t, "name"),
            Kind: o.fixedKind, // または ValidServiceKind().Draw(...)
            Replicas: ValidReplicaCount().Draw(t, "replicas"),
            Operations: make(map[string]*topology.Operation),
        }
        opCount := rapid.IntRange(1, o.maxOpsPerService).Draw(t, "n_ops")
        names := rapid.SliceOfNDistinct(ValidOperationName(), opCount, opCount, idFn).Draw(t, "op_names")
        for _, name := range names {
            svc.Operations[name] = &topology.Operation{
                Name: name,
                Service: svc,  // back-pointer 即時設定 (P-PERF-3)
                Calls: nil,    // ValidSchema 内で後段に注入される
            }
        }
        return svc
    })
}
```

#### 不変条件
- R-STR-1, R-STR-2 (back-pointer 整合)
- ValidService 単体の出力は `topology.Validate` を **直接は通らない** (cross-service の edge / Journey / Fault がないと不完全)
  → 「Service 単体としては valid 状態」を意味する: Service.Name 整合、Operations.back-pointer 整合
  → `ValidSchema` 内で組み立てに使う中間素材として完結性を持つ

### [LC-4] Schema Generator (`schema.go`)

#### 責務
- `ValidSchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema]`
- `AnySchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema]`
- **DAG builder** (topological order を使った Edge 構築、P-PERF-2)
- **Journey & Faults builder** (`ValidSchema` 内のサブステップ)

#### 公開 API
```go
func ValidSchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema]
func AnySchema(opts ...SchemaOption)   *rapid.Generator[*topology.Schema]
```

#### 内部設計 (主要関数)

```go
func ValidSchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema] {
    o := defaultSchemaOptions()
    for _, opt := range opts { opt(&o) }
    return rapid.Custom(func(t *rapid.T) *topology.Schema {
        schema := &topology.Schema{
            Services: make(map[topology.ServiceID]*topology.Service),
            Journeys: make(map[string]*topology.Journey),
        }
        // Step 1-2: services + operations
        topoOrder := buildServicesAndOperations(t, schema, o)
        // Step 3-4: edges (DAG ordered)
        buildEdges(t, topoOrder, o)
        // Step 5: journeys
        buildJourneys(t, schema, topoOrder, o)
        // Step 6: faults
        buildFaults(t, schema, topoOrder, o)
        // (本実装後の) Step 7: 念のため Validate
        // ※ pre-U1 では panic スタブなのでスキップ。
        //   U1 完了後にこの一行を有効化する: if err := topology.Validate(schema); err != nil { t.Fatal(err) }
        return schema
    })
}

// buildServicesAndOperations は services と operations を生成し、operation を
// 全順序リストにして返す (DAG ordering 用)。
func buildServicesAndOperations(t *rapid.T, schema *topology.Schema, o schemaOptions) []*topology.Operation { ... }

// buildEdges は topoOrder の各 operation について、自分より後ろの operation のみを
// target に選んで Calls を構築する。これにより DAG が構築時に保証される。
func buildEdges(t *rapid.T, topoOrder []*topology.Operation, o schemaOptions) { ... }

func buildJourneys(t *rapid.T, schema *topology.Schema, topoOrder []*topology.Operation, o schemaOptions) { ... }
func buildFaults(t *rapid.T, schema *topology.Schema, topoOrder []*topology.Operation, o schemaOptions) { ... }
```

#### 不変条件
- R-STR-1〜R-STR-8 すべて (Valid 系)
- DAG 性 (R-STR-4) は **構築時に保証** (P-PERF-2)
- 全 *Operation / *Edge ポインタは non-nil

### [LC-5] Schema Mutators (`mutators.go`)

#### 責務
- `AnySchema` で使う **degradation 関数群** (P-COMP-3)
- 各 mutator は valid schema を入力にとり、1 箇所だけ "壊した" schema を返す

#### 内部 API (unexported)
```go
package generators

// mutator は valid schema を 1 箇所だけ "壊した" schema に変換する関数。
type mutator func(t *rapid.T, s *topology.Schema) *topology.Schema

var schemaMutators = []mutator{
    unresolveEdgeTarget,   // R-STR-3 違反: Edge.To に schema 外の Operation を入れる
    introduceCycle,        // R-STR-4 違反: 既存の 2 つの edge を逆向きに張り直す
    misreferenceJourney,   // R-STR-5 違反: Journey.Steps[*].Op に存在しない Operation
    misreferenceFault,     // R-STR-6 違反: FaultSpec.Target に存在しないターゲット
    dropServiceMap,        // R-STR-1 違反: services マップから 1 件 delete (それを指す edge が orphan に)
    breakBackPointer,      // R-STR-2 違反: Operation.Service を別 Service にすり替え
    violateCallNodeVariant,// R-STR-7 違反: CallNode に Edge と Parallel の両方を入れる
    misownFallback,        // R-STR-8 違反: RecoveryPolicy.Fallback[0].From を別 Operation に
}

func unresolveEdgeTarget(t *rapid.T, s *topology.Schema) *topology.Schema { ... }
func introduceCycle(t *rapid.T, s *topology.Schema) *topology.Schema      { ... }
// ... 各 mutator の実装
```

#### 不変条件
- 各 mutator は入力 schema を **mutate せず**、コピーを返す (テスト分離)
- 各 mutator は **必ず 1 箇所だけ違反** を導入 (複数違反を作らない: 違反種類の独立検証のため)

### [LC-0] Package Documentation (`doc.go`)

```go
// Package generators provides domain-specific PBT (property-based testing)
// generators for the xk6-otel-gen project, built on pgregory.net/rapid.
//
// All public generators return *rapid.Generator[T] and must be drawn within
// a rapid.Check or rapid.MakeCheck call. See pgregory.net/rapid documentation
// for usage details.
//
// Generator naming:
//   - Valid<TypeName>() — guaranteed to produce values that satisfy domain
//                          invariants (e.g., topology.Validate passes).
//   - Any<TypeName>()   — may produce structurally degenerate values,
//                          useful for testing validation logic.
//
// Generators are composable via functional options (e.g., MaxServices(10)).
// Both Valid and Any flavors share atomic primitives where possible.
//
// PBT compliance: This package satisfies PBT-07 (Generator Quality) and
// PBT-09 (Framework Selection) per the project's AI-DLC PBT extension.
//
// See also:
//   - aidlc-docs/construction/u7-testutil/functional-design/ for design rationale.
//   - aidlc-docs/construction/u7-testutil/nfr-design/ for performance and patterns.
package generators
```

---

## 3. テスト論理コンポーネント

### [LC-T1] `primitives_test.go`
- ValidServiceID が `^[a-z][a-z0-9-]{2,30}$` にマッチ (PBT)
- ValidLatencyPair の p95 ≥ p50 (R-DOM-1) (PBT, TP-U7-6)
- AnyServiceID は invalid を含む (statistical PBT)
- 各 enum generator は対象 enum 値のみを返す (PBT)

### [LC-T2] `service_test.go`
- ValidService.Operations の back-pointer (R-STR-2) (PBT)
- ValidService.Name は ValidServiceID 範囲内 (PBT)
- ValidService.Operations が空でない (PBT)

### [LC-T3] `schema_test.go`
- TP-U7-1: ValidSchema は valid (PBT)
- TP-U7-2: ValidSchema は DAG (PBT、トポロジカルソートで verify)
- TP-U7-3: AnySchema 100 draw 中 invalid を含む (statistical PBT)
- TP-U7-5: 2 回 draw は異なる schema を出しうる (statistical PBT)
- (TP-U7-1 / TP-U7-2 は `topology.Validate` が U1 で実装されるまで panic スタブ — それまでは構造的チェックだけ行う meta-validator を使う)

### [LC-T4] `options_test.go`
- TP-U7-4: ValidSchema(MaxServices(N)) で len(Services) ≤ N (PBT)
- MaxOpsPerService, MaxCallsPerOp 等の各 Option について類似テスト

### [LC-T5] `bench_test.go`
- `BenchmarkValidSchemaDraw` (NFR-U7-6 検証)
- `b.ReportAllocs()` を有効化

---

## 4. カバレッジターゲットの達成戦略 (NFR-U7-5)

80% カバレッジを達成するため:

| ファイル | 想定カバレッジ寄与 |
|---|---|
| options.go (~50 LOC) | 100% (各 Option を options_test で網羅) |
| primitives.go (~150 LOC) | 90%+ (primitives_test で各 generator を draw) |
| service.go (~80 LOC) | 90%+ (service_test) |
| schema.go (~200 LOC) | 80%+ (schema_test、edge cases の一部は test ベース) |
| mutators.go (~150 LOC) | 80%+ (statistical PBT で各 mutator がカバーされる、追加で example-based 各 mutator 1 件) |

合計 LOC ~650、達成可能性は十分。

---

## 5. PBT-07 Generator Quality verification

| PBT-07 verification 項目 | LC への対応 |
|---|---|
| ドメインオブジェクトに対し custom generator | LC-2 (primitives), LC-3 (service), LC-4 (schema) |
| プリミティブをドメインの wrapper にする | LC-2 が wrapper を提供 |
| ドメイン制約を尊重 (R-DOM-*, R-STR-*) | LC-2..4 + business-rules.md §3-5 |
| 境界値の自然な含有 | rapid デフォルト挙動 (P-PERF-1 で説明) |
| 再利用可能なドメイン generator | LC-2..5 が全 unit のテストから import 可 (公開パッケージ) |

---

## 6. 物理依存図 (パッケージレベル)

```mermaid
graph TD
    Generators["testutil/generators/"]
    Topology["topology/<br/>(skeleton scaffolded by U7,<br/>methods filled by U1)"]
    Rapid["pgregory.net/rapid"]

    Generators -->|imports types| Topology
    Generators -->|imports framework| Rapid
    Topology -.->|uses (test only)| Generators

    classDef u7 fill:#FFF59D,stroke:#F57F17,color:#000
    classDef u1 fill:#90CAF9,stroke:#0D47A1,color:#000
    classDef ext fill:#E0E0E0,stroke:#424242,color:#000
    class Generators u7
    class Topology u1
    class Rapid ext
```

- 実線: build-time import 依存
- 点線: test-only 依存 (U1 のテストが U7 を import する将来形)

---

## 7. 構築フェーズと完了基準

### Phase 0: pre-U1 type skeleton (P-SKEL-1〜4)
- `topology/types.go`, `topology/enums.go`, `topology/doc.go` を作成
- メソッド本体は panic スタブ + `// NOTE: implementation in U1 Code Generation.`
- `topology/` ディレクトリで `go build ./topology/...` が通る

### Phase 1: U7 logical components 実装
- LC-0〜LC-5 を順に実装
- 各実装後に対応する LC-T テストを追加

### Phase 2: ベンチマーク + カバレッジ確認
- `BenchmarkValidSchemaDraw` で 1ms/draw を確認 (NFR-U7-6)
- `go test -cover` で 80% 以上 (NFR-U7-5)
- `go test -race` で race なし (NFR-U7-8)
- `golangci-lint run` で警告なし (NFR-U7-9 の支援)

### Phase 3: ドキュメント仕上げ
- doc.go と各 public 関数の GoDoc
- ExampleValidSchema, ExampleValidService, ExampleAnySchema (P-DOC-1)

各 Phase の詳細チェックリストは **U7 Code Generation Plan** で確定する。
