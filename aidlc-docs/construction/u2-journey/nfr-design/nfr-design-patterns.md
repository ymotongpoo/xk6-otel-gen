# U2 journey — NFR Design Patterns

本書は U2 (`journey/`) の **「どう実装するか」** のパターン群を確定する。FD + NFR-R を受けて、Performance / Concurrency / Error / API / Documentation / Test の各カテゴリで実装パターンを決める。

参照:
- FD: `aidlc-docs/construction/u2-journey/functional-design/`
- NFR-R: `aidlc-docs/construction/u2-journey/nfr-requirements/`
- Plan + Answers: `aidlc-docs/construction/plans/u2-journey-nfr-d-plan.md`
- U1 actual API: `topology/types.go` (FaultOverlay, Edge.LatencyDist)

---

## 1. Performance パターン

### 1.1 Engine struct の物理 layout (Q1=A baseline)

```go
type engineImpl struct {
    schema  *topology.Schema       // read-only after construction
    overlay *topology.FaultOverlay // read-only
    synth   synth.Synthesizer      // thread-safe by U3 NFR-U3-3

    plans map[string]*Plan         // populated in NewEngine, read-only after

    rand *rand.Rand                // shared random source
    rmu  sync.Mutex                // protects rand
}
```

すべての field は構築後 read-only (rand は内容 mutable だが mu で保護)、Plan / overlay は immutable map。

### 1.2 Random source (Q1=A)

`per-Engine + sync.Mutex` を baseline:

```go
func (e *engineImpl) randIntN(n int) int {
    if n <= 1 {
        return 0
    }
    e.rmu.Lock()
    defer e.rmu.Unlock()
    return e.rand.IntN(n)
}

func (e *engineImpl) randFloat64() float64 {
    e.rmu.Lock()
    defer e.rmu.Unlock()
    return e.rand.Float64()
}
```

#### 1.2.1 Bench で再評価

NFR-U2-6 の per-step overhead < 50 µs を満たすか bench で確認。k6 高並列 (1000 VU) で mutex 競合が顕著なら以下に切り替え:
1. `math/rand/v2` global `rand.IntN()` (thread-safe global、seed deterministic 困難)
2. per-VU `*rand.Rand` instance (k6 init phase で goroutine-local 構築)

切り替え判断は Code Generation Phase の bench 結果次第。

#### 1.2.2 Seed

`NewEngine` で `rand.New(rand.NewPCG(seed1, seed2))` (rand/v2 の PCG generator) を構築。seed は引数で受け取るか、`time.Now().UnixNano()` を default に。Test では deterministic seed 注入 (`NewEngineWithSeed` 等の test helper、API 表面には出さない)。

### 1.3 executeNode 直接再帰 (Q2=A)

```go
func (e *engineImpl) executeNode(ctx context.Context, node *Node, parent *Outcome) Outcome {
    // ... fault eval, sleep, recursive children ...
}
```

- 関数 stack は深さ 5-10 程度で safe (Go の default stack は 1 MB から動的拡張)
- code readability 優先
- profile / pprof で stack 圧迫が観測されたら NFR Design 改訂

### 1.4 Outcome 構築 (Q7=A)

```go
func (e *engineImpl) executeNode(ctx context.Context, node *Node, parent *Outcome) Outcome {
    // Initialize as success default; mutate as we discover failures.
    outcome := Outcome{
        Success:    true,
        StatusCode: 200,  // overridden by Operation/Edge spec or fault
    }
    
    startTime := time.Now()
    // 0. cascade check
    if parent != nil && !parent.Success {
        outcome.Success = false
        outcome.Cascaded = true
        outcome.ErrorType = parent.ErrorType
        outcome.StatusCode = parent.StatusCode
        // Sleep skip; emit zero-duration span (Q8=A)
        e.emitCascadeSpan(ctx, node, outcome)
        return outcome
    }
    // ... fault eval, replica selection, span lifecycle, children traversal ...
    outcome.Latency = time.Since(startTime)
    return outcome
}
```

local var を逐次更新、最後に return。Outcome は value type (struct copy で渡る)。

### 1.5 BuildPlan の eager 構築 (NFR-R Q7=A)

`NewEngine` で `schema.Journeys` をループして全 Plan を `plans map` に格納。

