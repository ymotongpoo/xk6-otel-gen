# U1 topology — Domain Entities & Method Contracts

本書は U1 の public API (型 + 関数 + メソッド) の **contract (前提・事後条件・エラー)** を確定する。型定義そのものは U7 で scaffold 済み (`topology/types.go`, `topology/enums.go`) — 本書では各エンティティの **業務的意味** とメソッドの **契約** を記述する。

末尾に **U7 への generator 追加リクエスト** セクション (Q13=A) を含む。

---

## 1. ドメインエンティティ

各型の意味と不変条件 (Parse 後の状態前提)。

### 1.1 `ServiceID` (newtype)

```go
type ServiceID string
```

- **意味**: サービスを一意に識別する名前。`Schema.Services` のマップキー
- **不変条件**: Parse 後、`s.Services[id].Name == id` (R-STR-1 / D-1 検証対象)
- **将来拡張**: 将来 multi-file をサポートする場合、`<namespace>/<name>` 形式へ拡張可能

### 1.2 `Schema`

```go
type Schema struct {
    Services map[ServiceID]*Service
    Journeys map[string]*Journey
    Faults   []FaultSpec
}
```

- **意味**: 1 つの topology YAML ファイル全体を表すルート
- **不変条件 (Parse 後 + Validate=nil)**:
  - `len(Services) >= 1` (D-13)
  - `len(Journeys) >= 1` (D-14)
  - すべての cross-reference は解決済みポインタ
  - Operation グラフは DAG (R-STR-4)

### 1.3 `Service`

```go
type Service struct {
    Name       ServiceID
    Kind       ServiceKind
    Replicas   int
    Language   string
    Framework  string
    Version    string
    Operations map[string]*Operation
}
```

- **意味**: 1 つの仮想サービス (アプリケーション、DB、外部 API 等)
- **不変条件**:
  - `Name` はゼロ値でない、`[a-z][a-z0-9-]{2,30}` 形式 (generator 規約準拠)
  - `Kind` は valid enum
  - `Replicas >= 1` (D-1)
  - `len(Operations) >= 1` (D-12)
  - 各 `op.Service == this` (R-STR-2)

### 1.4 `Operation`

```go
type Operation struct {
    Name    string
    Service *Service
    Calls   []*CallNode
}
```

- **意味**: サービスの呼び出し単位 (HTTP endpoint / RPC method / message topic)
- **不変条件**:
  - `Name` は非空 ASCII/UTF-8 文字列、≤ 120 文字
  - `Service` is non-nil、`Service.Operations[op.Name] == this` (R-STR-2)
  - `Calls` は nil-OK (leaf operation)、各要素は valid CallNode

### 1.5 `CallNode` (variant)

```go
type CallNode struct {
    Edge     *Edge
    Parallel []*CallNode
}
```

- **意味**: Operation の `Calls` 配列の要素。**Edge と Parallel は排他** (variant)
- **不変条件 (R-STR-7)**: ちょうど 1 つが non-nil
  - `Edge != nil && Parallel == nil` → 単一呼び出し
  - `Edge == nil && Parallel != nil && len(Parallel) >= 1` → 並列グループ

### 1.6 `Edge`

```go
type Edge struct {
    From         *Operation
    To           *Operation
    Protocol     Protocol
    Latency      LatencyDist
    ErrorRate    float64
    Timeout      time.Duration
    Retries      int
    RetryBackoff BackoffPolicy
    OnFailure    *RecoveryPolicy
}
```

- **意味**: Operation 間の有向呼び出し
- **不変条件**:
  - `From`, `To` is non-nil (R-STR-3)
  - `ErrorRate ∈ [0,1]` (D-2)
  - `Timeout >= 0` (D-3)
  - `Retries >= 0` (D-4)
  - `Latency.P95 >= Latency.P50 >= 0` (D-5 / D-6)
  - `OnFailure != nil` の場合、Fallback の所有関係が R-STR-8 を満たす

