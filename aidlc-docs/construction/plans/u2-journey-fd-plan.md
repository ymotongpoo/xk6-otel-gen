# U2 (journey) — Functional Design Plan

## ユニットコンテキスト

- **Unit ID**: U2
- **パッケージ**: `journey/`
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → **U2 (this)** → U5 → U6 → U8
- **Purpose** (Application Design より):
  - Topology + FaultOverlay から Plan (operation tree) を構築
  - Plan の実行制御 (sequential `Calls` + `parallel:` の `sync.WaitGroup` 並列実行)
  - エッジ呼び出し失敗時のリカバリーフロー (fallback chain → on_exhausted)
  - 条件付きカスケード障害伝播 (リカバリー枯渇かつ `propagate` 指定時のみ)
  - 実時間レイテンシシミュレーション (`time.Sleep`)
- **Upstream artifacts**:
  - `aidlc-docs/inception/application-design/component-methods.md` §C2 (Engine / Plan / Node / Outcome 型)
  - `aidlc-docs/inception/application-design/components.md`
  - `aidlc-docs/construction/u1-topology/` (Schema / Service / Edge / Operation / FaultOverlay の実装)
  - `aidlc-docs/construction/u3-synth/` (Synthesizer interface — Engine がこれを呼ぶ)
- **U3 から要求されている責務** (`u3-synth/functional-design/business-logic-model.md` §10, §13):
  - `SpanInput.InstanceIdx` を埋める (replica 選択)
  - `Outcome.ErrorType` を semconv 準拠 string で渡す
  - `StartTime` / `EndTime` を fault inflation 反映後の値で渡す

## FD で確定すべき事項

FD は「**何をする / どんなドメインルールに従う**」を確定する。U2 FD で扱う事項:

- **BuildPlan の構築アルゴリズム** — Topology の operation tree から Plan を組み立てる手順
- **Plan の実行モデル** — recursion vs explicit stack、parallel 実装パターン
- **Recovery flow の制御** — fallback chain の評価順、OnExhausted の 3 mode (`propagate` / `return_default` / `succeed_silently`)
- **Cascade 伝播条件** — recovery 枯渇 ∩ `propagate` のみ親方向へ伝播
- **Fault 適用ロジック** — `FaultOverlay` の 4 kind (`latency_inflation` / `error_rate_override` / `disconnect` / `crash`) を Outcome にどう反映
- **Replica 選択戦略** — per-step / per-VU sticky / weighted 等 (U3 から要請ありの責務)
- **時刻管理** — `time.Now()` / `time.Sleep` の使い方、Outcome.EndTime 計算
- **Error type taxonomy** — Engine が emit する semconv `error.type` 値リスト
- **並列実行モデル** — `sync.WaitGroup` × goroutine、context cancellation 伝播
- **Synthesizer 呼び出し順序** — BeginSpan → 子 → finishFunc → RecordMetric → EmitLog
- **PBT properties** — Plan 構造の不変条件、Outcome の不変条件
- **U7 への generator 追加リクエスト** — Plan / Node / Outcome generator

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u2-journey/functional-design/business-logic-model.md`
- [ ] `aidlc-docs/construction/u2-journey/functional-design/business-rules.md`
- [ ] `aidlc-docs/construction/u2-journey/functional-design/domain-entities.md`

---

## 設計確定のための質問

### Question 1: BuildPlan のアルゴリズム

`(*Engine).BuildPlan(journeyName)` の処理:

A) **DFS で operation tree を展開**: Journey.Steps を順に辿り、各 Step の Operation から `Operation.Calls` を再帰的に降りて Node tree を構築。`parallel:` 指定があれば Node.Parallel に配置、なければ Node.Children に sequential に配置 (推奨)

B) **BFS で展開** — 同じ結果だが queue ベース、tree 深さに比例しない call stack

C) **lazy 評価** — Plan は Journey reference + topology のみ持ち、Execute 時に on-the-fly で tree を辿る (Node 型を使わない) — memory 効率良いが debug 困難

X) Other

[Answer]: A

---

### Question 2: Plan の immutability

`*Plan` (および `*Node`) は構築後 immutable?

A) **Yes, immutable** (推奨) — `*Engine` で全 journey の Plan を BuildPlan 時に build しておき、Execute は read-only 走査。複数 VU 同時 Execute が race-free

B) **Mutable, per-Execute clone** — Execute ごとに Plan の copy を作り、execution state を埋め込む — clone コストあり

C) **Plan は immutable だが Node には execution state (Outcome) を埋める** — 同じ Plan instance を複数 VU が使うと race

X) Other

[Answer]: A

---

### Question 3: Parallel 実行モデル

`Node.Parallel` (兄弟 fan-out グループ) の並列実装:

A) **`sync.WaitGroup` + goroutine per Node** (推奨) — シンプル、Go の慣例的、parallel branch ごとに独立 goroutine

B) **errgroup.Group with limited concurrency** — context 連動の cancellation が容易、ただし依存追加

C) **Worker pool (固定 goroutine 数)** — 大量 fan-out 時の goroutine 爆発回避、ただし複雑

X) Other

[Answer]: A

---

### Question 4: 子 span の context 伝搬

Parent span から child span への trace context 伝搬:

A) **同期 child (sequential Children)**: 親の context を `synth.BeginSpan` で受け取った `ctx2` で継承 (推奨、OTel 標準)

B) **並列 child (Parallel)**: 親 ctx を `ctx2` ごと goroutine に渡す → 各 child は parent と兄弟関係になる (sibling spans, all sharing same trace_id, same parent_span_id)

C) A + B 両方 (混在は可) — Plan の構造的に sequential + parallel が混在可能 (`Children` と `Parallel` の組み合わせ)、自然に成立

X) Other

[Answer]: A

---

### Question 5: Recovery flow の制御フロー

Edge の `OnFailure` policy `Fallback []*Edge` を辿る順序:

A) **逐次評価**: `Fallback[0]` を試す → 失敗なら `Fallback[1]` → ... → 全部失敗なら `OnExhausted` (推奨、典型的な fallback パターン)

B) **並列評価 (fastest wins)** — 全 fallback を並列で試し、最初に成功したものを採用 — race condition だが circuit breaker 等で実用例あり

C) **指数バックオフ付き逐次** — fallback 間に sleep 入れる

X) Other

[Answer]: A

---

### Question 6: OnExhausted 3 mode の semantics

`RecoveryPolicy.OnExhausted` (`propagate` / `return_default` / `succeed_silently`) の挙動:

A) **as per `topology` 定義** (推奨):
   - `propagate`: 親 caller に失敗を伝える → 親 Outcome も failure になり、recovery がなければ更に上に cascade
   - `return_default`: caller には Outcome.Success=true、DefaultResponse をペイロード扱い、Outcome.DefaultUsed=true
   - `succeed_silently`: caller には Outcome.Success=true、Outcome.SilentlySucceeded=true、DefaultResponse なし
   いずれも `Outcome.PrimaryFailed=true` と `FallbackAttempts` を埋める

B) `return_default` と `succeed_silently` を統合 (DefaultResponse 有無で区別) — domain modeling 違反

C) Other modeling

X) Other

[Answer]: A

---

### Question 7: Cascade 伝播の正確な条件

「リカバリー枯渇かつ `propagate` 指定時のみ親方向へ cascade」の判定:

A) **Edge.OnFailure == nil の場合は default で propagate** (推奨、明示的 recovery が無いエッジは failure を素通し) — `Outcome.Cascaded` フラグは「parent からの強制」を表す、自分の failure には立てない

B) **Edge.OnFailure == nil は自動的に succeed_silently (failure を隠蔽)** — 危険、テストで意図と乖離

C) **明示 RecoveryPolicy なしの Edge では failure → エラー伝播 (cascade=true)、親 service にも failure を波及** — A と等価だが書き方が違う

X) Other

[Answer]: A

---

### Question 8: Fault 適用順序

`FaultOverlay` の 4 kind を Outcome にどう適用?

A) **lookup → apply 順** (推奨):
   1. `crash`: 該当 service なら即座に `Success=false, ErrorType="crashed"`、子は実行しない
   2. `disconnect`: 該当 edge なら `Success=false, ErrorType="connection_refused"` (子 = 別 service への call はしない)
   3. `error_rate_override`: `rand.Float64() < rate` で failure 強制 (`Success=false, ErrorType=<configured>`)
   4. `latency_inflation`: 基本 latency に inflation を加算 (multiplier or fixed delay)
   
   全 4 種を順に評価、複数 hit すれば順番に重ね、最終 Outcome を構築

B) **mutually exclusive (1 fault のみ適用)** — シンプルだが topology 仕様が複数 fault 同居を許す場合に対応不可

C) **DAG-style condition system** — fault graph を構築して条件評価 — 過剰

X) Other

[Answer]: A

---

### Question 9: Replica 選択戦略

`Service.Replicas > 1` の場合、各 step で `InstanceIdx` をどう選ぶ?

A) **per-step ランダム** (推奨、最も simulation 的): 各 step で `rand.IntN(svc.Replicas)` を独立に draw

B) **per-VU sticky** — k6 VU = 1 replica 固定 (VU 0 → replica 0、VU 1 → replica 1、...) — load balancer の sticky session を模擬

C) **per-journey weighted** — Service ごとに weight があれば cumulative distribution で選択 (Service.Replicas はあるが Service.ReplicaWeights は topology に無いので C は適用範囲不明)

X) Other

[Answer]: A

---

### Question 10: 時間管理 (latency simulation)

`time.Sleep` をどこで?

A) **各 Edge call (BeginSpan ↔ finishFunc 間) で sleep** (推奨) — 基本 latency + fault inflation = effective latency、Sleep してから finishFunc 呼ぶ → EndTime = StartTime + effective latency

B) **Sleep なし、EndTime のみ計算で進める** — wall-clock を進めない、純粋シミュレーション (test 高速化に有効、ただし backpressure を再現できない)

C) **Sleep フラグで切り替え** — `Engine.WithoutSleep()` 等のオプションで test 時 disable

X) Other

[Answer]: A

---

### Question 11: Error.type taxonomy

Engine が emit する `Outcome.ErrorType` semconv 文字列の集合:

A) **固定 enum-like 定数集合** (推奨、U3 FD §8 で要求された "Engine 側の責務として error.type 命名規約のドキュメント整備"):
   ```
   - "timeout"
   - "connection_refused"  (disconnect fault)
   - "dns_failure"
   - "http.500" / "http.502" / "http.503" / "http.504"
   - "grpc.unavailable" / "grpc.deadline_exceeded" / "grpc.unauthenticated"
   - "db.connection_lost" / "db.constraint_violation"
   - "crashed"  (crash fault)
   - "circuit_open"
   - "rate_limited"
   ```
   journey/errors.go に const として定義、PBT TP-U2-X で使われる allowed set

B) **完全自由 (任意 string)** — flexibility 高いが integration 検査 (TP-U3-4 / TP-U2-X) で broad set とせざるを得ない

C) **topology YAML で error.type を指定可能化** — error_rate_override の type field を増やす、よりカスタマイズ可能

X) Other

[Answer]: A

---

### Question 12: PBT testable properties

U2 で扱う property:

A) **5 properties** (推奨):
   - TP-U2-1: BuildPlan Idempotency (PBT-04) — 同じ Schema+journeyName で同じ Plan
   - TP-U2-2: Plan が all operations を visit (PBT-03 Invariant) — Journey.Steps 内の全 Operation が tree 上に出現
   - TP-U2-3: Cascade is conditional (PBT-03 Invariant) — Outcome.Cascaded=true ⇒ Edge.OnFailure==nil OR OnExhausted==propagate
   - TP-U2-4: error.type is in allowed set (PBT-03 Invariant) — Outcome.ErrorType ∈ AllowedErrorTypeSet ∪ {""}
   - TP-U2-5: Time monotonicity (PBT-03 Invariant) — child span の StartTime >= parent StartTime; finishFunc 呼び出し時 EndTime > StartTime

B) A + TP-U2-6: Recovery flow correctness (stateful PBT, PBT-06) — FallbackAttempts は OnFailure.Fallback の prefix

C) A + TP-U2-7: Parallel ordering invariance — 同じ parallel group 内の execution 順序は trace_id / parent_span_id に影響しない

X) Other

[Answer]: A

---

### Question 13: U7 への generator 追加リクエスト

PBT 用に U7 に追加する generator:

A) **3 generator pairs + 1 const list** (推奨):
   - `ValidPlan` / `AnyPlan` — `*journey.Plan` (Service / Operation / Edge / Parallel / Children)
   - `ValidNode` / `AnyNode` — `*journey.Node`
   - `ValidEngineOutcome` / `AnyEngineOutcome` — `journey.Outcome` (Success / Latency / StatusCode / ErrorType / Cascaded / recovery fields)
   - `AllowedErrorTypes []string` — TP-U2-4 用 const slice (PBT generator ではないが、testing で参照)
   合計 6 関数 + 1 const slice

B) A + Replica / InstanceIdx 等の補助 generator

C) Plan 生成は複雑なので minimal にして Node / Outcome のみ

X) Other

[Answer]: A

---

### Question 14: ファイル分割

`journey/` のファイル構成案:

A) **8 production + 6 test files** (推奨、最大規模 unit のため細分化):
   - `doc.go`
   - `engine.go` — Engine struct + NewEngine + ListJourneys + lifecycle
   - `plan.go` — Plan / Node 型 + BuildPlan アルゴリズム
   - `executor.go` — Execute 実装 + parallel / sequential dispatch + ctx cancellation
   - `recovery.go` — Recovery flow (fallback chain + OnExhausted) 実装
   - `fault.go` — FaultOverlay の lookup + apply ロジック
   - `replica.go` — Replica 選択 (Q9 戦略) + InstanceIdx 生成
   - `errors.go` — Engine-specific error 型 + AllowedErrorTypes const
   - tests: `engine_test.go`, `plan_test.go`, `executor_test.go`, `recovery_test.go`, `fault_test.go`, `pbt_test.go`

B) **5 production + 4 test files** — `executor.go` に recovery / fault / replica を内包
   - `doc.go`, `engine.go`, `plan.go`, `executor.go`, `errors.go`
   - tests: `engine_test.go`, `plan_test.go`, `executor_test.go`, `pbt_test.go`

C) **Functional 分割** — concern (build / execute / recover / fault) ごとに独立 file

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 3 つの FD アーティファクト (business-logic-model / business-rules / domain-entities) を生成して承認ゲートへ進みます。
