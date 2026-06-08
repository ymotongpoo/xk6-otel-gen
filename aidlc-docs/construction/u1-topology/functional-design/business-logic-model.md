# U1 topology — Business Logic Model

本書は `topology` パッケージのメソッド業務ロジック (Parse / Validate / MarshalYAML / Equal / ApplyFaults / ExportJSONSchema / FindServiceByName / JourneyNames) を確定する。型定義は U7 で scaffold 済み (`topology/types.go`, `topology/enums.go`)。

---

## 1. Parse の 2-pass フロー (Q1=C, Q2=C, Q3=A)

### 全体パイプライン

```text
io.Reader
   │
   │ Phase 1: YAML Decode (fail-fast)
   ▼
*rawSchema (string-based references)
   │
   │ Phase 2a: Build typed objects (defaults applied here per Q3=A)
   ▼
*Schema (services + operations populated, edges still unresolved)
   │
   │ Phase 2b: Resolve all string refs → pointers (collect errors per Q1=C)
   ▼
*Schema (all *Service / *Operation / *Edge pointers non-nil)
   │
   │ Phase 3: Validate (collect all violations per Q1=C, Q6=B)
   ▼
*Schema または error (multi-error containing structural + domain violations)
```

### Phase 1: YAML Decode (fail-fast、Q1=C)

```go
func Parse(r io.Reader) (*Schema, error) {
    raw, err := decodeRaw(r)
    if err != nil {
        // YAML syntax error: 行番号付きで即停止 (fail-fast)
        return nil, fmt.Errorf("topology: yaml decode: %w", err)
    }
    // ... Phase 2 へ
}

// rawSchema は yaml タグ駆動で `yaml.Unmarshal` する内部型。
// KnownFields(true) は使わない (Q2=C: lax)。
type rawSchema struct {
    Services map[string]*rawService `yaml:"services"`
    Journeys map[string]*rawJourney `yaml:"journeys"`
    Faults   []*rawFault            `yaml:"faults"`
    Extras   map[string]any         `yaml:",inline"` // 未知キーを記録 (Lint API 用)
}

type rawService struct {
    Kind       string                   `yaml:"kind"`
    Replicas   *int                     `yaml:"replicas,omitempty"`   // nil = default 適用
    Language   string                   `yaml:"language,omitempty"`
    // ...
    Operations []*rawOperation          `yaml:"operations"`
}

type rawOperation struct {
    Name  string         `yaml:"name"`
    Calls []*rawCallNode `yaml:"calls,omitempty"`
}

type rawCallNode struct {
    To        *rawCallTarget    `yaml:"to,omitempty"`        // 単一呼び出し
    Parallel  []*rawCallNode    `yaml:"parallel,omitempty"`  // 並列グループ
    Protocol  string            `yaml:"protocol,omitempty"`
    Latency   *rawLatencyDist   `yaml:"latency,omitempty"`
    ErrorRate *float64          `yaml:"error_rate,omitempty"`
    Timeout   *time.Duration    `yaml:"timeout,omitempty"`
    Retries   *int              `yaml:"retries,omitempty"`
    RetryBackoff string         `yaml:"retry_backoff,omitempty"`
    OnFailure *rawRecoveryPolicy `yaml:"on_failure,omitempty"`
}

type rawCallTarget struct {
    Service   string `yaml:"service"`
    Operation string `yaml:"operation"`
}
```

### Phase 2a: Build typed objects + Apply defaults (Q3=A)

```go
func buildSchema(raw *rawSchema) *Schema {
    schema := &Schema{
        Services: make(map[ServiceID]*Service, len(raw.Services)),
        Journeys: make(map[string]*Journey, len(raw.Journeys)),
    }
    // Step 1: Service 構築 (Operations の back-pointer は同時設定)
    for name, rs := range raw.Services {
        svc := &Service{
            Name:       ServiceID(name),
            Kind:       parseServiceKind(rs.Kind),
            Replicas:   intDefault(rs.Replicas, 1),    // Q3=A: default を適用
            Language:   rs.Language,
            Framework:  rs.Framework,
            Version:    rs.Version,
            Operations: make(map[string]*Operation, len(rs.Operations)),
        }
        for _, ro := range rs.Operations {
            op := &Operation{
                Name:    ro.Name,
                Service: svc, // back-pointer 即時設定
                Calls:   nil, // Phase 2b で構築
            }
            svc.Operations[ro.Name] = op
        }
        schema.Services[svc.Name] = svc
    }
    return schema
}
```

