# U1 topology — NFR Design Patterns

本書は U1 の **「どう実装するか」のパターン** を確定する。NFR-R で識別した NFR-U1-1〜10 を達成する具体的手段を、コードレベルで記述する。

---

## 1. Performance Patterns

### P-PERF-1: 共通 decode 関数 (Q1=A)

`Parse` (lax) と `Lint` (strict) で YAML decode 挙動を切り替えるが、本体は 1 つの内部関数:

```go
// decodeRaw is the shared YAML→rawSchema decoder. Used by Parse (strict=false)
// and Lint (strict=true).
func decodeRaw(r io.Reader, strict bool) (*rawSchema, error) {
    dec := yaml.NewDecoder(r)
    dec.KnownFields(strict)
    var raw rawSchema
    if err := dec.Decode(&raw); err != nil {
        return nil, &ParseError{
            Path:    "<root>",
            Message: "yaml decode failed",
            Inner:   err,
        }
    }
    return &raw, nil
}
```

利点:
- コード重複なし
- 両モードでバグが同じ振る舞いで再現
- 将来 `decodeOption` 関数を追加するなら、`bool` を可変引数 options に置換可能 (後方互換)

### P-PERF-2: アロケーション最小化 (Q7=A)

「必要最小限のみ」の方針で以下を遵守:

```go
// Map の初期容量予約
schema.Services = make(map[ServiceID]*Service, len(raw.Services))
svc.Operations = make(map[string]*Operation, len(rs.Operations))

// Slice の append には cap ヒント
op.Calls = make([]*CallNode, 0, len(ro.Calls))
errs := make([]error, 0, 8) // 典型的なエラー数の見積もり

// 文字列連結は strings.Builder
var sb strings.Builder
sb.WriteString("services.")
sb.WriteString(string(svcName))
sb.WriteString(".operations.")
sb.WriteString(opName)
path := sb.String()
```

`sync.Pool` での `rawSchema` 再利用は **採用しない** (Q7=A、Parse は init time の 1 回呼び出しなので必要性低い)。

### P-PERF-3: ベンチマーク 1 種類 (Q10=A)

`BenchmarkParse` は **典型 YAML 1 種類のみ** (10 svc / 30 op / 50 edges):

```go
func BenchmarkParse(b *testing.B) {
    yamlBytes := mustReadFile("testdata/typical.yaml")
    b.ReportAllocs()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        if _, err := Parse(bytes.NewReader(yamlBytes)); err != nil {
            b.Fatal(err)
        }
    }
}
```

`testdata/typical.yaml` は手書き fixture (Q9: complex fixture は testdata に、minimal は inline で扱うが、bench fixture は **複雑** なので testdata 側に置く判断)。

### P-PERF-4: io.ReadAll で全読み込み (NFR-R Q10=A)

```go
func Parse(r io.Reader) (*Schema, error) {
    data, err := io.ReadAll(r)
    if err != nil {
        return nil, fmt.Errorf("topology: read input: %w", err)
    }
    raw, err := decodeRaw(bytes.NewReader(data), false)
    if err != nil {
        return nil, err
    }
    // ...
}
```

ストリーミングは採用しない (典型 YAML は数十 KB なので impact なし)。

### P-PERF-5: Validate の早期構造チェック (Q5=A)

```go
func Validate(s *Schema) error {
    var errs []error

    // Phase A: 構造的検証 (R-STR-1..8) — 構造エラーが多いとドメイン check が無意味
    errs = append(errs, validateMapKeyConsistency(s)...)      // R-STR-1
    errs = append(errs, validateBackPointers(s)...)           // R-STR-2
    errs = append(errs, validateNoOrphanReferences(s)...)     // R-STR-3
    errs = append(errs, validateDAG(s)...)                    // R-STR-4
    errs = append(errs, validateJourneyReachability(s)...)    // R-STR-5
    errs = append(errs, validateFaultTargets(s)...)           // R-STR-6
    errs = append(errs, validateCallNodeVariants(s)...)       // R-STR-7
    errs = append(errs, validateRecoveryPolicyOwnership(s)...)// R-STR-8

    // Phase B: ドメイン検証 (D-1..D-14) — 構造が壊れていても意味のあるエラーが返せる
    errs = append(errs, validateDomainRanges(s)...)

    return errors.Join(errs...)
}
```

