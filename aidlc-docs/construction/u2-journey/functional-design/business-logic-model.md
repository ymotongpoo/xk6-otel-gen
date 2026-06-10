# U2 journey — Business Logic Model

本書は `journey/` パッケージのビジネスロジック (Engine の動作) を確定する。

参照: Application Design (`component-methods.md` §C2)、`plans/u2-journey-fd-plan.md` の Q1..Q14 回答。

---

## 1. パッケージの責務

`journey/` (`Engine`) は **Topology + FaultOverlay の情報を元に Journey を実行** し、各 step で `synth.Synthesizer` を呼び出して OTel signal を emit する。

### 1.1 入力

| 入力 | 提供者 | 内容 |
|---|---|---|
| `schema *topology.Schema` | k6 module (U5) | パース済 topology (Services / Edges / Operations / Journeys) |
| `overlay *topology.FaultOverlay` | k6 module (U5) | FaultSpec を service / operation / edge で lookup する辞書 |
| `synth synth.Synthesizer` | k6 module (U5) | OTel signal 合成 |
| `journeyName string` | JS API call (BuildPlan 引数) | 実行する Journey の名前 |
| `ctx context.Context` | k6 VU runtime (Execute 引数) | parent ctx (k6 iteration が deadline 持つ場合あり) |

### 1.2 出力

`Engine.Execute` は **error のみ返す**。telemetry は `synth` 経由で副作用として OTLP に流れる。

```text
k6 VU iteration:
    plan = engine.BuildPlan("checkout-flow")   ← 起動時 1 回 / VU で再利用
    for i := 0; i < N; i++ {
        err := engine.Execute(ctx, plan)
        // plan は immutable、複数回 / 並列 Execute 可能
    }
```

### 1.3 BuildPlan + Execute の分離

- **`BuildPlan(journeyName)`**: Topology から Plan (`*Plan`, immutable tree) を構築。1 回コスト
- **`Execute(ctx, plan)`**: Plan を走査して synth を呼ぶ。複数 VU 並列実行可
- **`ListJourneys()`**: Topology に定義された全 journey 名

---

## 2. BuildPlan の動作 (Q1=A DFS)

### 2.1 アルゴリズム

```text
BuildPlan(journeyName):
    journey := schema.Journeys[journeyName]
    if journey == nil:
        return ConfigError "unknown journey"
    
    rootSteps := journey.Steps  // []*Step
    root := buildNode(rootSteps[0])   // 簡略表記; 実際は複数 step の root 集約
    for i := 1 .. len(rootSteps)-1:
        appendChild(root, buildNode(rootSteps[i]))
    return &Plan{JourneyName: journeyName, Root: root}

buildNode(step *topology.Step):
    if step.Parallel != nil:    // step-level parallel group
        node := &Node{Service: nil, Parallel: []}   // virtual fan-out node
        for _, sub := range step.Parallel:
            node.Parallel = append(node.Parallel, buildNode(sub))
        return node
    
    node := &Node{
        Service: step.Op.Service,
        Operation: step.Op.Name,
        Edge: nil,               // root node: no inbound edge
    }
    expandCalls(node, step.Op)
    return node

expandCalls(parent *Node, op *topology.Operation):
    for _, call := range op.Calls:
        if call.Parallel != nil:    // operation-level parallel calls
            group := &Node{Service: nil, Parallel: []}
            for _, sub := range call.Parallel:
                child := makeChild(parent, sub.Edge, sub.Edge.To, sub.Edge.To.Operations[sub.OpName])
                expandCalls(child, sub.Edge.To.Operations[sub.OpName])
                group.Parallel = append(group.Parallel, child)
            parent.Children = append(parent.Children, group)
        else:
            child := &Node{
                Service: call.Edge.To,
                Operation: call.OpName,
                Edge: call.Edge,
            }
            expandCalls(child, call.Edge.To.Operations[call.OpName])
            parent.Children = append(parent.Children, child)
```

(疑似コード — 実装は再帰回避や cycle 検出等で refine、NFR Design 時に確定)

### 2.2 Cycle 検出

Topology は Schema.Validate で循環 edge を検出済 (U1 の責務)。BuildPlan は **cycle が起き得ない前提** で構築。万一発生したら panic ではなく `*PlanError{Kind: "cycle"}` を返す (defense in depth)。

### 2.3 Plan の immutability (Q2=A)

- `*Plan` / `*Node` 構築後の field 変更を禁止 (lint / convention)
- Engine field の全 Plan を `map[journeyName]*Plan` で保持
- 複数 goroutine から同じ Plan に対し `Execute(ctx, plan)` を呼ぶことが race-free

---

## 3. Execute の動作

### 3.1 全体フロー