Default 値 (Q3=A):
- `Replicas`: nil → `1`
- `ErrorRate`: nil → `0.0`
- `Timeout`: nil → `0` (= 無効/無制限の意味)
- `Retries`: nil → `0`
- `RetryBackoff`: "" → `"exponential"`
- `LatencyDist.Distribution`: "" → `"constant"`
- `Journey.Weight`: 0 → `1.0`

### Phase 2b: Reference Resolution (errors collected、Q1=C)

```go
func resolveReferences(schema *Schema, raw *rawSchema) error {
    var errs []error

    // Step 1: Service.Operations[*].Calls を解決
    for svcName, rs := range raw.Services {
        svc := schema.Services[ServiceID(svcName)]
        for _, ro := range rs.Operations {
            op := svc.Operations[ro.Name]
            op.Calls = make([]*CallNode, 0, len(ro.Calls))
            for i, rc := range ro.Calls {
                node, err := resolveCallNode(schema, svc, op, rc, fmt.Sprintf("services.%s.operations.%s.calls[%d]", svcName, ro.Name, i))
                if err != nil {
                    errs = append(errs, err)
                    continue
                }
                op.Calls = append(op.Calls, node)
            }
        }
    }

    // Step 2: Journeys を解決
    for jName, rj := range raw.Journeys {
        journey := &Journey{
            Name:   jName,
            Weight: float64Default(rj.Weight, 1.0),
            Steps:  make([]*Step, 0, len(rj.Steps)),
        }
        for i, rs := range rj.Steps {
            step, err := resolveStep(schema, rs, fmt.Sprintf("journeys.%s.steps[%d]", jName, i))
            if err != nil {
                errs = append(errs, err)
                continue
            }
            journey.Steps = append(journey.Steps, step)
        }
        schema.Journeys[jName] = journey
    }

    // Step 3: Faults を解決
    schema.Faults = make([]FaultSpec, 0, len(raw.Faults))
    for i, rf := range raw.Faults {
        target, err := resolveFaultTarget(schema, rf.Target, fmt.Sprintf("faults[%d].target", i))
        if err != nil {
            errs = append(errs, err)
            continue
        }
        schema.Faults = append(schema.Faults, FaultSpec{
            Target:   target,
            Kind:     parseFaultKind(rf.Kind),
            Severity: rf.Severity,
        })
    }

    return errors.Join(errs...)  // Go 1.20+: nil if errs is empty or all nil
}

// resolveCallNode は単一の rawCallNode を CallNode に変換する。
// rc.To と rc.Parallel は排他 (R-STR-7)、両方 nil ならエラー。
func resolveCallNode(schema *Schema, owningSvc *Service, owningOp *Operation, rc *rawCallNode, path string) (*CallNode, error) {
    hasTo := rc.To != nil
    hasParallel := len(rc.Parallel) > 0
    if hasTo == hasParallel {
        return nil, fmt.Errorf("%s: exactly one of 'to' or 'parallel' is required (R-STR-7)", path)
    }
    if hasParallel {
        children := make([]*CallNode, 0, len(rc.Parallel))
        for i, child := range rc.Parallel {
            cn, err := resolveCallNode(schema, owningSvc, owningOp, child, fmt.Sprintf("%s.parallel[%d]", path, i))
            if err != nil {
                return nil, err
            }
            children = append(children, cn)
        }
        return &CallNode{Parallel: children}, nil
    }
    // hasTo の場合: Edge を構築
    targetOp, err := lookupOperation(schema, rc.To.Service, rc.To.Operation)
    if err != nil {
        return nil, fmt.Errorf("%s.to: %w", path, err)
    }
    edge := &Edge{
        From:      owningOp,
        To:        targetOp,
        Protocol:  parseProtocol(rc.Protocol),
        Latency:   resolveLatency(rc.Latency),
        ErrorRate: float64Default(rc.ErrorRate, 0.0),
        Timeout:   durationDefault(rc.Timeout, 0),
        Retries:   intDefault(rc.Retries, 0),
        RetryBackoff: parseBackoff(rc.RetryBackoff),
    }
    if rc.OnFailure != nil {
        rp, err := resolveRecoveryPolicy(schema, owningOp, rc.OnFailure, path+".on_failure")
        if err != nil {
            return nil, err
        }
        edge.OnFailure = rp
    }
    return &CallNode{Edge: edge}, nil
}

func lookupOperation(schema *Schema, svcName, opName string) (*Operation, error) {
    svc, ok := schema.Services[ServiceID(svcName)]
    if !ok {
        return nil, fmt.Errorf("service %q not found", svcName)
    }
    op, ok := svc.Operations[opName]
    if !ok {
        return nil, fmt.Errorf("operation %q on service %q not found", opName, svcName)
    }
    return op, nil
}
```