### 1.7 `RecoveryPolicy`

```go
type RecoveryPolicy struct {
    Fallback        []*Edge
    OnExhausted     ExhaustedAction
    DefaultResponse map[string]any
}
```

- **意味**: Edge 失敗時のリカバリーフロー (cache-aside / circuit-breaker 等)
- **不変条件**:
  - 各 `Fallback[i].From == 親 Edge.From` (R-STR-8)
  - `OnExhausted` は valid enum
  - `OnExhausted == ExhaustedReturnDefault` のとき `DefaultResponse` は non-nil (Recommended、強制せず)

### 1.8 `Journey`

```go
type Journey struct {
    Name   string
    Steps  []*Step
    Weight float64
}
```

- **意味**: Critical User Journey (1 つの user action = 1 trace の起点シーケンス)
- **不変条件**:
  - `Name` 非空、`Schema.Journeys` のマップキーと一致
  - `len(Steps) >= 1` (D-11)
  - `Weight > 0` (D-10)

### 1.9 `Step`

```go
type Step struct {
    Op       *Operation
    Parallel []*Step
}
```

- **意味**: Journey の一要素 (entry operation を起動するか、journey-level fan-out グループ)
- **不変条件**: `Op != nil || (Parallel != nil && len(Parallel) >= 1)` (CallNode と同じ variant ルール)

### 1.10 `FaultTarget`

```go
type FaultTarget struct {
    Kind      TargetKind
    Service   *Service
    Operation *Operation
    Edge      *Edge
}
```

- **意味**: fault の対象 (Service / Operation / Edge のいずれか)
- **不変条件 (R-STR-6)**:
  - `Kind == TargetNode` → `Service != nil`, 他 nil
  - `Kind == TargetOperation` → `Operation != nil`, 他 nil
  - `Kind == TargetEdge` → `Edge != nil`, 他 nil

### 1.11 `FaultSpec`

```go
type FaultSpec struct {
    Target   FaultTarget
    Kind     FaultKind
    Severity SeverityParams
}
```

- **意味**: 1 つの障害宣言 (どこに、どんな種類の、どの程度の障害を注入するか)
- **不変条件**:
  - `Target` は valid (上記)
  - `Kind` は valid enum
  - `Severity.Probability ∈ [0,1]` (D-8)
  - `FaultLatencyInflation` のとき `Severity.Multiplier > 0` (D-9)

### 1.12 `FaultOverlay` (opaque)

```go
type FaultOverlay struct {
    nodeFaults      map[*Service][]FaultSpec
    operationFaults map[*Operation][]FaultSpec
    edgeFaults      map[*Edge][]FaultSpec
}

func (o *FaultOverlay) NodeFaults(svc *Service) []FaultSpec
func (o *FaultOverlay) OperationFaults(op *Operation) []FaultSpec
func (o *FaultOverlay) EdgeFaults(e *Edge) []FaultSpec
```

- **意味**: Journey Engine が実行時に「このノード/operation/edge に fault があるか」を O(1) で lookup するインデックス
- **不変条件**: ApplyFaults の純粋関数的結果。Schema の Faults スライスを 3 種の map に分配しただけ

---

## 2. メソッド/関数 Contracts

### 2.1 `func Parse(r io.Reader) (*Schema, error)`

| 項目 | 内容 |
|---|---|
| 引数 | `r`: トポロジー YAML のリーダー。non-nil 必須 (nil 渡しは panic 想定外、`io.ReadAll` の動作に依存) |
| 戻り値 | 成功時: `*Schema` (Parse + Resolution + Validate がすべて成功), `nil` error / 失敗時: `nil` Schema + error (`*ParseError` or `errors.Join`) |
| 副作用 | なし (内部状態なし) |
| Idempotent | はい (同じ Reader 内容なら同じ結果) |
| Thread-safe | はい (グローバル状態なし) |
| エラーパターン | (1) YAML 構文エラー → 単一 `*ParseError` / (2) 参照解決失敗 → `errors.Join(...)` / (3) Validate 違反 → `errors.Join(...)`、`*ValidationError` を含む |