固定順序: R-STR-1..8 → D-1..D-14。Validate 全体としてはすべての違反を集める (`errors.Join`)。

---

## 2. Error Aggregation Patterns

### P-ERR-1: 個別エラー型 (Q2=A)

```go
// errors.go (NEW)

// ParseError は Parse / Lint で発生したエラー (YAML 構文 + 参照解決).
type ParseError struct {
    Path    string // 例: "services.foo.operations.X.calls[0].to"
    Message string
    Inner   error  // 内部エラー (yaml.v3 など)、optional
}

func (e *ParseError) Error() string {
    if e.Inner != nil {
        return fmt.Sprintf("topology: %s: %s: %v", e.Path, e.Message, e.Inner)
    }
    return fmt.Sprintf("topology: %s: %s", e.Path, e.Message)
}

func (e *ParseError) Unwrap() error { return e.Inner }

// ValidationError は Validate が返す違反 (R-STR-* / D-*).
type ValidationError struct {
    Path    string
    Rule    string // 例: "R-STR-4", "D-2"
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("topology: %s: [%s] %s", e.Path, e.Rule, e.Message)
}
```

利用側は `errors.As(err, &validationErr)` で型を見て分岐可能:

```go
var pe *topology.ParseError
var ve *topology.ValidationError
if errors.As(err, &pe) {
    log.Printf("yaml issue at %s", pe.Path)
}
if errors.As(err, &ve) {
    log.Printf("rule %s violated at %s", ve.Rule, ve.Path)
}
```

### P-ERR-2: errors.Join の段階的累積

```go
func resolveReferences(schema *Schema, raw *rawSchema) error {
    var errs []error
    // ... loop, append errors as &ParseError{...}
    return errors.Join(errs...)
}

func Parse(r io.Reader) (*Schema, error) {
    raw, err := decodeRaw(r, false) // Phase 1: fail-fast
    if err != nil { return nil, err }
    schema := buildSchema(raw)
    if err := resolveReferences(schema, raw); err != nil { // Phase 2b: aggregated
        return nil, err
    }
    if err := Validate(schema); err != nil { // Phase 3: aggregated
        return nil, err
    }
    return schema, nil
}
```

Phase 2b と Phase 3 を **別々に** errors.Join する (両者が連続的に失敗しても、Phase 2b で止める。Phase 2b が成功してから Phase 3 を走らせる)。理由: Phase 2b の参照解決失敗時、Phase 3 の検証は意味をなさない (`*Operation` が nil の状態で R-STR-2 を検証しても無価値)。

### P-ERR-3: パス情報の組み立て

`path` 文字列は **YAML 階層を `.` 区切りで** 表現:
- `services.frontend`
- `services.frontend.operations.GET /products/{id}`
- `services.frontend.operations.GET /products/{id}.calls[0]`
- `services.frontend.operations.GET /products/{id}.calls[0].on_failure.fallback[0]`
- `journeys.checkout.steps[2]`
- `faults[0].target`

各 helper 関数が `path` を受け取り、エラーに含める:

```go
func resolveCallNode(schema *Schema, owningSvc *Service, owningOp *Operation, rc *rawCallNode, path string) (*CallNode, error) {
    // path = "services.<svc>.operations.<op>.calls[<i>]"
    if hasTo && hasParallel {
        return nil, &ParseError{
            Path:    path,
            Message: "exactly one of 'to' or 'parallel' is required (R-STR-7)",
        }
    }
    // ...
}
```

---

## 3. Marshal Patterns