### Phase 3: Validate (Q6=B)

Phase 2 が `error == nil` を返したら、Validate を呼んで構造的 + ドメイン妥当性を検証 (詳細は `business-rules.md` §3)。Validate も errors.Join で全違反を集約。

```go
func Parse(r io.Reader) (*Schema, error) {
    raw, err := decodeRaw(r)
    if err != nil {
        return nil, err
    }
    schema := buildSchema(raw)
    if err := resolveReferences(schema, raw); err != nil {
        return nil, err
    }
    if err := Validate(schema); err != nil {
        return nil, err
    }
    return schema, nil
}
```

---

## 2. Lint API (Q2=C)

Parse は未知 YAML キーを **無視** する (lax)。`Lint(r io.Reader)` を別途公開し、Parse と同等の処理を行いつつ未知キー警告を返す:

```go
type LintIssue struct {
    Path     string        // 例: "services.foo.unknown_field"
    Severity LintSeverity  // LintWarning | LintError
    Message  string
}

type LintSeverity int
const (
    LintWarning LintSeverity = iota
    LintError
)

func Lint(r io.Reader) ([]LintIssue, error) {
    // Parse と同じ流れだが、yaml.Decoder の KnownFields(true) で未知キーを検出
    // また、Validate のドメイン妥当性違反も LintIssue として返す
}
```

`Lint` は **CLI ツール `cmd/xk6-otel-gen-schema/`** から呼ばれる想定 (U8 で実装)。

---

## 3. MarshalYAML (Q4=A, Q5=A)

`(*Schema).MarshalYAML() (any, error)` は `*Service` / `*Operation` 等のポインタを名前文字列に逆変換し、yaml.v3 が利用可能な中間構造体を返す。

### 並び順 (Q4=A: アルファベット昇順)

- `Schema.Services` map → ServiceID 昇順
- 各 Service の `Operations` map → Operation.Name 昇順
- `Schema.Journeys` map → 名前昇順
- `Schema.Faults` slice → **元の登場順を維持** (slice は parse 時に index を保つ)
- `Operation.Calls` slice → **元の登場順を維持** (シーケンスは業務的に意味を持つ、Q5=A の前提)

### 中間構造体

`rawSchema` (Parse の Phase 1 で使ったもの) と全く同じ構造を返す。これにより `yaml.Marshal(schema)` で `rawSchema` がそのまま YAML 化される。

```go
func (s *Schema) MarshalYAML() (any, error) {
    raw := &rawSchema{
        Services: make(map[string]*rawService, len(s.Services)),
        Journeys: make(map[string]*rawJourney, len(s.Journeys)),
        Faults:   make([]*rawFault, 0, len(s.Faults)),
    }
    // Services: ServiceID 昇順で書き出す
    svcIDs := sortedServiceIDs(s.Services)
    for _, id := range svcIDs {
        svc := s.Services[id]
        raw.Services[string(id)] = marshalService(svc)
    }
    // Journeys: 名前昇順
    jNames := sortedKeys(s.Journeys)
    for _, name := range jNames {
        raw.Journeys[name] = marshalJourney(s.Journeys[name])
    }
    // Faults: 登場順維持
    for _, f := range s.Faults {
        raw.Faults = append(raw.Faults, marshalFault(f))
    }
    return raw, nil
}
```

map の繰り返しがアルファベット順なのは、yaml.v3 が `MapSlice` を使わず通常 map を使う限り **挿入順** を尊重する仕様に依存する (yaml.v3 ≥ 3.0)。あるいは MarshalYAML 内で `yaml.MapSlice` を返して明示順序を保証する。実装は U1 Code Generation で確定。

---

## 4. Equal (Q5=A: identifier-based strict)