```go
func NewEngine(schema *topology.Schema, overlay *topology.FaultOverlay, syn synth.Synthesizer) *Engine {
    e := &engineImpl{
        schema: schema, overlay: overlay, synth: syn,
        plans: make(map[string]*Plan, len(schema.Journeys)),
        rand:  newDefaultRand(),
    }
    for name := range schema.Journeys {
        plan, err := e.buildPlan(name)
        if err != nil {
            panic(fmt.Sprintf("journey: NewEngine: build %q: %v", name, err))
        }
        e.plans[name] = plan
    }
    return (*Engine)(e)
}
```

NFR-U2-2: 構築失敗は panic (programmer error / Schema 不正)。

---

## 2. Concurrency パターン

### 2.1 Parallel goroutine 起動 (Q3=A)

```go
func (e *engineImpl) executeParallelGroup(ctx context.Context, group *Node, parent *Outcome) Outcome {
    var wg sync.WaitGroup
    outcomes := make([]Outcome, len(group.Parallel))

    for i, child := range group.Parallel {
        wg.Add(1)
        go func(idx int, ch *Node) {
            defer wg.Done()
            defer func() {
                if r := recover(); r != nil {
                    outcomes[idx] = Outcome{
                        Success:   false,
                        ErrorType: "internal_error",
                    }
                }
            }()
            outcomes[idx] = e.executeNode(ctx, ch, parent)
        }(i, child)
    }
    wg.Wait()

    return aggregateParallelOutcomes(outcomes)
}

func aggregateParallelOutcomes(outcomes []Outcome) Outcome {
    agg := Outcome{Success: true, StatusCode: 200}
    var maxLatency time.Duration
    for _, o := range outcomes {
        if !o.Success {
            agg.Success = false
            if agg.ErrorType == "" {
                agg.ErrorType = o.ErrorType
                agg.StatusCode = o.StatusCode
            }
        }
        if o.Latency > maxLatency {
            maxLatency = o.Latency
        }
    }
    agg.Latency = maxLatency
    return agg
}
```

Go 1.22+ なら for-range capture が改善されているので `i, child := i, child` の shadow は不要だが保険として書く。

### 2.2 Engine の thread-safety (NFR-U2-3)

- `*Engine` struct は構築後 read-only (rand は mutex 保護)
- 複数 goroutine が同じ `*Engine.Execute(ctx, plan)` を並行呼び出し可
- Plan / overlay / schema / synth は all thread-safe な前提

---

## 3. Error パターン

### 3.1 Panic recovery の二段防御 (Q4=A)

```go
// Top-level recovery in Execute
func (e *engineImpl) Execute(ctx context.Context, plan *Plan) (err error) {
    if plan == nil {
        return &ExecuteError{Kind: "nil_plan"}
    }
    if ctx == nil {
        return &ExecuteError{Kind: "nil_ctx"}
    }
    defer func() {
        if r := recover(); r != nil {
            err = &ExecuteError{
                Kind:  "internal",
                Inner: fmt.Errorf("panic during Execute: %v", r),
            }
        }
    }()
    
    // ... execute plan tree ...
    _ = e.executeNode(ctx, plan.Root, nil)
    return nil
}

// Per-parallel-child recovery (in executeParallelGroup, see §2.1)
```

二段防御:
1. **Execute 最上位**: 同期実行中の panic を `*ExecuteError` として返す
2. **Parallel child goroutine 内**: child の panic を main goroutine の Execute に伝播させない (cross-goroutine panic で main goroutine 全体の defer recover が効かないため)

### 3.2 Panic message + Outcome

- 最上位 recover: `ExecuteError.Inner` に panic 値を fmt.Errorf で wrap
- parallel child recover: その child の Outcome に `"internal_error"` を set
- log message は OTel Logger 経由で emit (synth.EmitLog)

### 3.3 Error 型: PlanError / ExecuteError

