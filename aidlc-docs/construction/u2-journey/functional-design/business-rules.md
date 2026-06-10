# U2 journey — Business Rules

本書は U2 (`journey/`) の業務規則・不変条件・Testable Properties を確定する。

---

## 1. BuildPlan の規則 (Q1=A + Q2=A)

### 1.1 DFS 展開順序

- `Journey.Steps[]` を **順番に** 走査
- 各 Step の `Op.Calls` を再帰的に降りる
- `Step.Parallel != nil` → virtual fan-out Node (Service=nil) を作り `Node.Parallel` に子配置
- 上記以外 → 通常の Service+Operation Node を作り `Node.Children` に配置

### 1.2 Operation の Calls 構造

`topology.Operation.Calls` は `[]*Call` で、各 `Call` は:
- `Edge *topology.Edge` — どの edge 経由で呼ぶか
- `OpName string` — 呼び先 service の Operation 名
- `Parallel []*Call` — 並列 fan-out group

NFR Design 時に `Call` 型の正確な field 名を U1 実装に合わせて確認 (Application Design §C2 のスケッチに依拠)。

### 1.3 Plan は immutable

- 構築後 `*Plan.Root` 以下を変更しない
- 複数 goroutine が同じ Plan を read する
- 構造的 equality 比較 (`reflect.DeepEqual` または カスタム equal) で TP-U2-1 が成立

### 1.4 Cycle 検出

Schema.Validate が cycle を rejected していれば BuildPlan は遭遇しない。万一発生 (defense in depth) → `*PlanError{Kind: "cycle", Path: [...]}` を返す。

### 1.5 ListJourneys

`Engine.ListJourneys()` は `schema.Journeys` の keys を sort 済 (deterministic 用途 — JS 側で UI 表示するため) で返す。

---

## 2. Execute 全体規則

### 2.1 ルート span の構築

- `plan.Root` の Service.Kind から policy → `SpanKind`
- Root は entry node なので Edge=nil、Direction はデフォルト Server (Q4 で confirmed の通り)
- `synth.BeginSpan` が返す `ctx2` を全 child へ伝搬

### 2.2 Sequential children の走査

```text
for _, child := range node.Children:
    co := executeNode(parentCtx2, child, currentParentOutcome)
    if !co.Success:
        break  // sequential children: first failure halts subsequent
```

- ただし sequential 上の child failure → 親自身の primary failure とは別物。child failure は **parent の Outcome に直接は反映しない** (parent は自分の latency simulation で primary failure を別途決定する)
- "親が child の failure を観測して自分の Outcome を決める" 設計は **取らない** (シンプルさのため Q5/Q6 で確定)

### 2.3 Parallel group の走査 (Q3=A)

- `sync.WaitGroup` で全 child を待つ
- 全 child の Outcome を集約 (`aggregateParallelOutcomes`)
- aggregation rule: any child fail → group fail (`Success=false`), ErrorType は最初の failure
- group leader の Latency は max(child latencies) — 並列だから時間は最遅 child に律速

### 2.4 Context cancellation

- ctx.Done() を sleep 中も検知
- 中断された step は `Outcome.Success=false`, `ErrorType="context_canceled"`
- ctx.Done() による中断は **error.type allowed set に含む** (`AllowedErrorTypes`)

---

## 3. Fault 適用ルール (Q8=A)

### 3.1 評価順 (上から下、最初の hit で打ち切るか overlay するかは fault による)

| 順 | Fault | 振る舞い | 後続 child 実行? |
|---|---|---|---|
| 1 | `crash` | Outcome.Success=false, ErrorType="crashed", Latency=0 | No (子はスキップ) |
| 2 | `disconnect` | Outcome.Success=false, ErrorType="connection_refused", Latency=baseLatency (試行はしたが届かなかった) | No (この edge を渡らないので edge の to 側に行かない) |
| 3 | `error_rate_override` | rand < rate なら Success=false, ErrorType=<configured> | **Yes (children は実行する)** — error は最後に重ねる |
| 4 | `latency_inflation` | effectiveLatency += inflation | **常に適用** (failure と並行可) |

### 3.2 重複適用

- `error_rate_override` で失敗強制 **かつ** `latency_inflation` で遅延 → 両方とも適用
- `crash` と他 fault が同じ Service にある → `crash` が勝つ (即 fail、他評価しない)
- `disconnect` と error_rate が同じ edge → `disconnect` が勝つ (即 fail、edge を渡らない)

### 3.3 Fault 仕様の値域

- `error_rate_override.Rate ∈ [0.0, 1.0]`、外れ値は U1 Validate で reject 済前提
- `latency_inflation.Delay >= 0`、`Multiplier >= 1.0`
- `crash.Probability ∈ [0.0, 1.0]` (毎回 hit でなく確率的)

### 3.4 Fault の random seed

`rand.Float64()` 等の判定は `*Engine.rand *rand.Rand` を使う (per-Engine instance、k6 init で seed を VU 0 で固定するか per-VU instance を作るかは NFR Design)。