### 2.2 `func ParseFile(path string) (*Schema, error)`

| 項目 | 内容 |
|---|---|
| 引数 | `path`: ファイルパス。空文字や存在しないパスはエラー |
| 戻り値 | `Parse` と同じ |
| エラーパターン | ファイル open 失敗 → `fmt.Errorf("topology: open %s: %w", path, err)` (`*os.PathError` を wrap) + Parse のエラー |

### 2.3 `func Validate(s *Schema) error`

| 項目 | 内容 |
|---|---|
| 引数 | `s`: 非 nil 必須 (nil は panic) |
| 戻り値 | 違反なし: `nil` / 違反あり: `errors.Join(...)` (各エラーは `*ValidationError`) |
| 副作用 | なし (s を変更しない) |
| Idempotent | はい (TP-U1-6) |
| Thread-safe | はい (s を read-only として扱う限り) |
| 適用ルール | R-STR-1..8 + D-1..D-14 (`business-rules.md` §3) |

### 2.4 `func Equal(a, b *Schema) bool`

| 項目 | 内容 |
|---|---|
| 引数 | `a`, `b`: 両方 nil OK (両 nil なら true) |
| 戻り値 | 識別子ベース deep equality (`business-rules.md` §4) |
| 副作用 | なし |
| 性質 | 反射律 (`Equal(a, a) == true`)、対称律 (`Equal(a, b) == Equal(b, a)`)、推移律 |
| 注意 | `reflect.DeepEqual` を使わない (循環ポインタ + map iteration 順序の問題)。手動実装 |

### 2.5 `(*Schema).MarshalYAML() (any, error)`

| 項目 | 内容 |
|---|---|
| 引数 | レシーバ: 非 nil 必須 |
| 戻り値 | `*rawSchema` (yaml.v3 の Marshal 候補オブジェクト) + nil error |
| 副作用 | なし |
| 並び順 | services / operations / journeys は名前昇順、faults / calls / fallback / steps は登場順 (Q4=A) |
| 用途 | `yaml.Marshal(schema)` から自動呼び出しされる (yaml.v3 の Marshaler interface) |
| ラウンドトリップ | `Equal(Parse(Marshal(s)), s) == true` for valid s (TP-U1-1) |

### 2.6 `(*Schema).ApplyFaults() *FaultOverlay`

| 項目 | 内容 |
|---|---|
| 引数 | レシーバ: 非 nil 必須、Validate 後の Schema 推奨 (invalid Schema でも動くが結果に意味なし) |
| 戻り値 | `*FaultOverlay` (always non-nil、空 Schema でも空 Overlay を返す) |
| 副作用 | なし |
| 性能 | O(len(Schema.Faults)) |
| Idempotent | はい (TP-U1-7) |
| カスケード | pre-compute しない (Q8=A) |

### 2.7 `(*Schema).ExportJSONSchema() ([]byte, error)`

| 項目 | 内容 |
|---|---|
| 引数 | レシーバ: 非 nil (実体は s に依存しない — 静的テンプレート) |
| 戻り値 | JSON Schema Draft 2020-12 形式の `[]byte` + nil error (テンプレートは事前検証済み) |
| 副作用 | なし |
| 用途 | エディタ補完 / lint ツール用 (NFR-6.2) |
| 実装 | `//go:embed topology/jsonschema/topology.schema.json` |

### 2.8 `(*Schema).FindServiceByName(id ServiceID) (*Service, bool)`

| 項目 | 内容 |
|---|---|
| 引数 | `id`: ServiceID |
| 戻り値 | `*Service`, ok (`s.Services[id]` の素直なラッパ) |
| 副作用 | なし |

### 2.9 `(*Schema).JourneyNames() []string`