### P-MARSHAL-1: rawSchema 経由の単一 Marshaler (Q3=A)

`*Schema` のみ `MarshalYAML` interface を実装、内部で `*rawSchema` を組み立てて返す:

```go
// marshal.go (NEW)

func (s *Schema) MarshalYAML() (any, error) {
    raw := &rawSchema{
        Services: make(map[string]*rawService, len(s.Services)),
        Journeys: make(map[string]*rawJourney, len(s.Journeys)),
        Faults:   make([]*rawFault, 0, len(s.Faults)),
    }
    // Q4=A: services を ServiceID 昇順で
    for _, id := range sortedServiceIDs(s.Services) {
        raw.Services[string(id)] = marshalService(s.Services[id])
    }
    // journeys を名前昇順で
    for _, name := range sortedKeys(s.Journeys) {
        raw.Journeys[name] = marshalJourney(s.Journeys[name])
    }
    // faults は登場順を維持
    for _, f := range s.Faults {
        raw.Faults = append(raw.Faults, marshalFault(f))
    }
    return raw, nil
}

// marshalService は *Service を *rawService に変換。
// Operations は名前昇順、Calls は登場順。
func marshalService(svc *Service) *rawService {
    return &rawService{
        Kind:       svc.Kind.String(),
        Replicas:   ptrInt(svc.Replicas),
        Language:   svc.Language,
        Framework:  svc.Framework,
        Version:    svc.Version,
        Operations: marshalOperations(svc.Operations),
    }
}

func marshalOperations(ops map[string]*Operation) []*rawOperation {
    names := sortedKeys(ops)
    out := make([]*rawOperation, 0, len(ops))
    for _, name := range names {
        out = append(out, &rawOperation{
            Name:  name,
            Calls: marshalCallNodes(ops[name].Calls),
        })
    }
    return out
}

func marshalCallNodes(nodes []*CallNode) []*rawCallNode {
    out := make([]*rawCallNode, 0, len(nodes))
    for _, n := range nodes {
        out = append(out, marshalCallNode(n))
    }
    return out
}

func marshalCallNode(n *CallNode) *rawCallNode {
    if n.Edge != nil {
        return marshalEdge(n.Edge)
    }
    // Parallel グループ
    return &rawCallNode{
        Parallel: marshalCallNodes(n.Parallel),
    }
}
```

yaml.v3 は `MarshalYAML` の戻り値を再帰的に encode するため、`rawCallNode` 等の内部型は yaml タグで自然に encode される。各型 (Service, Operation, Edge, ...) に MarshalYAML を実装する必要なし。

### P-MARSHAL-2: ServiceID 昇順 helper

```go
func sortedServiceIDs(m map[ServiceID]*Service) []ServiceID {
    ids := make([]ServiceID, 0, len(m))
    for id := range m {
        ids = append(ids, id)
    }
    sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
    return ids
}

func sortedKeys[V any](m map[string]V) []string {
    keys := make([]string, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    return keys
}
```

---

## 4. DAG Validation Patterns

### P-VAL-DAG: Kahn's algorithm (Q4=A)