```go
func Equal(a, b *Schema) bool {
    if a == nil || b == nil {
        return a == b
    }
    return equalServices(a.Services, b.Services) &&
        equalJourneys(a.Journeys, b.Journeys) &&
        equalFaults(a.Faults, b.Faults)
}

func equalServices(a, b map[ServiceID]*Service) bool {
    if len(a) != len(b) {
        return false
    }
    for id, sa := range a {
        sb, ok := b[id]
        if !ok || !equalService(sa, sb) {
            return false
        }
    }
    return true
}

func equalService(a, b *Service) bool {
    return a.Name == b.Name &&
        a.Kind == b.Kind &&
        a.Replicas == b.Replicas &&
        a.Language == b.Language &&
        a.Framework == b.Framework &&
        a.Version == b.Version &&
        equalOperations(a.Operations, b.Operations)
}

func equalOperations(a, b map[string]*Operation) bool {
    if len(a) != len(b) {
        return false
    }
    for name, oa := range a {
        ob, ok := b[name]
        if !ok || !equalOperation(oa, ob) {
            return false
        }
    }
    return true
}

func equalOperation(a, b *Operation) bool {
    if a.Name != b.Name { return false }
    // Service back-pointer 経由で同じサービス所属を確認
    if a.Service.Name != b.Service.Name { return false }
    return equalCalls(a.Calls, b.Calls)
}

func equalCalls(a, b []*CallNode) bool {
    if len(a) != len(b) {
        return false
    }
    // Q5=A: Calls 順序維持
    for i := range a {
        if !equalCallNode(a[i], b[i]) {
            return false
        }
    }
    return true
}

func equalCallNode(a, b *CallNode) bool {
    if (a.Edge == nil) != (b.Edge == nil) {
        return false
    }
    if a.Edge != nil {
        return equalEdge(a.Edge, b.Edge)
    }
    return equalCallNodes(a.Parallel, b.Parallel)
}

func equalEdge(a, b *Edge) bool {
    return identifyOp(a.From) == identifyOp(b.From) &&
        identifyOp(a.To) == identifyOp(b.To) &&
        a.Protocol == b.Protocol &&
        equalLatency(a.Latency, b.Latency) &&
        a.ErrorRate == b.ErrorRate &&
        a.Timeout == b.Timeout &&
        a.Retries == b.Retries &&
        a.RetryBackoff == b.RetryBackoff &&
        equalRecoveryPolicy(a.OnFailure, b.OnFailure)
}

// identifyOp は *Operation を一意識別する key を返す: "<svcName>.<opName>"
func identifyOp(op *Operation) string {
    if op == nil { return "" }
    return string(op.Service.Name) + "." + op.Name
}
```

ポイント:
- **`*Operation` の比較は identifier 文字列 (`<svcName>.<opName>`) で行う**。ポインタ比較 (`a == b`) はしない (異なる Schema インスタンス間で比較するため)
- `Service.Operations` map と `Schema.Services` map は **キー集合 + 各値の equal で順不同 OK**
- `CallNode` / `Edge` のスライスは **順序維持**

---

## 5. Validate (Q6=B)

詳細な検証項目と分類は `business-rules.md` §3 に記述。本セクションでは関数の構造のみ:

```go
func Validate(s *Schema) error {
    var errs []error

    // 構造的検証 (R-STR-1..8)
    errs = append(errs, validateBackPointers(s)...)
    errs = append(errs, validateNoOrphanReferences(s)...)
    errs = append(errs, validateDAG(s)...)
    errs = append(errs, validateJourneyReachability(s)...)
    errs = append(errs, validateFaultTargets(s)...)
    errs = append(errs, validateCallNodeVariants(s)...)
    errs = append(errs, validateRecoveryPolicyOwnership(s)...)

    // ドメイン妥当性 (Q6=B)
    errs = append(errs, validateDomainRanges(s)...)

    return errors.Join(errs...)
}

// validateDAG は Operation を node とし Edge を edge とする有向グラフが
// DAG (循環なし) であることを Tarjan の SCC または Kahn のトポロジカルソートで検証。
func validateDAG(s *Schema) []error {
    // Kahn's algorithm の variant: 各 operation の incoming-edge-count を計算し
    // 0 を起点に BFS、最終的に全 operation を訪問できれば DAG。
}

func validateDomainRanges(s *Schema) []error {
    var errs []error
    for _, svc := range s.Services {
        if svc.Replicas < 1 {
            errs = append(errs, fmt.Errorf("services.%s.replicas: must be >= 1, got %d", svc.Name, svc.Replicas))
        }
        for _, op := range svc.Operations {
            errs = append(errs, validateOperationDomain(svc, op)...)
        }
    }
    for i, f := range s.Faults {
        if f.Severity.Probability < 0 || f.Severity.Probability > 1 {
            errs = append(errs, fmt.Errorf("faults[%d].severity.probability: must be in [0,1], got %v", i, f.Severity.Probability))
        }
    }
    return errs
}
```

---

## 6. ApplyFaults (Q7=A, Q8=A)

