# U7 testutil/generators — Business Logic Model

## 目的と位置づけ

U7 は **テスト支援パッケージ** であり、業務ロジックを持つアプリケーションコンポーネントではない。本書での "business logic model" は **ジェネレータの組成戦略・進化モデル** を意味する。

PBT (Full enforcement) によって U1〜U6 の各テストが domain-specific generator を必要とする。U7 はそれを **集中管理** し、複数ユニット間で再利用させる (PBT-07 Generator Quality)。

---

## 1. 設計原則

| # | 原則 | 由来 |
|---|---|---|
| P-1 | **atomic ジェネレータを public でエクスポート、上位は組成** | Q3=A |
| P-2 | **Valid 系と Any 系を両方提供、prefix で識別** | Q2=A |
| P-3 | **functional options で柔軟にパラメータ化** | Q4=A |
| P-4 | **`<TypeName>()` 命名規約** (`ValidSchema()`, `AnyService()` 等) | Q5=A |
| P-5 | **realistic range をデフォルトに内蔵** | Q6=A、PBT-07 |
| P-6 | **rapid のシュリンカ/境界値探索に任せる** | Q7=A |
| P-7 | **U7 は各ユニット FD で incremental に拡張される** | Q8=A、Q4 of Units Generation |

---

## 2. パッケージ構造 (skeleton)

```text
testutil/
└── generators/
    ├── doc.go                  // パッケージドキュメント (PBT-09 framework, PBT-07 quality 記載)
    ├── options.go              // functional options 共通実装 (P-3)
    ├── primitives.go           // 共通プリミティブ (Identifier, Latency, Probability 等の atomic builders)
    ├── topology.go             // U1 型のジェネレータ (Schema, Service)  — 初期スコープ (Q1=A)
    ├── topology_invariants.go  // ValidSchema/Any の不変条件を実装するヘルパー (resolveReferences の test 用ミラー等)
    └── ...                     // U1 FD 以降の追加ファイル: operation.go, edge.go, journey.go, fault.go, ...
```

**初期 (Q1=A) は `topology.go` のみが具体的なジェネレータ実装を持つ**。`options.go` / `primitives.go` / `topology_invariants.go` は骨格で、U1 FD と並走で実装が育つ。

---

## 3. ジェネレータ組成パターン (P-1, P-3)

### Atomic → Composed

```text
ValidServiceID  ─┐
ValidLatency    ─┤
ValidProbability─┤── 組成 ──→  ValidEdge ──┐
ValidProtocol   ─┘                          ├── 組成 ──→ ValidOperation ──┐
                                            │                              ├── 組成 ──→ ValidService ──┐
ValidServiceKind ───────────────────────────┘                              │                            ├── 組成 ──→ ValidSchema
                                                                            │                            │
                                                                            └── (内部参照解決ロジック)──┘
```

(U1 FD で具体型の生成器が確定するまで、上図は予定。実装は段階的)

### Functional Options (P-3) の共通形

```go
package generators

type SchemaOption func(*schemaOptions)

type schemaOptions struct {
    maxServices       int
    maxOpsPerService  int
    maxCallsPerOp     int
    maxFaults         int
    // ... 将来追加
}

func defaultSchemaOptions() schemaOptions {
    return schemaOptions{
        maxServices:      10,
        maxOpsPerService: 5,
        maxCallsPerOp:    5,
        maxFaults:        3,
    }
}

func MaxServices(n int) SchemaOption       { return func(o *schemaOptions) { o.maxServices = n } }
func MaxOpsPerService(n int) SchemaOption  { return func(o *schemaOptions) { o.maxOpsPerService = n } }
// ...
```

ジェネレータ関数:

```go
// ValidSchema returns a rapid generator producing a *topology.Schema that
// always passes topology.Validate. Domain ranges (P-5) are realistic by default.
func ValidSchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema] {
    o := defaultSchemaOptions()
    for _, opt := range opts { opt(&o) }
    return rapid.Custom(func(t *rapid.T) *topology.Schema {
        // ... build services, operations, edges; resolve references; sanity-check
    })
}

// AnySchema is like ValidSchema but may produce structurally degenerate
// schemas (unresolved references, cycles, empty services). Use to test
// topology.Validate.
func AnySchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema] { ... }
```

---

## 4. Valid 系 vs Any 系の関係 (P-2)

| 区分 | 不変条件保証 | 利用シーン |
|---|---|---|
| **Valid 系** | 出力は必ず `Validate(s) == nil` を満たす | ロジック層 (Journey Engine 等) の PBT で前提条件として使う |
| **Any 系** | 構造的に degenerate (循環、未解決参照、空、極端値) も生成しうる | `topology.Validate` の不変条件確認、エラーパスのテスト |

