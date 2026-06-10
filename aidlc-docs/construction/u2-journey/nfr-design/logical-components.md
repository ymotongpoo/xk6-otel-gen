# U2 journey — Logical Components

本書は `journey/` 内の **論理コンポーネント** (LC) を確定する。各 LC について 責務 / 公開 API / 実装スケッチ / 依存関係 を定義。

参照:
- FD: `aidlc-docs/construction/u2-journey/functional-design/`
- NFR Design Patterns: `nfr-design-patterns.md` (本ディレクトリ内)

---

## コンポーネント一覧

| LC | 名前 | ファイル | 責務 |
|---|---|---|---|
| LC-0 | Package Documentation | `doc.go` | パッケージレベル GoDoc |
| LC-1 | Engine | `engine.go` | Engine struct + NewEngine + ListJourneys |
| LC-2 | Plan Builder | `plan.go` | Plan / Node types + BuildPlan (DFS) |
| LC-3 | Executor | `executor.go` | Execute + executeNode + executeParallelGroup + ctx cancel + panic recovery |
| LC-4 | Recovery | `recovery.go` | Fallback chain + OnExhausted 3 mode + applyRecovery |
| LC-5 | Fault Adapter | `fault.go` | foldFaults (U1 FaultOverlay 3 method → foldedFault) + sampleInflation |
| LC-6 | Replica Selector | `replica.go` | randIntN(svc.Replicas) per step |
| LC-7 | Errors & Constants | `errors.go` | PlanError + ExecuteError + AllowedErrorTypes const |

---

## LC-0: Package Documentation (`doc.go`)

### 責務
- パッケージ全体の GoDoc
- Journey lifecycle 説明
- Cascade / Recovery / Fault の semantics

### 実装スケッチ
```go
// Package journey orchestrates the execution of topology-defined journeys,
// driving signal synthesis through a synth.Synthesizer and applying faults
// from a topology.FaultOverlay.
//
// Usage:
//
//   eng := journey.NewEngine(schema, overlay, syn)
//   plan, _ := eng.BuildPlan("checkout-flow")
//   for each VU iteration:
//       eng.Execute(ctx, plan)
//
// Cascade: a failed parent step forces its children to skip execution; the
// children's spans are still emitted with synth.cascaded=true for trace
// visibility.
//
// Recovery: failed primary edges traverse the OnFailure.Fallback chain
// sequentially; if all fallbacks fail, OnExhausted selects one of
// {propagate, return_default, succeed_silently}.
//
// Faults: crash > disconnect > error_rate_override > latency_inflation,
// applied in that precedence; multiple faults may coexist (overlay).
package journey
```

### 依存
- なし

---

## LC-1: Engine (`engine.go`)

### 責務
- `Engine` 構造体 (opaque public)
- `NewEngine(schema, overlay, syn) *Engine`
- `(*Engine).ListJourneys() []string`
- `engineImpl` 内部型 (実装)

### 公開 API
```go
type Engine struct { /* unexported */ }

func NewEngine(schema *topology.Schema, overlay *topology.FaultOverlay, syn synth.Synthesizer) *Engine
func (e *Engine) ListJourneys() []string
```

### 実装スケッチ
NFR Design Patterns §1.1 + §1.5 参照。eager BuildPlan、Plan map、rand source 初期化。

```go
type engineImpl struct {
    schema  *topology.Schema
    overlay *topology.FaultOverlay
    synth   synth.Synthesizer

    plans       map[string]*Plan
    journeyKeys []string // sorted

    rand *rand.Rand
    rmu  sync.Mutex
}

func NewEngine(schema *topology.Schema, overlay *topology.FaultOverlay, syn synth.Synthesizer) *Engine {
    if schema == nil { panic("journey: NewEngine: schema must not be nil") }
    if overlay == nil { panic("journey: NewEngine: overlay must not be nil") }
    if syn == nil { panic("journey: NewEngine: syn must not be nil") }

    e := &engineImpl{
        schema:  schema,
        overlay: overlay,
        synth:   syn,
        plans:   make(map[string]*Plan, len(schema.Journeys)),
        rand:    newDefaultRand(),
    }
    for name := range schema.Journeys {
        plan, err := e.buildPlan(name)
        if err != nil {
            panic(fmt.Sprintf("journey: NewEngine: build %q: %v", name, err))
        }
        e.plans[name] = plan
        e.journeyKeys = append(e.journeyKeys, name)
    }
    sort.Strings(e.journeyKeys)
    return (*Engine)(unsafe.Pointer(e))  // or wrap engineImpl as field of Engine
}

func (e *Engine) ListJourneys() []string {
    impl := (*engineImpl)(unsafe.Pointer(e))
    out := make([]string, len(impl.journeyKeys))
    copy(out, impl.journeyKeys)
    return out
}
```