```go
type PlanError struct {
    Kind  string
    Path  []string
    Inner error
}

func (e *PlanError) Error() string {
    if len(e.Path) == 0 {
        return fmt.Sprintf("journey: BuildPlan: %s: %v", e.Kind, e.Inner)
    }
    return fmt.Sprintf("journey: BuildPlan: %s at %s: %v",
        e.Kind, strings.Join(e.Path, "→"), e.Inner)
}

func (e *PlanError) Unwrap() error { return e.Inner }

type ExecuteError struct {
    Kind  string
    Inner error
}

func (e *ExecuteError) Error() string {
    if e.Inner != nil {
        return fmt.Sprintf("journey: Execute: %s: %v", e.Kind, e.Inner)
    }
    return fmt.Sprintf("journey: Execute: %s", e.Kind)
}

func (e *ExecuteError) Unwrap() error { return e.Inner }
```

---

## 4. Fault Overlay Adapter (Q5 confirmed via U1 reality)

### 4.1 U1 actual API

```go
// In topology/faults.go:
func (o *FaultOverlay) NodeFaults(svc *Service) []FaultSpec      // service-targeted
func (o *FaultOverlay) OperationFaults(op *Operation) []FaultSpec // operation-targeted
func (o *FaultOverlay) EdgeFaults(edge *Edge) []FaultSpec         // edge-targeted
```

各 method は `[]FaultSpec` を返す (複数 fault 共存可)。

### 4.2 U2 内 adapter (`journey/fault.go`)

```go
// foldFaults returns the effective fault state for a single Node by
// scanning topology.FaultOverlay through the three lookup methods and
// applying Q8=A precedence (crash > disconnect > error_rate > latency_inflation).
type foldedFault struct {
    crashed         bool
    disconnected    bool
    errorRate       float64
    errorType       string // for error_rate_override
    latencyInflate  time.Duration
}

func (e *engineImpl) foldFaults(node *Node) foldedFault {
    var ff foldedFault
    
    // 1. Service-level (crash)
    for _, fs := range e.overlay.NodeFaults(node.Service) {
        switch fs.Kind {
        case topology.FaultCrash:
            ff.crashed = true
        case topology.FaultLatencyInflation:
            ff.latencyInflate += sampleInflation(fs)
        }
    }
    
    // 2. Edge-level (disconnect)
    if node.Edge != nil {
        for _, fs := range e.overlay.EdgeFaults(node.Edge) {
            switch fs.Kind {
            case topology.FaultDisconnect:
                ff.disconnected = true
            case topology.FaultLatencyInflation:
                ff.latencyInflate += sampleInflation(fs)
            }
        }
    }
    
    // 3. Operation-level (error_rate, latency)
    if op := node.Service.Operations[node.Operation]; op != nil {
        for _, fs := range e.overlay.OperationFaults(op) {
            switch fs.Kind {
            case topology.FaultErrorRateOverride:
                ff.errorRate = fs.Severity.Rate
                ff.errorType = fs.Severity.ErrorType
            case topology.FaultLatencyInflation:
                ff.latencyInflate += sampleInflation(fs)
            }
        }
    }
    
    return ff
}
```

NFR-D 確定事項:
- adapter は U2 内に閉じる (`journey/fault.go`)
- U1 API は変更しない
- `FaultSpec.Kind` の列挙は U1 (`topology.FaultKind`) を参照

`sampleInflation` の実装は `FaultSpec.Severity` の構造に依存 (U1 実装を参照、Code Generation Phase で精緻化)。

### 4.3 fault precedence (FD Q8=A)

`foldedFault` を組み立てた後、適用順 (executor.go 内):
1. `crashed` → 即時 Outcome (Success=false, ErrorType="crashed")、子 skip
2. `disconnected` → 即時 Outcome (Success=false, ErrorType="connection_refused")
3. `rand.Float64() < errorRate` → primary failure 強制 (ErrorType=ff.errorType or default)
4. `latencyInflate` → effectiveLatency に加算 (常に適用)

---

## 5. baseLatency (Q6 = A confirmed via U1 reality)

### 5.1 U1 actual: Edge.LatencyDist

```go
// In topology/types.go:
type Edge struct {
    // ...
    Latency LatencyDist  // base latency distribution for this edge
}

type LatencyDist struct {
    Distribution string         // "lognormal" | "fixed" | ...
    P50          time.Duration
    P95          time.Duration
}
```

base latency は **Edge に紐づく** (Operation ではない)。Edge.Latency から sample する。

### 5.2 sampling 関数

