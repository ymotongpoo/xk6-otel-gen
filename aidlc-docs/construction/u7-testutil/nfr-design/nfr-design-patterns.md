# U7 testutil/generators — NFR Design Patterns

本書は U7 が採用する **設計パターン** を確定する。NFR Requirements で識別した NFR-U7-1〜10 を実現するための具体的な実装規範。

---

## 1. Performance Patterns

### P-PERF-1: Custom 中心の generator 構築 (Q1=A)

すべての rapid generator は `rapid.Custom[T](func(t *rapid.T) T)` を骨格として実装する。プリミティブ (`rapid.IntRange`, `rapid.SampledFrom`, `rapid.StringMatching` 等) は内部で組合せ使用するが、generator の **外向き API シグネチャ** は常に `*rapid.Generator[T]` を返す `rapid.Custom` ラッパー。

```go
func ValidLatencyPair() *rapid.Generator[LatencyPair] {
    return rapid.Custom(func(t *rapid.T) LatencyPair {
        p50 := time.Duration(rapid.IntRange(1, 5_000).Draw(t, "p50_ms")) * time.Millisecond
        p95 := time.Duration(rapid.IntRange(int(p50/time.Millisecond), 30_000).Draw(t, "p95_ms")) * time.Millisecond
        return LatencyPair{P50: p50, P95: p95}
    })
}
```

**理由**: 
- `rapid.Custom` は shrinker の挙動が直感的 (draw 順序通りに shrink される)
- デバッグ時に rapid の trace ログを読みやすい (`p50_ms` のような label が出る)
- `rapid.Map` / `rapid.FlatMap` は型変換ベースで一見軽量だが、shrink ロジックの追跡が困難

### P-PERF-2: Topological-order DAG 構築 (Q3=A)

`ValidSchema()` の DAG 性 (R-STR-4) は **構築時に強制**。次の手順で `Operation` ノードの全順序を決め、エッジは順位の小さい側 → 大きい側にのみ張る:

```text
1. Service 数 N を draw
2. Service 名を N 個 distinct に draw → 任意の総順序 (service_order[0..N-1])
3. 各 Service で operation 数を draw、Service 内 operation 名を distinct に draw
   → Operation の総順序 = lexicographic((service_index, operation_index_within_service))
4. 各 Operation について Calls を構築:
     可能な call target = それより順位が大きい (service_index, operation_index) の全 Operation
     callCount を draw、target を rapid.SampledFrom で選択
5. ParallelGroup を作る場合も、同じ「順位が大きい側のみ」制約を保つ
```

これにより:
- 構築完了時点で必ず DAG (R-STR-4 不変条件)
- `rapid.Filter` で後処理する必要なし → NFR-U7-6 (1ms/draw) 達成しやすい
- 決定論的: 同じシードで同じ schema が再現される

### P-PERF-3: 参照解決を構築と同時に行う (R-STR-1〜3, R-STR-6)

`Service` を生成しながら `Operation` を作り、Operation の back-pointer (`Operation.Service`) を **その場で設定**。エッジを張るときも `Edge.From` / `Edge.To` を **実際の `*Operation` で直接代入** する (後追い resolve なし)。

```go
for _, name := range serviceNames {
    svc := &topology.Service{Name: name, ...}
    schema.Services[name] = svc
    for _, opName := range opNames {
        op := &topology.Operation{Name: opName, Service: svc}  // back-pointer 即時設定
        svc.Operations[opName] = op
    }
}
// 後段で Edge を構築
edge := &topology.Edge{
    From: callerOp,  // *Operation を直接代入
    To:   calleeOp,
    ...
}
```

**効果**: `topology.Parse` が行う 2-pass resolution と同じ最終状態を、構築コストを払わずに直接組み立てる。

### P-PERF-4: アロケーション最小化

- スライスは `make([]T, 0, expectedSize)` で初期容量を予約 (`expectedSize` は draw 済みの長さ)
- 大きな string concatenation は `strings.Builder` を使う
- 中間構造体を作らず、最終 `*topology.Schema` に直接書き込む