`unsafe.Pointer` cast は public/private 分離のための一案。alternative:
```go
type Engine struct {
    impl *engineImpl
}
func (e *Engine) ListJourneys() []string { return e.impl.listJourneys() }
```

(Code Generation Plan で確定、`unsafe` を避ける方向で)

### 依存
- LC-2 (`buildPlan`)
- LC-7 (errors)
- `topology`, `synth`, `sync`, `sort`, `math/rand/v2`

---

## LC-2: Plan Builder (`plan.go`)

### 責務
- `Plan` / `Node` 型 (FD §1.2, §1.3)
- `(*Engine).BuildPlan(name) (*Plan, error)` — cache lookup
- `(*engineImpl).buildPlan(name) (*Plan, error)` — DFS 構築

### 公開 API
```go
type Plan struct {
    JourneyName string
    Root        *Node
}

type Node struct {
    Service   *topology.Service
    Operation string
    Edge      *topology.Edge
    Parallel  []*Node
    Children  []*Node
}

func (e *Engine) BuildPlan(journeyName string) (*Plan, error)
```

### 実装スケッチ
```go
func (e *Engine) BuildPlan(journeyName string) (*Plan, error) {
    impl := e.impl
    plan, ok := impl.plans[journeyName]
    if !ok {
        return nil, &PlanError{Kind: "unknown_journey", Path: []string{journeyName}}
    }
    return plan, nil
}

func (e *engineImpl) buildPlan(journeyName string) (*Plan, error) {
    j, ok := e.schema.Journeys[journeyName]
    if !ok {
        return nil, &PlanError{Kind: "unknown_journey", Path: []string{journeyName}}
    }
    if len(j.Steps) == 0 {
        return nil, &PlanError{Kind: "empty_journey", Path: []string{journeyName}}
    }

    // root: aggregate all steps under a virtual or single root
    rootNode := &Node{}
    for _, step := range j.Steps {
        child, err := e.buildStepNode(step, nil, journeyName)
        if err != nil {
            return nil, err
        }
        rootNode.Children = append(rootNode.Children, child)
    }
    if len(rootNode.Children) == 1 {
        // single-step journey: use it as root directly
        return &Plan{JourneyName: journeyName, Root: rootNode.Children[0]}, nil
    }
    return &Plan{JourneyName: journeyName, Root: rootNode}, nil
}

func (e *engineImpl) buildStepNode(step *topology.Step, parentEdge *topology.Edge, path string) (*Node, error) {
    if step.Parallel != nil {
        n := &Node{}
        for _, sub := range step.Parallel {
            c, err := e.buildStepNode(sub, parentEdge, path)
            if err != nil { return nil, err }
            n.Parallel = append(n.Parallel, c)
        }
        return n, nil
    }
    if step.Op == nil {
        return nil, &PlanError{Kind: "nil_op", Path: []string{path}}
    }
    node := &Node{
        Service:   step.Op.Service,
        Operation: step.Op.Name,
        Edge:      parentEdge,
    }
    for _, call := range step.Op.Calls {
        child, err := e.buildCallNode(call, path)
        if err != nil { return nil, err }
        node.Children = append(node.Children, child)
    }
    return node, nil
}
```

(buildCallNode の詳細は U1 Operation.Calls の actual shape 次第。Code Generation Plan で確定)

### 依存
- LC-7 (PlanError)
- `topology`

---

## LC-3: Executor (`executor.go`)

### 責務
- `(*Engine).Execute(ctx, plan) error`
- `executeNode(ctx, node, parent)` — 再帰
- `executeParallelGroup(ctx, group, parent)` — sync.WaitGroup 並列
- ctx cancel handling
- panic recovery (NFR-U2-5 二段防御)
- Synthesizer 呼び出し順序 (BeginSpan → children → finishFn → RecordMetric → EmitLog)

### 公開 API
```go
func (e *Engine) Execute(ctx context.Context, plan *Plan) error
```

### 実装スケッチ
NFR Design Patterns §1.4, §2, §3, §7 を参照。