```go
// validate.go

func validateDAG(s *Schema) []error {
    // 1. 全 Operation を収集
    var allOps []*Operation
    for _, svc := range s.Services {
        for _, op := range svc.Operations {
            allOps = append(allOps, op)
        }
    }

    // 2. 入次数を計算
    inDegree := make(map[*Operation]int, len(allOps))
    for _, op := range allOps {
        inDegree[op] = 0
    }
    forEachOutgoingEdge(s, func(e *Edge) {
        inDegree[e.To]++
    })

    // 3. 入次数 0 から BFS
    queue := make([]*Operation, 0, len(allOps))
    for _, op := range allOps {
        if inDegree[op] == 0 {
            queue = append(queue, op)
        }
    }
    visited := 0
    for len(queue) > 0 {
        op := queue[0]
        queue = queue[1:]
        visited++
        for _, child := range outgoingTargets(op) {
            inDegree[child]--
            if inDegree[child] == 0 {
                queue = append(queue, child)
            }
        }
    }

    // 4. 全 op を訪問できなければ循環
    if visited < len(allOps) {
        // 訪問できなかった op を報告
        unvisited := make([]string, 0)
        for op, deg := range inDegree {
            if deg > 0 {
                unvisited = append(unvisited, identifyOp(op))
            }
        }
        sort.Strings(unvisited)
        return []error{&ValidationError{
            Path:    "operation graph",
            Rule:    "R-STR-4",
            Message: fmt.Sprintf("cycle detected; operations in cycle: %v", unvisited),
        }}
    }
    return nil
}

func forEachOutgoingEdge(s *Schema, fn func(*Edge)) {
    for _, svc := range s.Services {
        for _, op := range svc.Operations {
            forEachEdgeInCalls(op.Calls, fn)
        }
    }
}

func forEachEdgeInCalls(calls []*CallNode, fn func(*Edge)) {
    for _, n := range calls {
        if n.Edge != nil {
            fn(n.Edge)
            if n.Edge.OnFailure != nil {
                for _, fallback := range n.Edge.OnFailure.Fallback {
                    fn(fallback)
                }
            }
        } else {
            forEachEdgeInCalls(n.Parallel, fn)
        }
    }
}

func outgoingTargets(op *Operation) []*Operation {
    var targets []*Operation
    forEachEdgeInCalls(op.Calls, func(e *Edge) {
        targets = append(targets, e.To)
    })
    return targets
}
```

複雑度: O(V + E)。`map` 操作のため定数倍は重いが、規模が小さい (典型 30 ops + 50 edges) ので 10ms 目標に十分余裕。

---

## 5. Default Application Patterns (Q6=A)

```go
// parse.go

func buildSchema(raw *rawSchema) *Schema {
    schema := &Schema{
        Services: make(map[ServiceID]*Service, len(raw.Services)),
        Journeys: make(map[string]*Journey, len(raw.Journeys)),
        // Faults は Phase 2b で構築
    }
    for name, rs := range raw.Services {
        svc := &Service{
            Name:       ServiceID(name),
            Kind:       parseServiceKind(rs.Kind),
            Replicas:   intDefault(rs.Replicas, 1),       // Q3=A: デフォルト適用
            Language:   rs.Language,
            Framework:  rs.Framework,
            Version:    rs.Version,
            Operations: make(map[string]*Operation, len(rs.Operations)),
        }
        for _, ro := range rs.Operations {
            svc.Operations[ro.Name] = &Operation{
                Name:    ro.Name,
                Service: svc,
                // Calls は Phase 2b で構築
            }
        }
        schema.Services[svc.Name] = svc
    }
    return schema
}

// helpers — 短小関数で意図明確化
func intDefault(p *int, def int) int        { if p == nil { return def }; return *p }
func float64Default(p *float64, def float64) float64 { if p == nil { return def }; return *p }
func durationDefault(p *time.Duration, def time.Duration) time.Duration { if p == nil { return def }; return *p }
```

Phase 2b で Edge を構築する際も同じパターン:

```go
edge := &Edge{
    From:         owningOp,
    To:           targetOp,
    Protocol:     parseProtocol(rc.Protocol),
    Latency:      resolveLatency(rc.Latency),
    ErrorRate:    float64Default(rc.ErrorRate, 0.0),
    Timeout:      durationDefault(rc.Timeout, 0),
    Retries:      intDefault(rc.Retries, 0),
    RetryBackoff: parseBackoff(rc.RetryBackoff),
}
```

`applyDefaults()` メソッドを別に作らない (Q6=A、複数パス回避)。

---

## 6. Immutability & Concurrency Patterns

### P-IMM-1: Convention via GoDoc (Q8=A)

`topology/doc.go` に明示:

