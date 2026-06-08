# U1 topology — Business Rules

本書は U1 の業務規則・検証規則・不変条件・Testable Properties を確定する。U7 の `business-rules.md` §5 で R-STR-1..8 を generator 視点で定義した。本書では同じ規則を **`Validate` の検証ロジック視点** で再定義し、加えてドメイン妥当性 (Q6=B) を追加する。

---

## 1. デフォルト値ルール (Q3=A)

`Parse` の Phase 2a で適用される省略時デフォルト:

| フィールド | YAML が省略 | Default 値 |
|---|---|---|
| `Service.Replicas` | `replicas: <none>` | `1` |
| `Service.Language` | 省略 | `""` (空文字) |
| `Service.Framework` | 省略 | `""` |
| `Service.Version` | 省略 | `""` |
| `Edge.ErrorRate` | 省略 | `0.0` |
| `Edge.Timeout` | 省略 | `0` (= 無制限の意味、Engine が解釈) |
| `Edge.Retries` | 省略 | `0` |
| `Edge.RetryBackoff` | 省略 / `""` | `BackoffExponential` |
| `LatencyDist.Distribution` | 省略 / `""` | `"constant"` |
| `LatencyDist.P50` | 省略 | `0` |
| `LatencyDist.P95` | 省略 | `P50` (= constant の時 p95==p50) |
| `Journey.Weight` | 省略 / `0` | `1.0` |
| `RecoveryPolicy.OnExhausted` | 省略 / `""` | `ExhaustedPropagate` |

これらは `Parse` 後の `*Schema` を見れば最終値が確認できる (Q3=A: Schema は explicit な状態)。

---

## 2. 未知フィールドの扱い (Q2=C)

- **Parse**: lax (未知キーを無視、Schema に取り込まない)
- **Lint**: `KnownFields(true)` 相当の strict 解析 + 未知キーを `LintIssue` (severity=Warning) として返す

警告対象は YAML キーレベルのみ。enum 値 (例: `kind: invalidkind`) は **Parse 時にエラー** (フォールバックで `KindApplication` にするなどはしない)。

---

## 3. Validate のチェックリスト (Q1=C, Q6=B)

Validate は **集約報告** (Q1=C): すべての違反を `errors.Join` で 1 つにまとめて返す。各エラーには **YAML パス** (例: `services.frontend.operations.GetProduct.calls[0]`) が含まれる。

### 3.1 構造的検証 (R-STR-1..8)

| Rule ID | チェック内容 | エラー例 |
|---|---|---|
| R-STR-1 | `s.Services[id].Name == id` (マップキーと Service.Name の整合) | `services.foo: name mismatch (key=foo, Service.Name=bar)` |
| R-STR-2 | 各 `Operation.Service == svc` (back-pointer 整合) | `services.foo.operations.X: Service back-pointer points to "bar", expected "foo"` |
| R-STR-3 | すべての `Edge.From` / `Edge.To` が `s.Services[...].Operations[...]` にある | (Parse の Phase 2b で解決失敗時に補足。Validate で再確認は不要だが念のため) |
| R-STR-4 | Operation グラフが DAG (循環なし) | `cycle detected in operation graph: services.A.X → services.B.Y → services.A.X` |
| R-STR-5 | すべての `Journey.Steps[i].Op` が schema 内の operation を指す | `journeys.checkout.steps[2].op: operation "Pay" on service "payment" not found` |
| R-STR-6 | `FaultSpec.Target` が schema 内の実体を指す (kind に応じて Service/Operation/Edge) | `faults[1].target: service "missing-svc" not found` |
| R-STR-7 | `CallNode` の `Edge` と `Parallel` は排他 (両方 non-nil または両方 nil はエラー) | (Parse の Phase 2b で検出。Validate では確認のみ) |
| R-STR-8 | `RecoveryPolicy.Fallback[i].From == 親 Edge.From` (fallback はオーナー operation 経由) | `services.foo.operations.X.calls[0].on_failure.fallback[0]: From mismatch — must be services.foo.operations.X` |

### 3.2 ドメイン妥当性 (Q6=B)