```go
// sampleEdgeLatency returns a base latency value sampled from an edge's
// LatencyDist. Distribution defaults to "fixed" (returns P50) when
// unspecified.
func (e *engineImpl) sampleEdgeLatency(edge *topology.Edge) time.Duration {
    if edge == nil {
        return defaultEntryLatency  // for journey root with no inbound edge
    }
    dist := edge.Latency
    switch dist.Distribution {
    case "", "fixed":
        return dist.P50
    case "lognormal":
        return e.sampleLognormal(dist.P50, dist.P95)
    case "uniform":
        return e.sampleUniform(dist.P50, dist.P95)
    default:
        return dist.P50  // fallback
    }
}

const defaultEntryLatency = 10 * time.Millisecond
```

Distribution string の値域は U1 で固定 (`topology` パッケージで const として定義済 or 確認)。

### 5.3 entry node (Edge==nil) の latency

journey root の node には Edge がない (incoming side from "external"). entry latency は固定 default (`10ms`) を採用。将来 `Operation.EntryLatency` 等を topology に追加するか、`Service.EntryLatency` を持つかは U1 拡張議論 (本 unit では default で進む)。

---

## 6. Cascade child span 表現 (Q8=A)

### 6.1 emit pattern

```go
func (e *engineImpl) emitCascadeSpan(ctx context.Context, node *Node, outcome Outcome) {
    spanIn := synth.SpanInput{
        Service:     node.Service,
        Edge:        node.Edge,
        Operation:   node.Operation,
        StartTime:   time.Now(),       // approx; cascade has near-zero duration
        InstanceIdx: 0,                // arbitrary; not executed
    }
    ctxSpan, finishFn := e.synth.BeginSpan(ctx, spanIn)
    // Set custom attribute via outcome → synth.SetAttributes inside finishFn
    // (synth's finishFn already sets error.type, status, etc.)
    finishFn(synth.Outcome{
        Success:    false,
        StatusCode: outcome.StatusCode,
        ErrorType:  outcome.ErrorType,
        EndTime:    time.Now(),
    })
    _ = ctxSpan
}
```

#### 6.1.1 `synth.cascaded=true` 属性

NFR-D で確定する追加事項: `outcome.Cascaded=true` の場合、`synth.SpanInput` に "Cascaded marker" を渡す方法が必要。

**選択肢 A**: SpanInput に `Cascaded bool` field を追加し、synth 側が custom attribute `synth.cascaded=true` を span に付与する
**選択肢 B**: U2 内で span 開始後 `synth` API を通さず直接 attribute を set (現状 API では不可能)
**選択肢 C**: outcome を渡す finishFn のシグネチャに hint を込める (Outcome に Cascaded を含めば synth が attribute 化)

→ **選択肢 C** が最も clean: `synth.Outcome` に何らかの "cascaded" hint を渡す方法を U3 と coordinate (NFR Design では C を採用案として明示)。具体的に:

```go
// synth.Outcome に Cascaded bool field を追加することを U3 へ提案
// (FD §10 で U3-to-U2 contract に追加する)
type synth.Outcome struct {
    Success    bool
    StatusCode int
    ErrorType  string
    EndTime    time.Time
    Cascaded   bool   // NEW (U2 → U3 coordination)
}
```

U3 側で `Cascaded=true` の場合に `synth.cascaded=true` attribute を span に追加する処理を NFR Design 改訂で追加。**U2 Code Generation Phase で synth.Outcome 拡張を確認**。

> **NOTE (U3 coordination)**: synth.Outcome への field 追加は SemVer minor bump で OK (NFR-U2-1 → NFR-U3-1 と整合)。NFR Design レベルで U3 docs にも追記が必要。Code Generation Plan で Phase として明示。

### 6.2 Sleep skip

cascade child は実行する必要がない (parent failed):
- Sleep 呼び出しなし
- 子 node の executeNode 呼び出しなし
- duration ≈ 0 (BeginSpan の StartTime と finishFn の EndTime がほぼ同じ)

---

## 7. ctx cancellation (NFR-U2-4)

### 7.1 sleep 中の cancel

```go
select {
case <-ctx.Done():
    outcome.Success = false
    outcome.ErrorType = "context_canceled"
    outcome.StatusCode = 0
    outcome.Latency = time.Since(startTime)
    return outcome
case <-time.After(effectiveLatency):
    // proceed
}
```