```go
func (e *Engine) Execute(ctx context.Context, plan *Plan) (err error) {
    if plan == nil { return &ExecuteError{Kind: "nil_plan"} }
    if ctx == nil { return &ExecuteError{Kind: "nil_ctx"} }
    defer func() {
        if r := recover(); r != nil {
            err = &ExecuteError{Kind: "internal", Inner: fmt.Errorf("panic: %v", r)}
        }
    }()
    impl := e.impl
    _ = impl.executeNode(ctx, plan.Root, nil)
    return nil
}

func (e *engineImpl) executeNode(ctx context.Context, node *Node, parent *Outcome) Outcome {
    // virtual fan-out node?
    if node.Parallel != nil {
        return e.executeParallelGroup(ctx, node, parent)
    }

    // 0. cascade check
    if parent != nil && !parent.Success {
        return e.executeCascade(ctx, node, parent)
    }

    // 1. fault evaluation
    ff := e.foldFaults(node)
    if ff.crashed {
        return e.executeCrashed(ctx, node)
    }
    if ff.disconnected {
        return e.executeDisconnected(ctx, node)
    }
    forceFail := false
    if ff.errorRate > 0 && e.randFloat64() < ff.errorRate {
        forceFail = true
    }

    // 2. replica selection
    instanceIdx := e.randIntN(maxInt(node.Service.Replicas, 1))

    // 3. base latency + inflation
    baseLatency := e.sampleEdgeLatency(node.Edge)
    effectiveLatency := baseLatency + ff.latencyInflate

    // 4. BeginSpan
    startTime := time.Now()
    spanCtx, finishFn := e.synth.BeginSpan(ctx, synth.SpanInput{
        Service: node.Service, Edge: node.Edge,
        Operation: node.Operation, StartTime: startTime,
        InstanceIdx: instanceIdx,
    })

    // 5. sleep with ctx cancel
    sleepResult := waitWithCancel(ctx, effectiveLatency)
    if sleepResult == ctxCanceled {
        endTime := time.Now()
        outcome := Outcome{
            Success:    false,
            ErrorType:  "context_canceled",
            StatusCode: 0,
            Latency:    endTime.Sub(startTime),
        }
        finishFn(toSynthOutcome(outcome, endTime))
        return outcome
    }

    // 6. children traversal
    var primaryFailed bool
    if forceFail {
        primaryFailed = true
    }
    selfOutcome := Outcome{Success: !primaryFailed, StatusCode: pickStatusCode(node, forceFail, ff.errorType)}
    if primaryFailed {
        selfOutcome.ErrorType = ff.errorType
        if selfOutcome.ErrorType == "" {
            selfOutcome.ErrorType = "http.500" // default
        }
    }

    for _, child := range node.Children {
        _ = e.executeNode(spanCtx, child, &selfOutcome)
    }

    // 7. apply recovery if primary failed and OnFailure exists
    if primaryFailed && node.Edge != nil && node.Edge.OnFailure != nil {
        selfOutcome = e.applyRecovery(ctx, node, selfOutcome)
    }

    // 8. finalize Outcome
    endTime := time.Now()
    selfOutcome.Latency = endTime.Sub(startTime)

    // 9. finishFn + Record + Emit
    finishFn(toSynthOutcome(selfOutcome, endTime))
    e.synth.RecordMetric(spanCtx, synth.MetricInput{...})
    e.synth.EmitLog(spanCtx, synth.LogInput{...})

    return selfOutcome
}
```

### 依存
- LC-2 (Plan, Node)
- LC-4 (applyRecovery)
- LC-5 (foldFaults, sampleInflation)
- LC-6 (randIntN)
- LC-7 (ExecuteError)
- `synth`, `sync`, `time`, `context`

---

## LC-4: Recovery (`recovery.go`)

### 責務
- `applyRecovery(ctx, node, primaryOutcome) Outcome`
- Fallback chain 逐次実行
- OnExhausted 3 mode の処理

### 実装スケッチ
```go
func (e *engineImpl) applyRecovery(ctx context.Context, node *Node, primary Outcome) Outcome {
    policy := node.Edge.OnFailure
    out := primary
    out.PrimaryFailed = true

    for _, fbEdge := range policy.Fallback {
        fbOutcome := e.executeFallback(ctx, node, fbEdge)
        out.FallbackAttempts = append(out.FallbackAttempts, fbEdge)
        if fbOutcome.Success {
            out.Success = true
            out.FallbackUsed = fbEdge
            out.ErrorType = ""
            out.StatusCode = fbOutcome.StatusCode
            return out
        }
    }

    switch policy.OnExhausted {
    case topology.ExhaustedPropagate:
        // out.Success remains false; cascade will be re-evaluated by parent
        return out
    case topology.ExhaustedReturnDefault:
        out.Success = true
        out.DefaultUsed = true
        out.ErrorType = ""
        out.StatusCode = 200
        return out
    case topology.ExhaustedSucceedSilently:
        out.Success = true
        out.SilentlySucceeded = true
        out.ErrorType = ""
        out.StatusCode = 200
        return out
    }
    return out
}

func (e *engineImpl) executeFallback(ctx context.Context, fromNode *Node, fbEdge *topology.Edge) Outcome {
    // build a "fallback node" on the fly
    fbNode := &Node{
        Service:   fbEdge.To.Service,   // (or however Edge.To resolves)
        Operation: fbEdge.To.Name,
        Edge:      fbEdge,
    }
    return e.executeNode(ctx, fbNode, nil)
}
```