| 項目 | 内容 |
|---|---|
| 戻り値 | journey 名のソート済み slice (アルファベット昇順) |
| 副作用 | なし、毎回新しい slice を作る |

### 2.10 `func Lint(r io.Reader) ([]LintIssue, error)`

| 項目 | 内容 |
|---|---|
| 引数 | `r`: トポロジー YAML リーダー |
| 戻り値 | `[]LintIssue` (空でも nil でない) + error (Lint プロセス自体の失敗時のみ、YAML 違反は `LintIssue` で返す) |
| 副作用 | なし |
| 用途 | CLI ツール (`cmd/xk6-otel-gen-schema/`) と editor integration |

---

## 3. パッケージレイアウト

```text
topology/
├── doc.go                       # U7 が scaffold 済み (U1 で内容更新)
├── enums.go                     # U7 が scaffold 済み (変更なし)
├── types.go                     # U7 が scaffold 済み (変更なし)
├── stubs.go                     # U7 が scaffold した panic stubs — U1 で削除して実装に置き換え
├── raw.go                       # NEW: rawSchema, rawService, rawOperation, etc. (Parse 内部型)
├── parse.go                     # NEW: Parse, ParseFile, decodeRaw, buildSchema, resolveReferences
├── validate.go                  # NEW: Validate + validateXxx ヘルパー群
├── marshal.go                   # NEW: (*Schema).MarshalYAML
├── equal.go                     # NEW: Equal + equalXxx ヘルパー
├── faults.go                    # NEW: (*Schema).ApplyFaults, FaultOverlay methods
├── jsonschema.go                # NEW: (*Schema).ExportJSONSchema (with //go:embed)
├── jsonschema/
│   └── topology.schema.json     # NEW: JSON Schema Draft 2020-12 テンプレート
├── lint.go                      # NEW: Lint, LintIssue, LintSeverity
├── errors.go                    # NEW: ParseError, ValidationError
├── doc_test.go                  # NEW: Example functions
├── parse_test.go                # NEW: example-based tests for Parse
├── parse_roundtrip_test.go      # NEW: TP-U1-1
├── parse_pointers_test.go       # NEW: TP-U1-2
├── parse_consistency_test.go    # NEW: TP-U1-3
├── validate_dag_test.go         # NEW: TP-U1-4
├── validate_idempotent_test.go  # NEW: TP-U1-6
├── validate_test.go             # NEW: example-based tests for Validate rules
├── applyfaults_test.go          # NEW: TP-U1-5 + TP-U1-7
├── jsonschema_roundtrip_test.go # NEW: TP-U1-8
├── marshal_test.go              # NEW: example-based for MarshalYAML
├── equal_test.go                # NEW: example-based for Equal
└── bench_test.go                # NEW: BenchmarkParse
```

---

## 4. 依存追加

`go.mod` に追加する依存:

| モジュール | 用途 | テスト依存? |
|---|---|---|
| `gopkg.in/yaml.v3` | YAML パース・Marshal | 本体 (Parse / MarshalYAML) |
| `github.com/santhosh-tekuri/jsonschema/v5` | JSON Schema 検証 | **テスト依存のみ** (TP-U1-8) |

`yaml.v3` は **AGENTS.md §2 の許可リスト** に既載 (U7 でも `topology/types.go` の yaml タグで使用)。

`jsonschema/v5` は本 FD で **新規追加**。AGENTS.md §2 で「依存追加は rapid + yaml.v3 のみ」と縛っていたが、本 FD で **テスト依存に限り 1 件追加** する。これは U1 Code Generation Planning で再度明示し、AGENTS.md には反映しない (テスト依存はクリーンビルドに影響しないため)。

---

## 5. エラー型階層 (再掲、`business-rules.md` §7 とリンク)

```text
error
 ├── *ParseError       (path + message + inner err)
 │     └── (Unwrap to underlying yaml.v3 / os.PathError)
 └── *ValidationError  (path + rule ID + message)

errors.Join(...) で複数集約。errors.As/Is でフィルタリング可能。
```