---

## 4. Recovery flow ルール (Q5=A + Q6=A)

### 4.1 Fallback chain の評価

- `Edge.OnFailure.Fallback []*Edge` を **インデックス順** に試行
- 各 fallback は **独立な span** として trace される (parent の子 span として)
- 最初の成功 fallback で chain 打ち切り
- 全失敗 → OnExhausted action

### 4.2 OnExhausted 3 mode

| Mode | Outcome.Success | Outcome.* field 更新 | Cascade 影響 |
|---|---|---|---|
| `ExhaustedPropagate` | `false` (primary の failure を維持) | PrimaryFailed=true, FallbackAttempts=[...], ErrorType=primary の ErrorType | parent へ failure 伝播 (parent も recovery 試みる) |
| `ExhaustedReturnDefault` | `true` | PrimaryFailed=true, FallbackAttempts=[...], DefaultUsed=true, ErrorType="", StatusCode=200 (or configured) | 親へは success が伝わる、child execution は通常通り |
| `ExhaustedSucceedSilently` | `true` | PrimaryFailed=true, FallbackAttempts=[...], SilentlySucceeded=true, ErrorType="", StatusCode=200 | 親へは success、child execution は通常 |

### 4.3 Fallback span attributes

各 fallback span は通常 span と同じく attribute policy に従う。custom attribute `synth.is_fallback=true` (or 同様) を NFR Design で追加検討。

### 4.4 Edge.OnFailure == nil の場合 (Q7=A)

- recovery を試みない
- primary failure → Outcome.Success=false のまま親に返る
- parent の executeNode が `shouldCascade(this.outcome)` を評価して cascade 継続

---

## 5. Cascade 規則 (Q7=A)

### 5.1 cascade 発生条件

```text
shouldCascade(parentOutcome) bool:
    return !parentOutcome.Success
```

シンプル: 親が失敗していれば child は実行不可と判断、child の Outcome は `{Success:false, Cascaded:true, ErrorType: parent.ErrorType}` を返す。

ただし以下例外:
- 親の `Edge.OnFailure` が `ReturnDefault` / `SucceedSilently` で recover した場合、parent.Success=true → child は **通常通り実行**
- 親が `Cascaded=true` 自身 (祖父からの cascade) でもその子は同じく cascade (`Outcome.Cascaded=true`)

### 5.2 Cascade flag のセマンティクス

- `Outcome.Cascaded=true` ⇒ "この outcome は親からの強制終了による" (自発的 failure ではない)
- `Outcome.Cascaded=false` ⇒ 自発的 (primary failure / fault) または成功
- PBT TP-U2-3: `Cascaded=true` ⇒ Latency=0 (実行されていないので)、ErrorType=親と同じ

### 5.3 Cascade の trace 表現

cascade した child でも **span は emit する** (visibility のため):
- `Outcome.Success=false, Cascaded=true` を `synth.BeginSpan + finishFn` 経由で span 化
- span attribute に `synth.cascaded=true` を追加 (custom namespace、NFR Design 確認)
- duration はほぼ 0 (cascade なら sleep スキップ)

---

## 6. Replica 選択 (Q9=A)

### 6.1 per-step random

```go
instanceIdx := rng.IntN(maxInt(svc.Replicas, 1))
```

- `svc.Replicas == 0` または `== 1` の場合 → idx=0
- `svc.Replicas == 3` → idx ∈ {0,1,2} uniform

### 6.2 sticky の不採用理由

- VU=replicaIdx の sticky は **load balancer の sticky session** を模擬するが、本ツールの主用途 (microservice topology の合成 telemetry) では **per-request load balancing** の方が一般的
- 将来 sticky が必要なら別 strategy (`Engine.WithStickyReplicas()`) で opt-in 可

### 6.3 InstanceIdx の SpanInput/MetricInput への伝搬

- `executeNode` で draw した `instanceIdx` を `synth.SpanInput.InstanceIdx` / `synth.MetricInput.InstanceIdx` の両方に詰める
- U3 がこれを `BuildResource(svc, idx)` 経由で `service.instance.id` 属性に展開

---

## 7. 時間管理 (Q10=A)

### 7.1 Real time.Sleep

```text
- StartTime: time.Now() (BeginSpan 直前)
- Sleep: select { case <-ctx.Done(): ... case <-time.After(effectiveLatency): ... }
- EndTime: time.Now() (finishFunc 直前) — 実 sleep 完了時刻
```

### 7.2 EndTime と StartTime の不変条件

- `EndTime >= StartTime` (TP-U2-5)
- 通常 `EndTime - StartTime ≈ effectiveLatency` (Go の time.After 精度内)
- cascade child は `Sleep` をスキップするので `EndTime ≈ StartTime`

### 7.3 ctx.Done() の StartTime/EndTime 影響

- sleep 中に ctx cancel → `EndTime = time.Now()` (sleep 途中の時刻)
- Outcome.Latency = EndTime - StartTime (sleep 完了より短い)

---