`time.After` は new channel を毎回作るので allocation あり; bench で気になれば `time.NewTimer` + 明示 Stop パターンに切り替え (NFR Design 改訂候補)。

### 7.2 cancel 後の children

ctx.Done() で current step が `context_canceled` outcome を返した場合、parent の executeNode は children を実行する前に cascade で skip するべきか?

→ **Yes, skip via cascade**: parent.Success=false (ErrorType="context_canceled") を見て child は cascade skip。span emit は emit (visibility のため)。

### 7.3 < 10 ms 中断保証

`time.After` の wakeup は 1-5ms 精度、ctx.Done channel 即時 signal、cascade skip の overhead < 100µs/step × 残り step 数。typical journey 残り step ≤ 10 で合計 < 10 ms 達成。

---

## 8. Stateful PBT 不採用 (Q9=A)

`pbt_test.go` で TP-U2-1〜5 を実装。PBT-06 (stateful) は初期実装では採用せず、TP-U2-3 (Cascade conditional) の通常 PBT で代用。

将来 cascade chain の deep test 必要性が判明したら `rapid.Run` ベースで追加検討。

---

## 9. Test 構造 (Q10=A)

### 9.1 `helpers_test.go`

```go
type mockSynth struct {
    mu       sync.Mutex
    spans    []spanCall
    metrics  []metricCall
    logs     []logCall
}

func newMockSynth() *mockSynth { ... }
func (m *mockSynth) BeginSpan(ctx, in) (ctx, finishFn) { ... record + return no-op finish ... }
func (m *mockSynth) RecordMetric(ctx, in) { ... }
func (m *mockSynth) EmitLog(ctx, in) { ... }

func newTestSchema(t *testing.T, opts ...schemaOption) *topology.Schema {
    // construct a minimal Schema via topology APIs
}

func newTestOverlay(t *testing.T, faults ...topology.FaultSpec) *topology.FaultOverlay {
    // construct a FaultOverlay with provided faults
}

func assertOutcomeMatches(t *testing.T, got, want Outcome) {
    t.Helper()
    // compare with explicit field-by-field, including recovery tracking
}
```

### 9.2 PBT (TP-U2-1..5)

`pbt_test.go` に集約。各 TP は独立 `rapid.Check` 呼び出し。`testutil/generators/` の `ValidPlan` / `ValidEngineOutcome` を活用。

### 9.3 Integration test cascade verification (Q11=A)

```go
// In journey/integration/integration_test.go (//go:build integration)
func TestIntegration_CascadePropagation(t *testing.T) {
    // 1. Construct topology with:
    //    - Service A → Service B → Service C
    //    - Edge A→B has OnFailure with no Fallback, OnExhausted=propagate
    //    - Service B has FaultCrash with Probability=1.0
    // 2. Start real Collector + real Pipeline
    // 3. Build Engine + Execute journey
    // 4. Read Collector JSON
    // 5. Assert:
    //    - 3 spans emitted (A, B, C) sharing same trace_id
    //    - Span A: Status=Error (cascade propagated from B)
    //    - Span B: Status=Error (Crashed)
    //    - Span C: Status=Error, attribute synth.cascaded=true
}
```

---

## 10. ファイル分割 (Q12=A)

8 production + 6 test (+ helpers/doc/bench):

```text
journey/
├── doc.go
├── engine.go         # Engine struct + NewEngine + ListJourneys
├── plan.go           # Plan / Node + BuildPlan (DFS)
├── executor.go       # Execute + executeNode + executeParallelGroup + ctx cancel handling
├── recovery.go       # Fallback chain + OnExhausted 3 mode + applyRecovery
├── fault.go          # foldFaults adapter (U1 FaultOverlay 3 methods → foldedFault)
├── replica.go        # replica selection (Q9=A per-step random)
├── errors.go         # PlanError + ExecuteError + AllowedErrorTypes const
│
├── engine_test.go
├── plan_test.go
├── executor_test.go
├── recovery_test.go
├── fault_test.go
├── pbt_test.go
├── helpers_test.go   # mockSynth + newTestSchema + assertOutcomeMatches
├── doc_test.go       # 3 Example functions
├── bench_test.go     # BenchmarkBuildPlan + BenchmarkExecute_PureOverhead
└── integration/      # //go:build integration
    ├── docker-compose.yaml
    ├── collector-config.yaml
    ├── helpers.go
    └── integration_test.go
```