| 規則 | チェック | エラー例 |
|---|---|---|
| D-1 | `Service.Replicas >= 1` | `services.foo.replicas: must be >= 1, got 0` |
| D-2 | `Edge.ErrorRate ∈ [0.0, 1.0]` | `services.foo.operations.X.calls[0].error_rate: must be in [0,1], got 1.5` |
| D-3 | `Edge.Timeout >= 0` | `services.foo.operations.X.calls[0].timeout: must be >= 0, got -5s` |
| D-4 | `Edge.Retries >= 0` | `services.foo.operations.X.calls[0].retries: must be >= 0, got -1` |
| D-5 | `Edge.Latency.P95 >= Edge.Latency.P50` (両方 > 0 のとき) | `services.foo.operations.X.calls[0].latency: p95 (1ms) must be >= p50 (10ms)` |
| D-6 | `Edge.Latency.P50 >= 0` | `services.foo.operations.X.calls[0].latency.p50: must be >= 0, got -5ms` |
| D-7 | `LatencyDist.Distribution ∈ {"constant", "lognormal", "normal", "exponential"}` | `latency.distribution: unsupported "weibull"; allowed: constant/lognormal/normal/exponential` |
| D-8 | `FaultSpec.Severity.Probability ∈ [0.0, 1.0]` (該当する FaultKind のみ) | `faults[0].severity.probability: must be in [0,1], got 2.0` |
| D-9 | `FaultSpec.Severity.Multiplier > 0` (`FaultLatencyInflation` のとき) | `faults[0].severity.multiplier: must be > 0 for latency_inflation, got 0` |
| D-10 | `Journey.Weight > 0` | `journeys.checkout.weight: must be > 0, got 0` |
| D-11 | `Journey.Steps` は非空 | `journeys.checkout.steps: must contain at least one step` |
| D-12 | `Service.Operations` は非空 | `services.foo.operations: must contain at least one operation` |
| D-13 | `Schema.Services` は非空 | `services: must contain at least one service` |
| D-14 | `Schema.Journeys` は非空 | `journeys: must contain at least one journey` |

### 3.3 業務的妥当性 (Q6=B は採用、Q6=C は不採用)

例えば `kind: database` のサービスでも outgoing calls を持てる (例: DB から audit log への書き込みを表現したい場合)。本プロジェクトはこのような**業務的制約を強制しない**。利用者の自由度を残す。

---

## 4. Equal の比較規則 (Q5=A 確定の詳細)

### 4.1 順序維持を要求する箇所

| 構造 | 順序 |
|---|---|
| `Operation.Calls []*CallNode` | **順序維持** (シーケンス意味あり) |
| `CallNode.Parallel []*CallNode` | **順序維持** (将来の semantic 拡張で順序が意味を持つ可能性、保守的に保持) |
| `RecoveryPolicy.Fallback []*Edge` | **順序維持** (fallback chain は順番が意味を持つ) |
| `Journey.Steps []*Step` | **順序維持** (step は順次実行される) |
| `Schema.Faults []FaultSpec` | **順序維持** (faults は declaration 順を保持) |

### 4.2 順序を問わない箇所 (set equality)

| 構造 | 比較 |
|---|---|
| `Schema.Services map[ServiceID]*Service` | キー集合 + 各値 equal |
| `Service.Operations map[string]*Operation` | キー集合 + 各値 equal |
| `Schema.Journeys map[string]*Journey` | キー集合 + 各値 equal |

### 4.3 識別子ベース

ポインタ比較は **絶対にしない**。`*Operation` の同一性は `(Service.Name, Operation.Name)` ペアで判定 (`identifyOp` ヘルパー)。

---

## 5. Parse パフォーマンス目標 (Q11=A)

| 規模 | 目標 |
|---|---|
| 典型: 10 services, 30 operations, 50 edges, 5 journeys, 3 faults | `Parse` ≤ 10 ms |
| 大規模 (best-effort, 目標ではない): 100 services, 500 operations, 1000 edges | 監視対象だが具体目標値なし |

`BenchmarkParse` でゴール検証。CI ベンチでなく開発者がローカルで確認する形 (Q7 of U7 NFR-R と整合)。

---

## 6. Lint API 仕様

```go
type LintIssue struct {
    Path     string
    Severity LintSeverity
    Message  string
}

type LintSeverity int
const (
    LintError LintSeverity = iota
    LintWarning
)

func Lint(r io.Reader) ([]LintIssue, error)
```

`Lint` が返す `LintIssue`:

| Severity | 内容 |
|---|---|
| `LintError` | Validate が検出する全違反 (R-STR-1..8, D-1..D-14) |
| `LintWarning` | 未知 YAML キー、deprecated フィールド (将来使用)、効率的でない記述 (例: `parallel` ブロック内が 1 要素のみ) |

`Lint` の `error` 戻り値は **YAML 構文エラー** など Lint プロセス自体の失敗時のみ。違反は `[]LintIssue` で返す。

---

## 7. エラー型階層

```go
// ParseError は Parse / Lint で発生したエラーを表す。
type ParseError struct {
    Path    string  // YAML パス (e.g., "services.foo.operations[0].calls[2].to")
    Message string
    Inner   error
}

func (e *ParseError) Error() string {
    if e.Inner != nil {
        return fmt.Sprintf("topology: %s: %s: %v", e.Path, e.Message, e.Inner)
    }
    return fmt.Sprintf("topology: %s: %s", e.Path, e.Message)
}

func (e *ParseError) Unwrap() error { return e.Inner }

// ValidationError は Validate が返すエラー (R-STR-* / D-* 違反)
type ValidationError struct {
    Path    string
    Rule    string  // e.g., "R-STR-4", "D-2"
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("topology: %s: [%s] %s", e.Path, e.Rule, e.Message)
}
```