## 8. Synthesizer 呼び出し順序

### 8.1 順序契約

各 node 実行の synth 呼び出し順は厳密に:

```
1. BeginSpan (ctx → ctx2)
2. (sleep, children, fault eval)
3. finishFn(outcome)
4. RecordMetric(ctx2, ...)
5. EmitLog(ctx2, ...)
```

理由:
- finishFn の前に Record/Emit すると span がまだ open、`outcome.EndTime` を反映できない
- ctx2 (= span context) を Record/Emit に渡すことで trace_id/span_id を関連付け

### 8.2 panic recovery

executeNode 内で panic 発生時、`defer finishFn(Outcome{Success: false, ErrorType: "internal_error"})` で span を必ず close (NFR Design で確認)。U2 自身は panic を投げないが、synth / topology の panic 経由は防御。

---

## 9. Engine の Concurrency 規則

### 9.1 Engine field

- `*Engine.schema` — `*topology.Schema`、read-only
- `*Engine.overlay` — `*topology.FaultOverlay`、read-only
- `*Engine.synth` — `synth.Synthesizer`、thread-safe (U3 NFR-U3-3)
- `*Engine.plans` — `map[string]*Plan`、構築後 read-only
- `*Engine.rand` — random number source、NFR Design で per-VU か mutex 付きか確定

### 9.2 Plan / Node の read-only

- 構築後 field を変更しない
- 複数 goroutine が同じ Plan を read

### 9.3 Goroutine 安全性のレビュー対象

- Parallel group の WaitGroup
- ctx cancellation の伝搬
- random number generator の seed と access

---

## 10. Testable Properties (PBT-01, Q12=A)

### TP-U2-1: BuildPlan Idempotency (PBT-04)

```text
For all (schema, journeyName) drawn from ValidSchema() / journeys keys:
    p1, _ := engine.BuildPlan(journeyName)
    p2, _ := engine.BuildPlan(journeyName)
    structuralEqual(p1, p2) == true
```

`structuralEqual` は Service / Operation / Edge ポインタを名前比較 (アドレスは新規 Plan で異なる可能性がある)。

### TP-U2-2: Plan visits all journey ops (PBT-03 Invariant)

```text
For Journey J with Operations { op1, op2, ... }:
    walked := walkPlanNodes(p)
    {op | op ∈ walked} ⊇ Journey.Steps.flatten().Operations
```

(Journey の Steps に含まれる全 Operation が Plan tree 上に出現する。Plan には fault によって到達しない branch も含まれる)

### TP-U2-3: Cascade is conditional (PBT-03 Invariant)

```text
For any executed outcome o:
    if o.Cascaded:
        o.Success == false
        o.Latency ≈ 0  (cascade は sleep skip)
```

実装: stateful PBT — Schema/journeyName を draw、faults を inject、Execute を回し、得られた outcome を再帰的に検査。

### TP-U2-4: error.type in allowed set (PBT-03 Invariant)

```text
For any Outcome o:
    o.ErrorType == "" OR o.ErrorType ∈ AllowedErrorTypes
```

### TP-U2-5: Time monotonicity (PBT-03 Invariant)

```text
For any executed Plan:
    For every child span c with parent p:
        c.StartTime >= p.StartTime
    For every node:
        Outcome.EndTime >= SpanInput.StartTime
```

### (Optional) TP-U2-6 / TP-U2-7

`u2-journey-fd-plan.md` Q12 で B/C を選んでいないので、TP-U2-6 (FallbackAttempts は OnFailure.Fallback の prefix) や TP-U2-7 (parallel ordering invariance) は **将来追加候補** として記録、現バージョンでは scope 外。

---

## 11. パフォーマンスとリソース (FD 時点目安、NFR-R で確定)

| 項目 | 期待値 |
|---|---|
| `BuildPlan(name)` 所要時間 | < 1 ms (typical journey、深さ ≤ 5 段、operations ≤ 20 個) |
| `Execute(ctx, plan)` 自体のオーバーヘッド (synth call を除いた dispatch) | < 50 µs / journey の per-step オーバーヘッド |
| `Engine` インスタンスメモリ | < 1 MB (Plan キャッシュ込み、複雑 topology まで) |
| `*Plan` per-journey メモリ | < 10 KB (深さ ≤ 5、operations ≤ 20) |

NFR-R で各項目を厳密化。

---

## 12. Error 型

- `*PlanError{Kind, Path, Inner}` — BuildPlan 失敗
- `*ExecuteError{Kind, NodePath, Inner}` — Execute 失敗 (programmer error / ctx cancel 以外)
- ctx.Done() / per-step failure は **Outcome の failure として表現** (Execute の戻り値 error にしない、journey は完走させる)

Engine の public method signature:
- `BuildPlan(name) (*Plan, error)` — error は `*PlanError`
- `Execute(ctx, plan) error` — error は **programmer error 限定** (nil plan, nil ctx 等)

---

## 13. Out of Scope (再掲)

`business-logic-model.md` §11 と同じ。