実装戦略:
- **Valid 系**: 内部で構造を組み立てる際に **参照を確実に解決し、DAG を保つ** (例: edges の To は既に追加済みの service/operation からのみ選択)
- **Any 系**: 部分的に valid (生成器の確率分布で半数程度) + 部分的に invalid。`rapid.Filter` の併用ではなく、最初から両方を含む確率的ミックスにする (Filter は遅い)

両系統が同じ atomic 生成器 (`ValidServiceID()`, `AnyServiceID()`) から組み立てられ、コードの重複を最小化する。

---

## 5. 進化モデル (P-7)

```text
┌─────────────────────────────────────────────────────────────────┐
│ U7 FD (今回): skeleton + Schema/Service の Valid/Any            │
│                            ↓                                     │
│ U7 CG (今回): 上記の実装                                         │
│                            ↓                                     │
│ ┌─ U1 FD: topology の Testable Properties を識別                 │
│ │   ↓                                                            │
│ │  U1 FD ドキュメントに「U7 に必要な追加ジェネレータ」セクション │
│ │   (例: ValidOperation, ValidEdge, ValidJourney, FaultSpec 等)  │
│ │   ↓                                                            │
│ │  U7 code-generation-plan.md に項目追加 (Q8=A)                  │
│ │   ↓                                                            │
│ │  U1 CG 中に U7 の追加実装をあわせて行う                        │
│ │   ↓                                                            │
│ │  U1 完了                                                       │
│ │                                                                │
│ │  同様に U4 → U3 → U2 → U5 → U6 で繰り返し                     │
└──┴──────────────────────────────────────────────────────────────┘
```

各ユニットの FD ドキュメント (`business-logic-model.md` 等) の末尾に **「U7 に必要な追加ジェネレータ」** セクションを必須項目として置く。U7 の code-generation-plan.md はそれらを統合した「育つ TODO リスト」となる。

---

## 6. テストフレームワーク仕様 (PBT-09 関連)

| 項目 | 値 |
|---|---|
| フレームワーク | `pgregory.net/rapid` |
| 採用バージョン | rapid 最新 stable (Functional Design で `go.mod` の minimum version 確定。NFR Design で固定値を入れる) |
| シュリンク | 既定 (rapid 自動シュリンカに任せる、P-6) |
| シード/再現性 | `RAPID_SEED` 環境変数 (rapid 既定) を CI ログに出力 (NFR-4.3) |
| カスタムジェネレータ | `rapid.Custom[T](func(t *rapid.T) T { ... })` |
| Filter | 性能影響大のため、Valid 系では避ける。テスト個別の `rapid.Check` 内で例外的に許可 |

---

## 7. データフロー (テスト実行時)

```text
   テスト関数
      ↓ rapid.Check(t, func(t *rapid.T) { ... })
   テスト関数内
      ↓ schema := generators.ValidSchema(generators.MaxServices(5)).Draw(t, "schema")
   ValidSchema()
      ↓ atomic 組み立て (ValidServiceID, ValidLatency, ...)
      ↓ 参照解決 (ノード集合確定 → エッジを既存ノードからのみ生成)
      ↓ DAG 検証 (内部)
   *topology.Schema (Validate を通る valid なスキーマ)
      ↓ テスト本体: schema を引数に渡してロジックを検証
```

---

## 8. 関係する他ユニットへの影響

| ユニット | U7 への期待 |
|---|---|
| U1 topology | `ValidSchema` / `ValidService` を使って `Parse(Marshal(s)) ≡ s` (PBT-02) を検証 |
| U2 journey | `ValidSchema` + journey 名指定で `BuildPlan` の冪等性検証 |
| U3 synth | (将来) `SpanInput`/`MetricInput` 生成器が必要 — U3 FD で確定 |
| U4 exporter | (将来) `Config` 生成器 + `MergeWith` の優先順位則検証 |
| U5/U6 | 主に integration test が中心。PBT 適用範囲は限定 |

---

## 9. 範囲外 (Out of Scope)

- **integration test ハーネス** — U8 で扱う。U7 はあくまで rapid generator 集約
- **モック/スタブ** — `Synthesizer` interface のモックは別途必要だが、それは U3 FD で扱う (`synth/mocks/` 等を独立検討)
- **テストカバレッジ計測ツール** — Go 標準 `go test -cover` で十分、ツール追加は不要