```go
// Package topology provides the schema, parser, and validator for declarative
// microservice topologies consumed by xk6-otel-gen.
//
// IMMUTABILITY: Schema and all the types it contains (Service, Operation,
// Edge, Journey, FaultSpec, ...) are designed to be IMMUTABLE after Parse
// returns successfully. Mutating any field — including writing to
// Schema.Services or Service.Operations maps, appending to slices, or
// changing field values — yields undefined behavior, especially under
// concurrent access. Treat *Schema as read-only.
//
// CONCURRENCY: A read-only *Schema is safe to share across goroutines.
// Multiple journey engines (e.g., per-VU in k6) may read the same Schema
// instance concurrently without locking. The package itself holds no
// global mutable state and is fully reentrant.
package topology
```

`*Schema` 型の GoDoc 冒頭にも短く:

```go
// Schema is the root of a parsed topology. It is immutable after Parse
// returns; treat all fields as read-only. See package documentation for
// concurrency guarantees.
type Schema struct { ... }
```

### P-IMM-2: defensive copy しない (NFR-U1-5)

`(*Schema).FindServiceByName` などのメソッドは `*Service` をそのまま返す (deep copy しない)。利用者は read-only convention に従う:

```go
func (s *Schema) FindServiceByName(id ServiceID) (*Service, bool) {
    svc, ok := s.Services[id]
    return svc, ok // pointer のまま、defensive copy なし
}
```

deep copy は性能インパクトが大きく、convention 強制なら不要。

### P-CONC-1: パッケージレベル mutable state なし

`topology` パッケージは:
- パッケージレベル `var` での mutable state なし
- `init()` 関数なし
- グローバル cache なし

これにより複数 goroutine から Parse / Validate / ApplyFaults を同時呼び出ししても安全。

`go vet -race` テストで保証。

---

## 7. JSON Schema Patterns

### P-JSON-1: hand-written template (Q11=A)

`topology/jsonschema/topology.schema.json` を手書き、`//go:embed` で取り込む:

```go
// jsonschema.go

import _ "embed"

//go:embed jsonschema/topology.schema.json
var jsonSchemaTemplate []byte

func (s *Schema) ExportJSONSchema() ([]byte, error) {
    // 利用者が変更しないよう新しい slice を返す
    out := make([]byte, len(jsonSchemaTemplate))
    copy(out, jsonSchemaTemplate)
    return out, nil
}
```

template は U1 Code Generation で:
- `$schema: https://json-schema.org/draft/2020-12/schema`
- `$id: https://github.com/ymotongpoo/xk6-otel-gen/schemas/topology.schema.json`
- `services`, `journeys`, `faults` のトップレベル
- `$defs` に各型 (Service, Operation, ...) の schema
- enum 値の列挙
- 各フィールドに `description` (FD `domain-entities.md` §1 から引用)
- 1-2 例の `examples`

を含む。`additionalProperties: true` (NFR-U1-2C: lax 入力許容)。

### P-JSON-2: 整合性テスト (任意)

`Go 型とテンプレートの整合性` を独立 example-based test で確認する (TP-U1-8 とは別、内部 sanity check):

```go
func TestJSONSchemaTemplate_ContainsAllEnums(t *testing.T) {
    schema, err := (&Schema{}).ExportJSONSchema()
    require.NoError(t, err)
    s := string(schema)

    // ServiceKind 全値
    require.Contains(t, s, "application")
    require.Contains(t, s, "database")
    require.Contains(t, s, "external_api")
    require.Contains(t, s, "cache")
    require.Contains(t, s, "queue")

    // Protocol 全値
    require.Contains(t, s, "http")
    require.Contains(t, s, "grpc")
    require.Contains(t, s, "messaging")
    // ...
}
```

これによりテンプレート手書きで enum 値追加忘れを検知。

---

## 8. Testing Patterns

### P-TEST-1: inline fixtures (Q9=A)

簡単な example-based test は inline 文字列で:

```go
func TestParse_MinimalSchema(t *testing.T) {
    t.Parallel()
    const yamlSrc = `