Parse の最終戻り値型:
- 単一 YAML syntax error → `*ParseError` (Phase 1)
- 複数の参照解決 / 検証エラー → `errors.Join(parseError1, parseError2, ...)` (Phase 2b / 3)

`errors.As` / `errors.Is` で個別エラーを抽出可能。

---

## 8. ApplyFaults の振る舞い詳細 (Q7=A, Q8=A)

### 入力

`Schema.Faults` (順序付き slice)。各要素の `Target` は Parse 時に解決済み。

### 出力

`*FaultOverlay` (内部 3 つの map):
- `map[*Service][]FaultSpec` (node target)
- `map[*Operation][]FaultSpec` (operation target)
- `map[*Edge][]FaultSpec` (edge target)

複数の FaultSpec が同じ target を指す場合は **slice に append**。順序は `Schema.Faults` の順序を維持。

### 公開 lookup API (Journey Engine 向け)

```go
func (o *FaultOverlay) NodeFaults(svc *Service) []FaultSpec
func (o *FaultOverlay) OperationFaults(op *Operation) []FaultSpec
func (o *FaultOverlay) EdgeFaults(e *Edge) []FaultSpec
```

各メソッドは O(1) (map lookup)。slice が空 / 存在しない場合は `nil` を返す (Go 慣習)。

### 何を pre-compute **しないか** (Q8=A 確定)

- **カスケード派生は計算しない**: Edge X→Y が `disconnect` fault でも、X の OnFailure に Y 以外への fallback があれば cascade しない可能性がある。判定は Journey Engine の実行時責務 (Application Design `services.md` 契約 O-4)
- **失敗確率の評価はしない**: `severity.probability` の評価も実行時 (`rapid.Float64Range` ベース)

### Idempotency (TP-U1-7、PBT-04 適用)

`ApplyFaults` は **副作用なし** で同じ `*Schema` から同じ `*FaultOverlay` を返す (`faultOverlayEqual` で比較可能):

```go
o1 := s.ApplyFaults()
o2 := s.ApplyFaults()
// faultOverlayEqual(o1, o2) == true (TP-U1-7)
```

---

## 9. JSON Schema Export 仕様 (Q9=A)

### Draft 2020-12 の構造

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/ymotongpoo/xk6-otel-gen/schemas/topology.schema.json",
  "title": "xk6-otel-gen topology schema",
  "type": "object",
  "required": ["services", "journeys"],
  "additionalProperties": true,
  "properties": {
    "services": { ... },
    "journeys": { ... },
    "faults":   { ... }
  },
  "$defs": {
    "Service":        { ... },
    "Operation":      { ... },
    "CallNode":       { ... },
    "Edge":           { ... },
    "RecoveryPolicy": { ... },
    "Journey":        { ... },
    "Step":           { ... },
    "FaultSpec":      { ... },
    "FaultTarget":    { "oneOf": [...] },
    "LatencyDist":    { ... },
    "SeverityParams": { ... }
  }
}
```

### 各 `$defs` エントリに含める

- `type`: object / array / string / number / integer / boolean
- `required`: 必須フィールド名
- `properties`: 各フィールドの再帰的 schema
- `enum`: enum 値の列挙 (例: `ServiceKind` は `["application", "database", "external_api", "cache", "queue"]`)
- `description`: フィールドの簡易説明
- `examples`: 1〜2 個の例値

### Schema が embed される場所

`topology/jsonschema/topology.schema.json` に静的ファイルとして配置。`//go:embed` でバイナリに同梱。`ExportJSONSchema()` はバイト列をそのまま返す。

---

## 10. Testable Properties (PBT-01、Q12=B)

Application Design `application-design.md` §6 で識別済みの 5 件 + 本 FD で追加する 3 件 = 計 8 件。

### TP-U1-1: Round-trip (PBT-02)

```text
For all valid Schema s drawn from generators.ValidSchema():
    topology.Equal(Parse(Marshal(s)), s) == true
```

実装:
```go
func TestParse_RoundTrip(t *testing.T) {
    t.Parallel()
    rapid.Check(t, func(t *rapid.T) {
        s := generators.ValidSchema().Draw(t, "schema")
        yamlBytes, err := yaml.Marshal(s)
        require.NoError(t, err)
        s2, err := topology.Parse(bytes.NewReader(yamlBytes))
        require.NoError(t, err)
        require.True(t, topology.Equal(s, s2),
            "round-trip should preserve schema; lost or altered fields")
    })
}
```