---

## 11. NFR-R Open Questions の解消

| Open Question | 確定 |
|---|---|
| `*rand.Rand` mutex 競合 | §1.2: baseline は per-Engine + Mutex、bench で再評価 |
| Stateful PBT (PBT-06) | §8: 採用しない、TP-U2-3 で代用 |
| error_rate_override の ErrorType field | tech-stack §9: future revisit |
| Replica weighted / sticky | tech-stack §9: future revisit |
| latency_inflation の jitter | sampleInflation 内で実装可、U1 FaultSpec.Severity に jitter param あれば適用 |

---

## 12. NFR-R 各項目との対応

| NFR-R | 対応する Design パターン |
|---|---|
| NFR-U2-1 (API stability) | §3.3 error 型階層、§10 ファイル分割で internal/external 区別 |
| NFR-U2-2 (Engine lifecycle) | §1.5 eager BuildPlan in NewEngine |
| NFR-U2-3 (Concurrency) | §1.2 random source、§2 parallel pattern、Plan immutability |
| NFR-U2-4 (Context cancellation) | §7 sleep select pattern |
| NFR-U2-5 (Panic recovery) | §3.1 二段防御 |
| NFR-U2-6 (Performance) | §1 全体、§2.1 parallel |
| NFR-U2-7 (Observability) | self-metric なし (struct 確認) |
| NFR-U2-8 (Documentation) | §10 doc_test.go の 3 Examples |
| NFR-U2-9 (Testability) | §9 全体 + integration |
| NFR-U2-10 (Compatibility) | math/rand/v2 採用、topology / synth 依存 |
| NFR-U2-11 (PBT compliance) | §9.2 |

---

## 13. Anti-patterns (採用しない)

| アンチパターン | 不採用理由 |
|---|---|
| explicit stack で executeNode (Q2 案 B) | readability 劣化、深さ問題なし |
| goroutine-per-node CSP (Q2 案 C) | overhead 大 |
| executeNode の各 step で defer recover (Q4 案 B) | overhead 多、最上位 + parallel child の二段で十分 |
| overlay 汎用 Lookup (Q5 案 B) | O(N) scan のリスク、明示 method の方が efficient |
| overlay.LookupBaseLatency 統合 (Q6 案 B) | U1 actual は Edge.LatencyDist、責務違反 |
| Outcome builder helper explosion (Q7 案 B) | local var 逐次更新で十分 |
| Mutable shared Outcome across goroutines (Q7 案 C) | race danger |
| Cascade span を emit しない (Q8 案 B) | trace で何が起きたか分からなくなる |
| Stateful PBT (Q9 案 B) | 初期実装複雑度回避 |
| helpers_test.go 分割 (Q10 案 C) | 過剰 |
| executor.go を seq/parallel/cascade に分割 (Q12 案 C) | 共通 helper の重複、結合度高い |

---

## 14. U3 coordination (cascade attribute)

U2 cascade span の `synth.cascaded=true` 表現のため、**U3 `synth.Outcome` に `Cascaded bool` field を追加** を coordinate する必要あり。

- 影響: U3 FD `domain-entities.md` §1.2、U3 NFR-D `nfr-design-patterns.md` §4 (Outcome handling)
- SemVer: minor bump (field 追加、既存 callers は default false で動作)
- 実装: U3 が `synth.cascaded=true` attribute を span に書く、U2 が `Outcome.Cascaded` を true で渡す
- 確定 timing: U2 Code Generation Plan で `Phase X — Update synth.Outcome to support Cascaded marker` を含む (U3 へ patch を入れる Phase)

または **U2 内で完結する** alternative:
- U2 が synth.BeginSpan の SpanInput に attribute hint を渡す (例えば `synth.SpanInput.ExtraAttributes`)
- これも synth API 拡張を要するため同様 minor bump

**決定**: NFR Design Phase で U3 と coordinate して **synth.Outcome に `Cascaded bool` 追加** を実施。代替案は将来検討。