ベンチマーク `BenchmarkValidSchemaDraw` (NFR-U7-6) で `b.ReportAllocs()` を有効にし、回帰を検知する。

### P-PERF-5: ベンチマーク粒度は top-level のみ (Q6=A)

`BenchmarkValidSchemaDraw` 1 件のみを初期スコープに含む。シナリオ別 (`MaxServices=1/10/100`) や generator 毎の細分化は **将来必要時に追加**。過剰先取りを避ける。

```go
func BenchmarkValidSchemaDraw(b *testing.B) {
    gen := generators.ValidSchema()
    b.ReportAllocs()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = gen.Example()  // rapid の Example() で 1 draw
    }
}
```

(`Example()` は `*rapid.T` 不要で 1 サンプルだけ取り出す API。bench に最適)

### P-PERF-6: メモリ検証は暗黙 (Q7=A)

NFR-U7-7 (≤ 1 MB / drawn schema) は **CI 自動チェックなし**。`BenchmarkValidSchemaDraw` の `b.ReportAllocs()` 出力を開発者が見て判断。明らかに退化したら `pprof` で深堀り。

---

## 2. Composition / Maintainability Patterns

### P-COMP-1: Atomic → Composed 階層

```text
Layer 0 (rapid 標準):  rapid.IntRange, rapid.StringMatching, rapid.SampledFrom, ...
Layer 1 (atomic):      ValidServiceID, ValidProbability, ValidLatencyPair, ValidServiceKind, ...
Layer 2 (entity):      ValidService, ValidOperation (初期スコープ外)
Layer 3 (root):        ValidSchema
```

Layer N の generator は **Layer < N の generator のみ** を内部で使用。逆方向の依存を作らない (保守性確保)。

### P-COMP-2: Functional options unexported struct パターン (Q2=A)

```go
package generators

// SchemaOption mutates schema-generation parameters.
type SchemaOption func(*schemaOptions)

// schemaOptions は unexported。利用者は Option 関数経由でのみ設定可能。
type schemaOptions struct {
    maxServices       int
    maxOpsPerService  int
    maxCallsPerOp     int
    maxFaults         int
    biasValid         float64  // for AnySchema
}

func defaultSchemaOptions() schemaOptions {
    return schemaOptions{
        maxServices:      10,
        maxOpsPerService: 5,
        maxCallsPerOp:    5,
        maxFaults:        3,
        biasValid:        0.5,
    }
}

// MaxServices caps the number of services in the generated schema.
func MaxServices(n int) SchemaOption {
    return func(o *schemaOptions) {
        if n < 1 { n = 1 }
        o.maxServices = n
    }
}
```

**パターンの約束事**:
- options struct は **unexported** (lib 利用者がフィールドに直接触れない)
- 各 Option 関数は **値域を内部で正規化** (例: 負数を 1 に clamp)
- `defaultXxxOptions()` の戻り値は **値 (ポインタでない)** で immutable のように扱う

### P-COMP-3: AnySchema は ValidSchema の出力を確率的に degrade する (Q4=A)

```go
func AnySchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema] {
    return rapid.Custom(func(t *rapid.T) *topology.Schema {
        o := defaultSchemaOptions()
        for _, opt := range opts { opt(&o) }
        schema := ValidSchema(opts...).Draw(t, "valid_base")
        // BiasValid 確率でそのまま返す
        if rapid.Float64Range(0, 1).Draw(t, "bias_roll") < o.biasValid {
            return schema
        }
        // 残りは 1 箇所だけ "壊す"
        mutateKind := rapid.SampledFrom([]mutator{
            unresolveEdgeTarget,     // R-STR-3 違反
            introduceCycle,          // R-STR-4 違反
            misreferenceJourney,     // R-STR-5 違反
            misreferenceFault,       // R-STR-6 違反
        }).Draw(t, "mutate_kind")
        return mutateKind(t, schema)
    })
}

type mutator func(t *rapid.T, s *topology.Schema) *topology.Schema
```