services:
  frontend:
    kind: application
    operations:
      - name: GET /
        calls: []
journeys:
  hello:
    steps:
      - service: frontend
        operation: GET /
`
    s, err := Parse(strings.NewReader(yamlSrc))
    require.NoError(t, err)
    require.Len(t, s.Services, 1)
}
```

複雑な fixture (bench 入力など) は `testdata/typical.yaml` 等のファイルに置く (Q9 は inline を選んだが、典型 YAML レベルの長さは inline で苦しい — 実装時に判断、両方併用)。

### P-TEST-2: ファイル構成 (Q12=A)

FD `domain-entities.md` §3 のレイアウトをそのまま採用。`logical-components.md` で各ファイルを LC として整理。

### P-TEST-3: t.Parallel() ですべて並列

全テスト関数の最初に `t.Parallel()` (NFR-U1-6 thread-safety の根拠)。

---

## 9. API Extension / Backward-Compat Patterns

### P-API-1: SemVer post-v1 (NFR-U1-10)

U7 と同じ:
- v1.0.0 まで: 破壊変更 OK
- v1.0.0 以降: 後方互換性厳守、`// Deprecated:` GoDoc コメント + 1 minor version 猶予

### P-API-2: 新規 public 関数追加は OK

将来の追加例:
- `Lint` の wrapper として `LintFile(path string) ([]LintIssue, error)`
- `Schema.Operations() iter.Seq[*Operation]` (range over func) — Go 1.23+ 機能、1.25 で安定済み

これらは patch リリースで追加可能。

---

## 10. Documentation Patterns

### P-DOC-1: Example function (top-level 関数のみ)

U7 と同じパターン:
- `ExampleParse` — minimal な YAML を Parse する例
- `ExampleSchema_MarshalYAML` — round-trip の例
- `ExampleLint` — Lint 結果を読む例

atomic helpers (`intDefault` 等) は example なし、GoDoc 1 行で十分。

### P-DOC-2: パッケージ doc.go

`topology/doc.go` に:
- パッケージ概要 (3-4 段落)
- Immutability / Concurrency 明示 (P-IMM-1)
- 主要型・関数の overview (Parse, Validate, Equal, Lint, ApplyFaults, ExportJSONSchema)
- 利用例の概要 (詳細は Example function に委ねる)
- 参照: FD / NFR-D へのリンクは aidlc-docs にあるので doc.go に書かない (利用者目線)

---

## 11. パターン適用と NFR の対応表

| NFR | 関連パターン |
|---|---|
| NFR-U1-1 (Parse 10ms) | P-PERF-1, P-PERF-2, P-PERF-3, P-PERF-4 |
| NFR-U1-2 (memory ≤1MB) | P-PERF-2 (アロケーション最小化) |
| NFR-U1-3 (no library logs) | (パターン不要、`log` import なしで自然に達成) |
| NFR-U1-5 (immutability) | P-IMM-1, P-IMM-2 |
| NFR-U1-6 (thread-safe) | P-IMM-1, P-CONC-1 |
| NFR-U1-7 (Go 1.25+) | (パターン不要、go.mod で宣言) |
| NFR-U1-8 (coverage 80%) | テスト網羅で達成 (`logical-components.md` §4) |
| NFR-U1-9 (Lint ≤15ms) | P-PERF-1 (decodeRaw 共通化) |
| NFR-U1-10 (SemVer) | P-API-1, P-API-2 |
| Validate / errors.Join | P-ERR-1, P-ERR-2, P-ERR-3 |
| MarshalYAML round-trip | P-MARSHAL-1, P-MARSHAL-2 |
| DAG check | P-VAL-DAG |
| Default values | P-PERF-2 (intDefault 等) |
| JSON Schema | P-JSON-1, P-JSON-2 |
| Testing | P-TEST-1, P-TEST-2, P-TEST-3 |
| Documentation | P-DOC-1, P-DOC-2 |