```text
Execute(ctx, plan):
    rootCtx, rootFinish := synth.BeginSpan(ctx, spanInputFor(plan.Root))
    rootOutcome := executeNode(rootCtx, plan.Root, nil /* no parent outcome */)
    rootFinish(rootOutcome)
    synth.RecordMetric(rootCtx, metricInputFor(plan.Root, rootOutcome))
    synth.EmitLog(rootCtx, logInputFor(plan.Root, rootOutcome))
    return nil  // err only on programmer error / context cancellation
```

### 3.2 executeNode (再帰)

```text
executeNode(ctx, node, parentOutcome) Outcome:
    // 0. parent からの cascade 強制 (Q7=A)
    if parentOutcome != nil && shouldCascade(parentOutcome):
        return Outcome{Success: false, Cascaded: true, ErrorType: parentOutcome.ErrorType}
    
    // 1. instanceIdx selection (Q9=A per-step random)
    instanceIdx := rand.IntN(max(node.Service.Replicas, 1))
    
    // 2. parallel group の場合
    if node.Parallel != nil:
        return executeParallelGroup(ctx, node, parentOutcome)
    
    // 3. fault evaluation (Q8=A order: crash > disconnect > error_rate > latency_inflation)
    crash := overlay.LookupCrash(node.Service)
    if crash != nil:
        return Outcome{Success: false, Cascaded: false, ErrorType: "crashed", ...}
    
    disconnect := overlay.LookupDisconnect(node.Edge)
    if node.Edge != nil && disconnect != nil:
        return Outcome{Success: false, Cascaded: false, ErrorType: "connection_refused", ...}
    
    baseLatency := pickBaseLatency(node.Operation)  // Operation の base latency 仕様 (topology)
    inflation := overlay.LookupLatencyInflation(node)
    effectiveLatency := baseLatency + inflation
    
    errRate := overlay.LookupErrorRate(node)
    forceFailure := rand.Float64() < errRate
    
    // 4. start span (synth)
    startTime := time.Now()
    spanCtx, finishFn := synth.BeginSpan(ctx, SpanInput{
        Service: node.Service, Edge: node.Edge,
        Operation: node.Operation, StartTime: startTime,
        InstanceIdx: instanceIdx,
    })
    
    // 5. simulate latency (Q10=A real Sleep)
    select {
    case <-ctx.Done():
        finishFn(Outcome{Success: false, ErrorType: "context_canceled", EndTime: time.Now()})
        return Outcome{Success: false, ErrorType: "context_canceled"}
    case <-time.After(effectiveLatency):
    }
    
    // 6. 子 node の実行 (sequential or parallel, 自 outcome に応じて)
    childOutcomes := []Outcome{}
    var childFailed bool
    for _, child := range node.Children:
        co := executeNode(spanCtx, child, currentBest(forceFailure, parentOutcome))
        childOutcomes = append(childOutcomes, co)
        if !co.Success {
            childFailed = true
            break  // first child failure halts sequential children (depends on recovery)
        }
    
    // 7. local failure determination
    var primaryFailed bool
    if forceFailure || childFailed:
        primaryFailed = true
    
    // 8. recovery flow (if needed)
    outcome := assembleOutcome(node, primaryFailed, forceFailure, childOutcomes, effectiveLatency, startTime)
    if primaryFailed && node.Edge != nil && node.Edge.OnFailure != nil:
        outcome = applyRecovery(ctx, node, outcome)
    
    // 9. close span
    finishFn(toSynthOutcome(outcome))
    
    // 10. record metric + log
    synth.RecordMetric(spanCtx, MetricInput{...})
    synth.EmitLog(spanCtx, LogInput{...})
    
    return outcome
```

(再度疑似コード。実装は NFR Design で詳細化)

### 3.3 Parallel group の実行 (Q3=A)

```text
executeParallelGroup(parentCtx, group, parentOutcome) Outcome:
    var wg sync.WaitGroup
    outcomes := make([]Outcome, len(group.Parallel))
    for i, child := range group.Parallel:
        wg.Add(1)
        go func(idx int, ch *Node) {
            defer wg.Done()
            outcomes[idx] = executeNode(parentCtx, ch, parentOutcome)
        }(i, child)
    wg.Wait()
    
    // group の総合 outcome を計算 (any failure → group failure)
    aggregated := aggregateParallelOutcomes(outcomes)
    return aggregated
```

`parentCtx` は parent BeginSpan で得られた `ctx2` — Q4=C により全 child は **same parent_span_id を共有する兄弟 spans** となる。

### 3.4 Recovery flow (Q5=A 逐次評価)