**効果**:
- `rapid.Filter` 不要 (NFR-U7-6)
- 各 mutator が責任を持つ違反パターンを明示
- 後で違反パターンを追加するときは mutator slice に append するだけ

---

## 3. API 拡張 / 互換性 Patterns

### P-API-1: SemVer pre-v1 break OK, post-v1 strict (Q9 of NFR-R + Q10=A)

- v1.0.0 リリース前は public API の破壊変更を許容
- v1.0.0 以降は **追加変更のみ許可**:
  - 新 generator の追加 → patch リリース OK
  - 新 Option 関数の追加 → patch リリース OK
  - 既存 generator の signature 変更 → major version up

### P-API-2: Deprecation は `// Deprecated:` コメントのみ (Q10=A)

```go
// Deprecated: use ValidSchemaV2 instead. Will be removed in v2.0.0.
func ValidSchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema] { ... }
```

`gorelease` / `apidiff` は U7 単体では使わない (Build and Test ステージで再評価)。

### P-API-3: Incremental 拡張時のチェックリスト (FD Q8=A の運用化)

各ユニット FD から U7 への追加リクエストが来たとき、CG 担当 (Codex / Cursor) は以下を確認:

- [ ] 既存 public 関数の signature を変えていない (追加のみ)
- [ ] 新規 Option 関数も `<package>.OptionType` を返す (既存 API への影響なし)
- [ ] 新規 generator は Valid 系と Any 系の **両方** を提供 (Q2 of FD)
- [ ] atomic → composed 階層を尊重 (P-COMP-1)
- [ ] realistic range をデフォルトに (Q6 of FD)
- [ ] GoDoc + Example function を追加 (P-DOC-1)

---

## 4. Documentation Patterns

### P-DOC-1: Top-level generator に Example function (Q9=A)

`ValidSchema`, `ValidService`, `AnySchema` などのトップレベル generator には Example function を 1 件ずつ:

```go
func ExampleValidSchema() {
    rapid.Check(&testing.T{}, func(t *rapid.T) {
        schema := generators.ValidSchema(
            generators.MaxServices(3),
            generators.MaxOpsPerService(2),
        ).Draw(t, "schema")
        if err := topology.Validate(schema); err != nil {
            t.Fatalf("invalid: %v", err)
        }
    })
    // Output: (none — rapid.Check has no stdout in success path)
}
```

(`Output:` が空または無しでも Go test は許容する — Example function はドキュメント目的のみ)

### P-DOC-2: Atomic generator は GoDoc 1-2 行 + 不変条件参照

```go
// ValidServiceID returns a generator producing a topology.ServiceID
// matching ^[a-z][a-z0-9-]{2,30}$. See business-rules.md §3.
func ValidServiceID() *rapid.Generator[topology.ServiceID] { ... }
```

Example function は **トップレベルのみ** (P-DOC-1)、atomic は GoDoc 文字列のみ (保守負担最小化)。

---

## 5. Concurrency / Safety Patterns

### P-CONC-1: 完全 thread-safe、グローバル状態なし (Q8 of NFR-R)

- パッケージレベルの mutable 変数を持たない
- `sync.Pool` は使わない (初期スコープ。性能問題が出たら NFR-Design で再評価)
- すべての generator が `*rapid.Generator[T]` を **値レシーバー** 同等で返す (rapid 内部のスレッド安全性に依存)

### P-CONC-2: テストは `t.Parallel()` (Q4 of NFR-R)

```go
func TestValidSchema_PassesValidate(t *testing.T) {
    t.Parallel()
    rapid.Check(t, func(t *rapid.T) {
        schema := generators.ValidSchema().Draw(t, "s")
        if err := topology.Validate(schema); err != nil {
            t.Fatalf("invalid: %v", err)
        }
    })
}
```

### P-CONC-3: Context / timeout は採用しない (Q8=A)

- rapid 自身に context-cancel 機構なし
- Go test の `-timeout` フラグで全体 budget を制御 (デフォルト 10 分、CI でも十分)
- 個別 deadline は導入しない

---

## 6. Pre-U1 Type Skeleton Pattern (Q5=A)

### P-SKEL-1: 正式に `topology/` パッケージとして書く