---

## 6. U7 への generator 追加リクエスト (Q13=A)

本セクションは **U7 の `domain-entities.md` §8 (U7 に必要な追加ジェネレータ集約場所)** へ追記される項目を記述する。U1 Code Generation 実行時に U7 の `code-generation-plan.md` を拡張する形で、以下の generator を追加実装する。

### Request from U1 FD

U1 のテストで必要となる generator (Valid 系 + Any 系のペア):

| Generator | 概要 | 利用される TP |
|---|---|---|
| `ValidOperation` / `AnyOperation` | 単独の `*topology.Operation` を生成 (`Service` back-pointer は test-side で設定するパターン) | TP-U1-3 など |
| `ValidEdge` / `AnyEdge` | 単独の `*topology.Edge` を生成 (From/To の 2 つの `*Operation` を要求する constructor option) | TP-U1-1 round-trip 内部 |
| `ValidCallNode` / `AnyCallNode` | variant (Edge or Parallel)、Parallel の場合は再帰的に CallNode を生成 | TP-U1-1 |
| `ValidRecoveryPolicy` / `AnyRecoveryPolicy` | fallback chain (1〜3 段)、on_exhausted の各 enum 値 | TP-U1-1, TP-U1-3 |
| `ValidJourney` / `AnyJourney` | journey 全体 (entry + steps + weight) | TP-U1-1, TP-U1-2 |
| `ValidStep` / `AnyStep` | step 単独 (variant: Op or Parallel) | TP-U1-1 |
| `ValidFaultSpec` / `AnyFaultSpec` | 単独の FaultSpec (3 種の Target をランダム) | TP-U1-5 |
| `ValidFaultTarget` / `AnyFaultTarget` | TargetKind ごとの分岐生成 | TP-U1-5 |
| `ValidFaultOverlay` / `AnyFaultOverlay` | `*FaultOverlay` (内部 map を直接組み立て) | TP-U1-7 |

**合計**: 9 ペア × 2 = **18 関数**。

### 各 generator の不変条件 (Valid 系)

- 既存の `business-rules.md` §3-5 (U7 側) の R-DOM-* / R-STR-* を踏襲
- 各 generator は **standalone な valid** であること: 例えば `ValidEdge(from, to)` は与えられた 2 つの `*Operation` 間の valid な Edge を 1 つ生成し、その結果は `topology.Validate` が個別の Edge レベルで合理的とみなす状態

### 各 generator のオプション

- `ValidEdge(from, to *topology.Operation, opts ...EdgeOption)` 形式
- オプション例: `WithProtocol(p)`, `WithLatency(p50, p95)`, `WithErrorRate(r)`, `WithOnFailure(rp)`
- Functional options パターン (P-COMP-2 を踏襲)

### 実装スケジュール

これらの追加は **U1 Code Generation Planning** の Phase ヘッダで、`testutil/generators/` への追記項目として列挙される。U1 の本実装と並走して U7 を拡張する形 (Q8 of U7 FD の incremental 拡張プロセス)。

---

## 7. Out of Scope (U1 では扱わない)

- **`MarshalJSON`**: YAML のみ対応。JSON への直接シリアライズは将来 (`U8` か別ユニット) で必要なら検討
- **partial update**: `Schema` を mutate して再 Validate する API (`Schema.Update(...)`) は本 FD では設計しない。実装側で必要になれば後追い
- **schema migration**: トポロジー YAML のバージョン互換性管理 (v1 → v2) は本 MVP では扱わない
- **Mermaid 入力**: 要件で見送り (requirements.md FR-2.1)、`Parse` の入り口を `io.Reader` 抽象化に保つことで将来追加可能
- **Schema diff**: 2 つの Schema の差分計算 API は本 FD では設計しない (debug 用に必要なら別途)