```go
type FaultOverlay struct {
    nodeFaults      map[*Service][]FaultSpec
    operationFaults map[*Operation][]FaultSpec
    edgeFaults      map[*Edge][]FaultSpec
}

func (s *Schema) ApplyFaults() *FaultOverlay {
    o := &FaultOverlay{
        nodeFaults:      map[*Service][]FaultSpec{},
        operationFaults: map[*Operation][]FaultSpec{},
        edgeFaults:      map[*Edge][]FaultSpec{},
    }
    for _, f := range s.Faults {
        switch f.Target.Kind {
        case TargetNode:
            o.nodeFaults[f.Target.Service] = append(o.nodeFaults[f.Target.Service], f)
        case TargetOperation:
            o.operationFaults[f.Target.Operation] = append(o.operationFaults[f.Target.Operation], f)
        case TargetEdge:
            o.edgeFaults[f.Target.Edge] = append(o.edgeFaults[f.Target.Edge], f)
        }
    }
    return o
}

// 公開 lookup API (Journey Engine が使う)
func (o *FaultOverlay) NodeFaults(svc *Service) []FaultSpec       { return o.nodeFaults[svc] }
func (o *FaultOverlay) OperationFaults(op *Operation) []FaultSpec { return o.operationFaults[op] }
func (o *FaultOverlay) EdgeFaults(e *Edge) []FaultSpec            { return o.edgeFaults[e] }
```

カスケード判定は **Journey Engine が実行時** に行う (Q8=A、Application Design `services.md` §O-4 で確定済み)。Overlay は単純 lookup table のみ。

---

## 7. ExportJSONSchema (Q9=A)

JSON Schema Draft 2020-12 を出力。**静的テンプレート方式** (リフレクションでなく hand-written) を採用:

```go
//go:embed jsonschema/topology.schema.json
var jsonSchemaTemplate []byte

func (s *Schema) ExportJSONSchema() ([]byte, error) {
    // テンプレートをそのまま返す。バリデーション対象は s ではなく
    // "本 schema が生成する YAML" の構造なので、s に依存しない静的出力で十分。
    return jsonSchemaTemplate, nil
}
```

JSON Schema ファイル本体は U1 Code Generation で `topology/jsonschema/topology.schema.json` として書き、`//go:embed` で取り込む。テンプレートに含める内容:
- `$schema`: `https://json-schema.org/draft/2020-12/schema`
- `services`, `journeys`, `faults` の各セクション定義
- enum 値 (ServiceKind, Protocol, ExhaustedAction, FaultKind, BackoffPolicy)
- `description` / `examples` (minimal な YAML サンプル)
- `additionalProperties: true` (Q2=C: lax)

---

## 8. FindServiceByName / JourneyNames

```go
func (s *Schema) FindServiceByName(id ServiceID) (*Service, bool) {
    svc, ok := s.Services[id]
    return svc, ok
}

func (s *Schema) JourneyNames() []string {
    names := make([]string, 0, len(s.Journeys))
    for n := range s.Journeys {
        names = append(names, n)
    }
    sort.Strings(names)  // 決定論性のため (Q4=A)
    return names
}
```

---

## 9. データフロー図

```mermaid
flowchart LR
    YAML[(topology.yaml)]
    Parse["Parse(io.Reader)"]
    Validate["Validate(*Schema)"]
    Apply["ApplyFaults()"]
    JE[Journey Engine<br/>(U2)]
    Marshal["MarshalYAML()"]
    JSON["ExportJSONSchema()"]
    Lint["Lint(io.Reader)"]

    YAML --> Parse
    YAML --> Lint
    Parse -->|*Schema| Validate
    Validate -->|*Schema valid| Apply
    Apply -->|*FaultOverlay| JE
    Parse -->|*Schema| Marshal
    Marshal --> YAML
    Parse -->|*Schema| JSON
    JSON --> Editor[(IDE schema completion)]

    classDef u1 fill:#90CAF9,stroke:#0D47A1,color:#000
    classDef ext fill:#FFF59D,stroke:#F57F17,color:#000
    class Parse,Validate,Apply,Marshal,JSON,Lint u1
    class JE,Editor ext
```

---

## 10. テストフレームワーク (Q11=A, Q12=B)

- 性能目標: 典型的 YAML (10 services / 30 operations / 50 edges) を **10 ms 以下** で Parse 完了 (`BenchmarkParse`)
- ストリーミング不要 (Q10=A): `io.ReadAll` で全読み込みしてから `yaml.Unmarshal`
- TP-U1-1..8 のうち TP-U1-1..5 は Application Design で識別済み。本 FD で TP-U1-6..8 を追加 (詳細は `business-rules.md` §10)
- TP-U1-8 (JSON Schema round-trip) のために **依存追加**: `github.com/santhosh-tekuri/jsonschema/v5` (テスト依存)