### 依存
- LC-3 (executeNode 経由で fallback も実行)
- `topology` (ExhaustedAction const)

---

## LC-5: Fault Adapter (`fault.go`)

### 責務
- `foldFaults(node) foldedFault` — U1 FaultOverlay 3 method を adapter で fold
- `sampleInflation(spec) time.Duration` — fault severity から sample
- `sampleEdgeLatency(edge) time.Duration` — Edge.LatencyDist から sample

### 実装スケッチ
NFR Design Patterns §4 + §5 参照。

```go
type foldedFault struct {
    crashed        bool
    disconnected   bool
    errorRate      float64
    errorType      string
    latencyInflate time.Duration
}

func (e *engineImpl) foldFaults(node *Node) foldedFault {
    var ff foldedFault
    // ... NodeFaults / EdgeFaults / OperationFaults をスキャン、precedence 適用 ...
    return ff
}

func (e *engineImpl) sampleInflation(spec topology.FaultSpec) time.Duration {
    // spec.Severity (U1 FaultSpec の actual shape) から jitter 含めて sample
    // Code Generation Plan で U1 の Severity field shape を確定
    return spec.Severity.Delay // placeholder
}

func (e *engineImpl) sampleEdgeLatency(edge *topology.Edge) time.Duration {
    if edge == nil {
        return defaultEntryLatency
    }
    switch edge.Latency.Distribution {
    case "", "fixed":
        return edge.Latency.P50
    case "lognormal":
        return e.sampleLognormal(edge.Latency.P50, edge.Latency.P95)
    case "uniform":
        return e.sampleUniform(edge.Latency.P50, edge.Latency.P95)
    default:
        return edge.Latency.P50
    }
}
```

### 依存
- LC-6 (rand)
- `topology` (FaultOverlay, FaultSpec, LatencyDist)
- `time`

---

## LC-6: Replica Selector (`replica.go`)

### 責務
- `randIntN(n int) int` (mutex 付き wrapper)
- `randFloat64() float64`
- `newDefaultRand() *rand.Rand` — PCG seed 構築

### 実装スケッチ
NFR Design Patterns §1.2 参照。

```go
func newDefaultRand() *rand.Rand {
    var seed1, seed2 uint64 = uint64(time.Now().UnixNano()), 0xdeadbeefcafebabe
    return rand.New(rand.NewPCG(seed1, seed2))
}

func (e *engineImpl) randIntN(n int) int {
    if n <= 1 { return 0 }
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

### 依存
- `math/rand/v2`
- `sync`, `time`

---

## LC-7: Errors & Constants (`errors.go`)

### 責務
- `PlanError`, `ExecuteError` 型 (FD `domain-entities.md` §1.6)
- `AllowedErrorTypes []string` const (FD §11)

### 実装スケッチ
```go
type PlanError struct {
    Kind  string
    Path  []string
    Inner error
}
func (e *PlanError) Error() string  { ... }
func (e *PlanError) Unwrap() error  { return e.Inner }

type ExecuteError struct {
    Kind  string
    Inner error
}
func (e *ExecuteError) Error() string  { ... }
func (e *ExecuteError) Unwrap() error  { return e.Inner }