### TP-U1-2: Non-nil pointers after Parse

```text
For all valid yaml bytes produced from Schema:
    after s, _ := Parse(yaml):
        all *Service / *Operation / *Edge pointers in s are non-nil
```

### TP-U1-3: Map-key / Name consistency

```text
For all s drawn from ValidSchema():
    forall id in s.Services: s.Services[id].Name == id
    forall (svcID, op) in expanded ops: op.Service.Name == svcID && op.Service.Operations[op.Name] == op
```

### TP-U1-4: DAG after Validate

```text
For all s where Validate(s) == nil:
    operation graph is acyclic (Kahn's topological sort succeeds)
```

### TP-U1-5: ApplyFaults consistency

```text
For all s drawn from ValidSchema():
    overlay := s.ApplyFaults()
    forall f in s.Faults:
        f.Target is mapped in overlay's appropriate map
    sum of all len(overlay.{Node,Operation,Edge}Faults(...)) == len(s.Faults)
```

### TP-U1-6 (NEW, PBT-04 Idempotency — Validate)

```text
For all s drawn from ValidSchema():
    err1 := Validate(s)
    err2 := Validate(s)
    (err1 == nil) == (err2 == nil)
    // valid なら両方 nil、invalid なら両方非 nil (エラー集合の identity は保証しないが、状態は同じ)
```

### TP-U1-7 (NEW, PBT-04 Idempotency — ApplyFaults)

```text
For all s drawn from ValidSchema():
    o1 := s.ApplyFaults()
    o2 := s.ApplyFaults()
    faultOverlayEqual(o1, o2) == true
```

`faultOverlayEqual` ヘルパーは U1 内で定義 (3 つの map の同等性チェック)。

### TP-U1-8 (NEW, Round-trip — JSON Schema)

```text
For all s drawn from ValidSchema():
    schema_json := s.ExportJSONSchema()
    yaml_bytes  := yaml.Marshal(s)
    json_bytes  := yamlToJSON(yaml_bytes)
    validator   := jsonschema.MustCompileBytes("topology.json", schema_json)
    validator.Validate(json_bytes) == nil
```

依存: `github.com/santhosh-tekuri/jsonschema/v5` (テスト依存のみ、Go module の indirect dependency として `go.mod` の `require` セクションに追加)。**AGENTS.md §2 の「追加依存禁止」を本 FD で 1 件緩和**: テスト専用、本体コードに影響なし。

### 各 TP のテストファイル配置

| TP | テストファイル | テスト関数 |
|---|---|---|
| TP-U1-1 | `topology/parse_roundtrip_test.go` | `TestParse_RoundTrip` |
| TP-U1-2 | `topology/parse_pointers_test.go` | `TestParse_NoNilPointers` |
| TP-U1-3 | `topology/parse_consistency_test.go` | `TestParse_MapKeyConsistency` |
| TP-U1-4 | `topology/validate_dag_test.go` | `TestValidate_AlwaysDAG` |
| TP-U1-5 | `topology/applyfaults_test.go` | `TestApplyFaults_OverlayCovers` |
| TP-U1-6 | `topology/validate_idempotent_test.go` | `TestValidate_Idempotent` |
| TP-U1-7 | `topology/applyfaults_test.go` | `TestApplyFaults_Idempotent` |
| TP-U1-8 | `topology/jsonschema_roundtrip_test.go` | `TestExportJSONSchema_RoundTrip` |

加えて U7 の `TestValidSchema_ValidatePlaceholder` (`t.Skip` 状態) を **本 unit で `t.Skip` を外す** ことで TP-U7-1 を有効化。

---

## 11. 性能 / 並行性

- Parse は **idempotent + thread-safe**: 同じ io.Reader (繰り返し読めるもの) から同じ結果。グローバル状態なし。
- 返した `*Schema` は **read-only として扱う**: 利用側で mutate しないこと。これは convention としてのみ要求 (Go の immutability 機構なし)。Journey Engine 側も Schema を変更しない。
- `FaultOverlay` も read-only。
- 並行 Parse / Validate / ApplyFaults は OK (内部状態がない)。

---

## 12. デフォルトレンジに関する PBT-07 整合性

U7 の `business-rules.md` §3 で定めた generator のレンジは、本 U1 の Validate のドメイン妥当性 (D-1..D-14) と **完全に互換**。

すなわち:
- `generators.ValidSchema()` の出力は常に `Validate` を pass する (TP-U1-3 の前提)
- `generators.AnySchema()` の出力の一部は `Validate` でエラーになる (R-STR-* の違反を mutator が注入する設計)

D-1..D-14 のドメイン違反を `AnySchema` で生成する mutator が必要かは、本 FD では追加要求しない (R-STR-* 違反だけで十分なカバレッジが見込める)。後で必要になれば U7 への追加リクエストとして発行。