```text
applyRecovery(ctx, node, primaryOutcome) Outcome:
    policy := node.Edge.OnFailure
    out := primaryOutcome
    out.PrimaryFailed = true
    
    // 1. fallback chain (sequential, Q5=A)
    for _, fbEdge := range policy.Fallback:
        // execute fallback as its own sub-step
        fbOutcome := executeFallback(ctx, fbEdge)
        out.FallbackAttempts = append(out.FallbackAttempts, fbEdge)
        if fbOutcome.Success:
            out.Success = true
            out.FallbackUsed = fbEdge
            out.ErrorType = ""
            out.StatusCode = fbOutcome.StatusCode
            return out
    
    // 2. all fallbacks exhausted → OnExhausted action
    switch policy.OnExhausted:
    case ExhaustedPropagate:
        // out.Success remains false, ErrorType remains the primary's
        // Cascade will be evaluated by parent
        return out
    case ExhaustedReturnDefault:
        out.Success = true
        out.DefaultUsed = true
        out.ErrorType = ""
        return out
    case ExhaustedSucceedSilently:
        out.Success = true
        out.SilentlySucceeded = true
        out.ErrorType = ""
        return out
    }
```

`executeFallback` は本質的に小さな `executeNode` 相当: own span 開始 → 自 outcome 確定 → finish。fallback span は parent span の子 (recovery branch を trace で視覚化可能)。

### 3.5 Cascade 判定 (Q7=A)

```text
shouldCascade(parentOutcome) bool:
    if parentOutcome.Success: return false
    // parent failure → child は本質的に実行不可
    // (parent が完全に死んでいる場合、その下流の call は意味がない)
    return true

// Edge.OnFailure == nil の Edge → default で propagate 扱い (Q7=A)
// recovery を試みず、即座に Outcome.Success=false を返し、親に伝える
```

Cascade flag (`Outcome.Cascaded`) は **parent からの強制終了** を表す。自分の primary failure では立てない (Q7=A 明示)。

---

## 4. Fault 適用 (Q8=A)

### 4.1 評価順 (mutually exclusive ではなく overlay)

```text
1. crash      → Service レベル、子も実行しない
2. disconnect → Edge レベル、子も実行しない (edge が通らない)
3. error_rate_override → Operation レベル、確率で primary failure 強制
4. latency_inflation   → Operation/Service レベル、latency に加算
```

複数 fault が同じ node に適用される可能性あり (例: latency_inflation + error_rate_override) → 両方順に適用。

### 4.2 Fault 仕様の参照

`*topology.FaultOverlay` の API (U1 既定):
```go
LookupCrash(svc *Service) *FaultSpec
LookupDisconnect(edge *Edge) *FaultSpec
LookupErrorRate(node nodeKey) float64
LookupLatencyInflation(node nodeKey) time.Duration
```

(具体的な API は U1 FaultOverlay の実装によるが、上記の概念 lookup が成立すれば良い)

### 4.3 Random seed

per-VU seed:
- k6 VU id をシード材料に使う (deterministic per VU)
- Engine インスタンスに `rand *rand.Rand` を持たせる、または call site で `rand.New(rand.NewSource(seed))` を使い回す
- NFR Design で確定 (deterministic test 用途のため重要)

---

## 5. Replica 選択 (Q9=A per-step random)

各 step (node 実行時) で `rand.IntN(max(svc.Replicas, 1))` を独立に draw して `InstanceIdx` を決定。

- 並列 child でも各自独立 draw
- Recovery の fallback でも各 fallback step が独立 draw
- per-step independent なため、同じ journey 内で同じ service の InstanceIdx が異なる場合あり (sticky session 模擬したいなら U2 拡張 or 別 unit)

---

## 6. 時間管理 (Q10=A)

### 6.1 Real time.Sleep

各 step の latency simulation:
```go
select {
case <-ctx.Done():
    // 中断
case <-time.After(effectiveLatency):
    // sleep 完了 → finishFunc 呼ぶ
}
```

`time.After` を使うことで `ctx.Done()` 同時待機可能 (`time.Sleep` だと cancel しない)。

### 6.2 StartTime / EndTime の計算

```go
startTime := time.Now()
// ... sleep ...
endTime := time.Now()
// 実際の挙動として StartTime + effectiveLatency ≈ EndTime (time.After の精度内)
```

U3 の `SpanInput.StartTime` には `startTime`、`Outcome.EndTime` には `endTime` を渡す。Fault による latency inflation はここで反映される (U3 から見ると transparent)。

### 6.3 effectiveLatency の組成

```text
effectiveLatency = baseLatency + faultInflation

baseLatency:
    Operation の topology 定義から得る (Operation.Latency が指定されていれば、無ければデフォルト 10ms)
faultInflation:
    overlay.LookupLatencyInflation(node) の値 (fixed Duration、または multiplier × base)
```