var AllowedErrorTypes = []string{
    "timeout",
    "connection_refused",
    "dns_failure",
    "http.500", "http.502", "http.503", "http.504",
    "grpc.unavailable", "grpc.deadline_exceeded", "grpc.unauthenticated",
    "db.connection_lost", "db.constraint_violation",
    "crashed",
    "circuit_open",
    "rate_limited",
    "context_canceled",
}
```

### 依存
- `fmt`, `strings`

---

## コンポーネント間依存図

```text
              ┌──────────────────┐
              │ LC-0 doc.go      │
              └──────────────────┘

              ┌──────────────────┐
              │ LC-7 errors.go   │ ◄────────── (used by 1,2,3)
              │ - PlanError      │
              │ - ExecuteError   │
              │ - AllowedError-  │
              │   Types          │
              └──────────────────┘

              ┌──────────────────┐
              │ LC-6 replica.go  │ ◄────── (rand source)
              │ - randIntN       │
              │ - randFloat64    │
              │ - newDefaultRand │
              └──────────────────┘
                       ▲
                       │
              ┌──────────────────┐
              │ LC-5 fault.go    │
              │ - foldFaults     │
              │ - sampleInflation│
              │ - sampleEdge-    │
              │   Latency        │
              └────────┬─────────┘
                       │
              ┌────────▼─────────┐
              │ LC-4 recovery.go │
              │ - applyRecovery  │
              │ - executeFallback│
              └────────┬─────────┘
                       │
              ┌────────▼─────────┐
              │ LC-3 executor.go │
              │ - Execute        │
              │ - executeNode    │
              │ - executeParallel│
              │ - executeCascade │
              └────────┬─────────┘
                       │
              ┌────────▼─────────┐
              │ LC-2 plan.go     │
              │ - BuildPlan      │
              │ - buildPlan(int) │
              └────────┬─────────┘
                       │
              ┌────────▼─────────┐
              │ LC-1 engine.go   │
              │ - Engine (opaque)│
              │ - NewEngine      │
              │ - ListJourneys   │
              └──────────────────┘
```

---

## ビルド時の依存外部パッケージ

| 用途 | パッケージ |
|---|---|
| Topology types | `github.com/ymotongpoo/xk6-otel-gen/topology` |
| Synthesizer interface | `github.com/ymotongpoo/xk6-otel-gen/synth` |
| Random | `math/rand/v2` |
| Context | `context` |
| Sync primitives | `sync` |
| Time | `time` |
| Format | `fmt`, `strings`, `sort` |

**Excluded**: OTel SDK 直接 (synth 経由)、`golang.org/x/sync/errgroup` (sync.WaitGroup で十分)、`math/rand` v1.

---

## テストコンポーネント (Code Generation 時に詳細化)

| テストファイル | LC 対象 | テスト形式 |
|---|---|---|
| `engine_test.go` | LC-1 | example-based (NewEngine / ListJourneys / nil panic) |
| `plan_test.go` | LC-2 | example-based + TP-U2-1 (BuildPlan Idempotency), TP-U2-2 (all ops visited) |
| `executor_test.go` | LC-3 | example-based (sequential / parallel / ctx cancel / panic recovery) + TP-U2-5 (time monotonicity) |
| `recovery_test.go` | LC-4 | OnExhausted 3 mode の各々を test |
| `fault_test.go` | LC-5 | Fault precedence (Q8=A order) test |
| `pbt_test.go` | LC-3, LC-7 | TP-U2-3 (Cascade conditional), TP-U2-4 (error.type allowed) |
| `helpers_test.go` | (全 LC 共通) | mockSynth + newTestSchema + assertOutcomeMatches |
| `doc_test.go` | LC-0..LC-7 | 3 Example functions |
| `bench_test.go` | LC-2, LC-3 | BenchmarkBuildPlan / BenchmarkExecute_PureOverhead |
| `integration/integration_test.go` | LC-3 全体 (E2E) | `//go:build integration`、Docker Collector + cascade pattern verify |

---

## U3 coordination 要件 (cascade attribute)

`synth.Outcome` への `Cascaded bool` field 追加が U2 cascade span 表現に必要 (NFR Design Patterns §14)。

- 影響先: `synth/interface.go` (Outcome struct) + `synth/synthesizer.go` (finishFn 内で attr 追加)
- SemVer: minor bump (backward-compatible field 追加)
- Code Generation Plan に **Phase: Update synth.Outcome with Cascaded marker** を含む

---

## まとめ

- **8 production files** (FD §3 と完全一致)
- **9 test files** (helpers / doc / bench を含む) + integration/ subdir
- 各 LC は Single Responsibility
- 依存関係は単方向 (LC-0/7 → LC-6 → LC-5 → LC-4 → LC-3 → LC-2 → LC-1)
- 公開 API は FD §4 で確定済の最小セット
- U3 への coordination (`synth.Outcome.Cascaded`) は Code Generation Plan で明示 phase 化
- U1 FaultOverlay actual API (`NodeFaults` / `OperationFaults` / `EdgeFaults`) と adapter で連携、`Edge.Latency LatencyDist` から base latency sample
