# U2 (journey) — NFR Design Plan

## ユニットコンテキスト

- **Unit ID**: U2
- **パッケージ**: `journey/`
- **FD**: `aidlc-docs/construction/u2-journey/functional-design/` (committed b585d7e)
- **NFR-R**: `aidlc-docs/construction/u2-journey/nfr-requirements/` (committed 7111053)
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → **U2 (this — NFR-D)** → U5 → U6 → U8

## NFR Design の焦点

FD で「何をする」、NFR-R で「何を達成するか」を確定済。NFR Design は **「どう実装するか」のパターン** を確定する:

- **Engine struct の物理 layout** — Plan cache, rand, mutex の配置
- **Random source の物理戦略** — per-Engine mutex vs per-VU pool (NFR-R open question)
- **executeNode 関数の再帰 vs explicit stack**
- **Parallel goroutine 起動パターン**
- **Panic recovery の defer 配置** — どの層で recover するか
- **Fault overlay lookup の物理 API** — U1 と coordinate
- **baseLatency の取得 API** — Operation の latency 仕様の参照
- **Outcome 構築パターン** — partial build / final assembly
- **Cascade child の span 表現** — emit するか skip するか確定 (FD で emit と決定したが詳細実装)
- **stateful PBT (PBT-06) の採用判断**
- **Test helper の物理構造**
- **Integration test の cascade verification 方法**
- **ファイル分割の最終確認**

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u2-journey/nfr-design/nfr-design-patterns.md`
- [ ] `aidlc-docs/construction/u2-journey/nfr-design/logical-components.md`

---

## 設計確定のための質問

### Question 1: Random source の物理戦略 (NFR-R Open Q)

`*rand.Rand` の管理方式:

A) **per-Engine 単一 source + sync.Mutex** (推奨、シンプル) — NFR-R で baseline 提案、bench 後に再評価方針
```go
type engineImpl struct {
    rand *rand.Rand
    rmu  sync.Mutex
}
func (e *engineImpl) rint(n int) int {
    e.rmu.Lock(); defer e.rmu.Unlock()
    return e.rand.IntN(n)
}
```

B) **per-VU `sync.Pool[*rand.Rand]`** — mutex なし、ただし pool 管理コスト + seed 戦略 (どこで Pool 初期化?)

C) **`math/rand/v2` global `rand.IntN()`** — Go 1.22+ で global race-free、deterministic seed 困難 (test 再現性低下)

X) Other

[Answer]: A

---

### Question 2: executeNode の実装スタイル

`executeNode(ctx, node, parentOutcome)` の再帰モデル:

A) **直接再帰** (推奨) — Go の関数 stack で十分、深さ 5 程度なら問題なし
```go
func (e *engineImpl) executeNode(ctx context.Context, node *Node, parent *Outcome) Outcome {
    // ... fault eval ...
    for _, child := range node.Children {
        co := e.executeNode(childCtx, child, ...)
        // ...
    }
}
```

B) **explicit stack (loop + []Node)** — 再帰なし、深い tree に対応、ただし readability 劣化

C) **goroutine-per-node (CSP モデル)** — 並列実行の自然な拡張だが overhead 大

X) Other

[Answer]: A

---

### Question 3: Parallel goroutine 起動

`Node.Parallel` グループの goroutine 起動:

A) **`sync.WaitGroup` + `go func()` per child** (推奨、FD Q3=A の素直な実装):
```go
var wg sync.WaitGroup
outcomes := make([]Outcome, len(node.Parallel))
for i, child := range node.Parallel {
    wg.Add(1)
    i, child := i, child  // shadow for Go < 1.22
    go func() {
        defer wg.Done()
        outcomes[i] = e.executeNode(ctx, child, parent)
    }()
}
wg.Wait()
```

B) **`go` で起動した child goroutine 内で panic recover** — panic 伝播阻止のため明示的に

C) **A + B の組み合わせ** — defer Recover を child goroutine 内に置き、main goroutine に Outcome 経由で notify

X) Other

[Answer]: A

---

### Question 4: Panic recovery の defer 配置

NFR-U2-5 で panic recovery が必要、具体的にどこに defer recover を置く?

A) **`Execute` の最上位 + parallel child goroutine 内** (推奨、二段防御):
```go
func (e *engineImpl) Execute(ctx context.Context, plan *Plan) (err error) {
    defer func() {
        if r := recover(); r != nil {
            err = &ExecuteError{Kind: "internal", Inner: fmt.Errorf("panic: %v", r)}
        }
    }()
    // ...
}