NFR Design で `baseLatency` の取得 API を確定。

---

## 7. Synthesizer 呼び出し順序

```text
For each node (in execution order):
    1. synth.BeginSpan(ctx, spanInputFor(node))  → ctx2, finishFn
    2. (sleep / children / fault eval)
    3. finishFn(outcome)
    4. synth.RecordMetric(ctx2, metricInputFor(node, outcome))
    5. synth.EmitLog(ctx2, logInputFor(node, outcome))
```

- `BeginSpan` → `finishFn` の lifecycle は必ず完了 (panic 時も `defer finishFn(...)` を考慮)
- RecordMetric / EmitLog は span の **終了後** に呼ぶ (span context は ctx2 経由で trace_id 共有される)

---

## 8. Engine の Concurrency Model

### 8.1 Engine instance の共有

`*Engine` は **全 VU で共有** される (k6 init phase で 1 個構築、各 VU が同じインスタンスを使う):
- Plan は immutable map → race-free read
- `synth.Synthesizer` も thread-safe (U3 で保証)
- `*topology.Schema` / `*topology.FaultOverlay` も immutable (U1 で構築後変更されない)
- `rand` 等の擬似乱数 generator は **per-VU instance** または **goroutine-local** にする (NFR Design で確定)

### 8.2 Execute の並列実行

```text
k6 VU 0 -> Execute(ctx0, plan)
k6 VU 1 -> Execute(ctx1, plan)  // 同じ plan instance
...
```

→ Engine 内部状態は read-only、Plan は read-only、synth は thread-safe → race-free。

### 8.3 Context Cancellation

- `ctx.Done()` で実行中の step を即時中断 (sleep を抜ける)
- 中断時の outcome: `Success: false, ErrorType: "context_canceled"`
- 親 ctx が cancel されると全 child の sleep も解放される (Go の Context tree により自動)

---

## 9. Error.type taxonomy (Q11=A)

Engine が emit する `Outcome.ErrorType` の固定 set (`errors.go` の const として定義):

```go
// AllowedErrorTypes lists every error.type value that the journey engine
// may write into an Outcome. PBT TP-U2-4 asserts that no Outcome carries
// an ErrorType outside this set (plus the empty string for success).
var AllowedErrorTypes = []string{
    "timeout",
    "connection_refused",   // disconnect fault
    "dns_failure",
    "http.500",
    "http.502",
    "http.503",
    "http.504",
    "grpc.unavailable",
    "grpc.deadline_exceeded",
    "grpc.unauthenticated",
    "db.connection_lost",
    "db.constraint_violation",
    "crashed",              // crash fault
    "circuit_open",
    "rate_limited",
    "context_canceled",     // ctx.Done() during execution
}
```

Engine が `Outcome.ErrorType` に書き込むのはこの set 内の値のみ。`error_rate_override` の error type は topology の FaultSpec で指定された値が **この set 内であれば** 採用、外れていれば `"unspecified"` (or warn). NFR Design で確定。

---

## 10. PBT properties (Q12=A)

| ID | 名前 | 種別 |
|---|---|---|
| TP-U2-1 | BuildPlan Idempotency | PBT-04 — 同じ (schema, name) で同じ Plan (structural equal) |
| TP-U2-2 | Plan visits all Journey ops | PBT-03 — Journey.Steps の全 Operation が tree 上に出現 |
| TP-U2-3 | Cascade is conditional | PBT-03 — Outcome.Cascaded=true ⇒ parent.Success=false |
| TP-U2-4 | error.type in allowed set | PBT-03 — Outcome.ErrorType ∈ AllowedErrorTypes ∪ {""} |
| TP-U2-5 | Time monotonicity | PBT-03 — child.StartTime >= parent.StartTime; finishFunc 呼び出し時 EndTime > StartTime |

詳細 implementation は `business-rules.md` §10 + Code Generation 時に確定。

---

## 11. Out of Scope (U2 では扱わない)

- **Span / Metric / Log 構築**: U3 (synth) の責務
- **OTLP 送信**: U4 (exporter) の責務
- **k6 lifecycle 管理**: U5 (k6otelgen) の責務
- **YAML parsing**: U1 (topology) の責務
- **FaultOverlay 構築**: U1 の責務 (Schema.BuildFaultOverlay 等)
- **Cycle detection in topology**: U1 の責務 (Schema.Validate で済)
- **Replica weighted distribution**: 現バージョンは uniform random (Q9=A); 将来検討
- **Sticky session emulation**: 同様、将来検討
- **Cross-iteration state**: VU 内 iteration 間で state を引き継ぐ機能なし