U7 Code Generation のフェーズは、実は **U1 の型定義を先に書く** ことを含む:

```text
U7 Code Generation Plan (Planning ステージで詳細確定):
  Phase 0: pre-U1 type skeleton
    - topology/doc.go          // パッケージ概要
    - topology/types.go        // Schema, Service, ServiceID, Operation, CallNode, Edge,
                               //   Journey, Step, FaultSpec, FaultTarget の構造体定義のみ
                               //   メソッド (Parse, Validate, MarshalYAML, Equal) は **未実装**
    - topology/enums.go        // ServiceKind, Protocol, ExhaustedAction, FaultKind の iota 型
  Phase 1: U7 testutil/generators 本体
    - testutil/generators/doc.go
    - testutil/generators/options.go
    - testutil/generators/primitives.go
    - testutil/generators/service.go
    - testutil/generators/schema.go
    - testutil/generators/*_test.go
    - testutil/generators/bench_test.go
```

### P-SKEL-2: 型骨格は Application Design `component-methods.md` に完全準拠

`component-methods.md` で定義された:
- フィールド名・型・順序
- 公開度 (大文字 / 小文字)
- struct タグ (yaml タグも、ただし `Marshal/Unmarshal` 実装は U1 で)

を **1 文字も外さず** 写経する。差分が出たら component-methods.md を更新するか、U7 CG で議論。

### P-SKEL-3: メソッド本体は panic スタブ

```go
// Parse decodes a topology YAML from r.
// NOTE: implementation in U1 Code Generation.
func Parse(r io.Reader) (*Schema, error) {
    panic("topology.Parse not yet implemented (U1)")
}

// Validate checks structural invariants of the Schema.
// NOTE: implementation in U1 Code Generation.
func Validate(s *Schema) error {
    panic("topology.Validate not yet implemented (U1)")
}
```

**例外**: `Equal` は U7 で必要 (PBT-02 ラウンドトリップ等価判定)。U7 で **最低限の identifier-based 等価実装** を行い、U1 で正式版に書き換える可能性を残す。これは U7 CG Planning で議論。

`panic` スタブは U7 のテストでは絶対に呼ばれない設計 (`Parse`/`Validate` は U7 内では使わない)。万一テストが panic したら設計バグ。

### P-SKEL-4: U1 への引き継ぎマーカー

U7 が書いた骨格ファイルには:

```go
// AUTOGEN-MARKER-U1: This file was scaffolded during U7 (testutil/generators)
// Code Generation. The U1 (topology) Code Generation must:
//   - Implement Parse, Validate, MarshalYAML, ApplyFaults, etc.
//   - Replace the panic stubs.
//   - Add unit + property-based tests.
// Once U1 is complete, this marker comment can be removed.
```

を冒頭に記載。U1 CG が見落とさないようにする。

---

## 7. パターン適用と NFR の対応表

| NFR | 関連パターン |
|---|---|
| NFR-U7-1 (rapid 採用) | P-PERF-1 |
| NFR-U7-2 (シード再現性) | (rapid デフォルトに任せる、特別なパターンなし) |
| NFR-U7-3 (テスト時間予算) | P-PERF-1, P-PERF-2, P-PERF-3, P-PERF-4 |
| NFR-U7-4 (t.Parallel) | P-CONC-2 |
| NFR-U7-5 (coverage 80%) | テスト網羅で達成 (logical-components.md §4 参照) |
| NFR-U7-6 (1ms/draw) | P-PERF-2, P-PERF-3, P-PERF-4, P-PERF-5 |
| NFR-U7-7 (≤ 1 MB) | P-PERF-4, P-PERF-6 |
| NFR-U7-8 (thread-safe) | P-CONC-1 |
| NFR-U7-9 (SemVer) | P-API-1, P-API-2 |
| NFR-U7-10 (incremental 拡張) | P-COMP-1, P-COMP-2, P-API-3, P-DOC-2 |
| (U7-U1 dependency) | P-SKEL-1, P-SKEL-2, P-SKEL-3, P-SKEL-4 |