// parallel child:
go func() {
    defer wg.Done()
    defer func() {
        if r := recover(); r != nil {
            outcomes[i] = Outcome{Success: false, ErrorType: "internal_error", ...}
        }
    }()
    outcomes[i] = e.executeNode(ctx, child, parent)
}()
```

B) **executeNode 内の各 step で defer recover** — 細粒度、span close 保証強化、ただし overhead 多

C) **A + finishFn を defer で必ず呼ぶ pattern** — span 漏洩防止

X) Other

[Answer]: A

---

### Question 5: Fault overlay lookup API (U1 coordinate)

`*topology.FaultOverlay` の lookup signature 期待:

A) **明示的 method 4 個** (推奨、FD §4.2 で言及):
```go
overlay.LookupCrash(svc *Service) *FaultSpec
overlay.LookupDisconnect(edge *Edge) *FaultSpec
overlay.LookupErrorRate(svc *Service, op string) (rate float64, errorType string)
overlay.LookupLatencyInflation(svc *Service, op string) time.Duration
```
U1 実装と coordinate、U1 側が異なる shape なら U2 側で adapter 関数を持つ

B) **汎用 lookup**: `overlay.Lookup(target FaultTarget) []*FaultSpec` — kind ごとに filter する責務は U2 側
   U1 API が変わっても U2 が吸収できる、ただし lookup 毎に O(N) スキャンの risk

C) **NFR Design で確定保留、CG Plan 時に U1 実装を見て確定** — もっとも保守的

X) Other

[Answer]: A

---

### Question 6: baseLatency の取得

Operation の base latency をどう得る?

A) **`topology.Operation.Latency time.Duration` field を参照** (推奨) — U1 の Operation 型に既存 field (or 追加)、defaultLatency=10ms 等の default を NFR Design で確定

B) **`overlay.LookupBaseLatency(svc, op) time.Duration`** — fault overlay 側で base + inflation を統合管理

C) **NFR Design で確定、ここでは Operation.Latency field の有無を U1 と coordinate**

X) Other

[Answer]: A

---

### Question 7: Outcome 構築パターン

Outcome を build する場所:

A) **executeNode 内で local var `outcome Outcome` を逐次更新、最後に return** (推奨、明示的):
```go
outcome := Outcome{StatusCode: 200}  // assume success initially
// ... apply fault, run children, etc ...
outcome.Latency = endTime.Sub(startTime)
return outcome
```

B) **Outcome builder helper (`newSuccessOutcome` / `newFailureOutcome` / etc.)** — DRY、ただし helper 関数 explosion

C) **Outcome を mutable struct として goroutine 間で共有** — race 危険

X) Other

[Answer]: A

---

### Question 8: Cascade child の span 表現

FD で「cascade child でも span は emit する」と決めた。具体的に:

A) **BeginSpan + finishFn を呼ぶ、ただし Sleep skip、Outcome.Cascaded=true** (推奨、FD §5.3 通り):
   - span.SetAttributes に `synth.cascaded=true` (custom namespace)
   - duration ≈ 0 (sleep skip)
   - status は Error
   - ErrorType は親と同じ

B) **span を emit しない (cascade child は trace 上で見えない)** — シンプルだが trace で何が起きたか分かりにくい

C) **synthetic "cascade" span を emit (specific span name, attributes)** — A と等価だが span name を変える

X) Other

[Answer]: A

---

### Question 9: stateful PBT (PBT-06)

NFR-R で "optional, 実装時に難易度判断" としていた stateful PBT:

A) **採用しない、TP-U2-3 の通常 PBT で代用** (推奨、初期実装の複雑度回避) — Outcome.Cascaded=true ⇒ parent.Success=false の関係は通常 PBT で十分検証可能

B) **採用、`rapid.Run` ベースで実装** — cascade chain の deep test、ただし test 実装コスト高

C) **後追い** — Phase 後半で必要性確認、ない可能性大

X) Other

[Answer]: A

---

### Question 10: Test helper の物理構造

`journey/helpers_test.go` に置く utility:

A) **mock synth + test schema builder + outcome 比較ヘルパー** (推奨):
```go
type mockSynth struct{ ... }
func newMockSynth() *mockSynth
func newTestSchema(t *testing.T, ops ...) *topology.Schema
func newTestOverlay(t *testing.T, faults ...) *topology.FaultOverlay
func assertOutcomeMatches(t *testing.T, got, want Outcome)
```

B) **mock synth のみ、schema は test ごとに inline 構築** — DRY 違反だが test の意図が露わ

C) **helpers_test.go を分割** (`mocks_test.go`, `fixtures_test.go`, `asserts_test.go`)

X) Other

[Answer]: A

---

### Question 11: Integration test の cascade verification 方法

`journey/integration/` で cascade を verify する方法:

A) **OnExhausted=propagate を topology に含み、cascade 後に child span が emit されることを Collector JSON で確認** (推奨) — span attribute `synth.cascaded=true` の存在で identify

B) **cascade なしの baseline run と比較し、span 構造の差異から推定** — indirect、fragile

C) **integration test では cascade を test しない、unit test の TP-U2-3 で十分** — coverage gap

X) Other

[Answer]: A

---

### Question 12: ファイル分割の最終確認

FD §3 の 8 production + 6 test files (Q14=A):

A) **そのまま採用** (推奨):
   - `doc.go`, `engine.go`, `plan.go`, `executor.go`, `recovery.go`, `fault.go`, `replica.go`, `errors.go`
   - tests: `engine_test.go`, `plan_test.go`, `executor_test.go`, `recovery_test.go`, `fault_test.go`, `pbt_test.go`
   + (additional) `helpers_test.go`, `doc_test.go`, `bench_test.go`

B) **errors.go 省略**: U3 と同じく panic ベースなら不要かも。ただし U2 は `*PlanError` / `*ExecuteError` を **戻り値として** 返す (FD 確定) → 必要

C) **executor.go を分割**: 大きくなる場合 `executor_seq.go` / `executor_parallel.go` / `executor_cascade.go`

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR Design アーティファクトを生成して承認ゲートへ進みます。
