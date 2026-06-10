# U2 journey — Domain Entities & Method Contracts

本書は `journey/` パッケージの **公開 API** (型 / 関数 / メソッド) の contract を確定する。

末尾に **U7 への generator 追加リクエスト** セクション (Q13=A) を含む。

---

## 1. ドメインエンティティ

### 1.1 `Engine`

```go
package journey

import (
    "context"

    "github.com/ymotongpoo/xk6-otel-gen/synth"
    "github.com/ymotongpoo/xk6-otel-gen/topology"
)

// Engine drives the execution of journeys against a fixed Schema and
// FaultOverlay, delegating signal emission to a Synthesizer. An Engine
// is safe for concurrent use by multiple k6 VUs.
type Engine struct {
    // unexported
}
```

### 1.2 `Plan`

```go
// Plan is an immutable, pre-computed tree of operation invocations for a
// single journey. Construct via Engine.BuildPlan.
type Plan struct {
    JourneyName string
    Root        *Node
}
```

### 1.3 `Node`

```go
// Node is one operation invocation in a Plan tree. A Node has either
// Children (sequential next steps) or Parallel (fan-out siblings), never
// both. Nodes are immutable after BuildPlan returns.
type Node struct {
    Service   *topology.Service
    Operation string             // operation name within Service; "" for virtual fan-out node
    Edge      *topology.Edge     // edge used to reach this node from parent; nil for root
    Parallel  []*Node            // sibling group (mutually exclusive with Children)
    Children  []*Node            // sequential next steps
}
```

> **NOTE**: virtual fan-out node (`Service==nil`, `Operation==""`, `Parallel != nil`) は executor が parallel group のために挿入する。User code でも `*Node` の Service==nil をチェックする可能性がある。

### 1.4 `Outcome`

```go
// Outcome is the per-step result captured by the Engine and rolled up to
// the caller. The Engine writes the Outcome and the Synthesizer renders it
// into the OTel signals (status, attributes, log severity).
type Outcome struct {
    Success    bool
    Latency    time.Duration     // includes failed primary + fallback retries
    StatusCode int               // HTTP/gRPC code (or 0)
    ErrorType  string            // semconv error.type value; "" if Success
    Cascaded   bool              // true ⇔ outcome forced by upstream cascade (no recovery available)

    // Recovery tracking (per RecoveryPolicy)
    PrimaryFailed     bool                  // primary edge failed (regardless of final Success)
    FallbackAttempts  []*topology.Edge      // ordered list of fallback edges tried (all of which failed)
    FallbackUsed      *topology.Edge        // fallback edge that ultimately succeeded (nil if none)
    DefaultUsed       bool                  // true ⇔ OnExhausted==return_default consumed the failure
    SilentlySucceeded bool                  // true ⇔ OnExhausted==succeed_silently consumed the failure
}
```

### 1.5 不変条件 (Outcome)

- `Outcome.Success == true` → `Outcome.ErrorType == ""`
- `Outcome.Success == false` ∧ `Outcome.Cascaded == false` → `Outcome.ErrorType != ""`
- `Outcome.Cascaded == true` → `Outcome.Latency ≈ 0` (sleep skipped)
- `Outcome.DefaultUsed == true` ∨ `Outcome.SilentlySucceeded == true` → `Outcome.Success == true`
- `Outcome.FallbackUsed != nil` → `Outcome.Success == true` ∧ `Outcome.FallbackAttempts` の末尾要素は `FallbackUsed` ではない (FallbackAttempts は **失敗した** fallback の list)
- `Outcome.ErrorType ∈ AllowedErrorTypes ∪ {""}` (TP-U2-4)

### 1.6 エラー型

```go
type PlanError struct {
    Kind  string  // "unknown_journey" | "cycle" | "validate"
    Path  []string
    Inner error
}
func (e *PlanError) Error() string
func (e *PlanError) Unwrap() error

type ExecuteError struct {
    Kind    string  // "nil_plan" | "nil_ctx" | "internal"
    Inner   error
}
func (e *ExecuteError) Error() string
func (e *ExecuteError) Unwrap() error
```

### 1.7 内部型 (公開しない)

```go
// engineImpl: Engine 構造体の内部 representation
type engineImpl struct {
    schema  *topology.Schema
    overlay *topology.FaultOverlay
    synth   synth.Synthesizer

    plans map[string]*Plan        // BuildPlan で構築、Execute では read-only

    rand *rand.Rand               // per-Engine? per-VU? NFR Design で確定
    rmu  sync.Mutex               // rand を保護する場合の mutex (NFR Design)
}

// nodeKey: FaultOverlay lookup の key
type nodeKey struct {
    serviceName string
    operation   string
    edgeID      string // optional
}
```

---

## 2. 関数 / メソッド Contracts

### 2.1 `func NewEngine(schema *topology.Schema, overlay *topology.FaultOverlay, syn synth.Synthesizer) *Engine`

| 項目 | 内容 |
|---|---|
| 引数 | 3 つとも non-nil (nil → panic、programmer error) |
| 戻り値 | `*Engine` (concrete 型は `*engineImpl`) |
| 副作用 | Plans の Build 1 回 (overhead は init phase で吸収する設計、起動時にやる場合は engine.BuildAll() を持つかも — NFR Design 確定) |
| 失敗時 | schema invalid → panic (Schema は事前に Validate 済前提) |
| Thread-safe | はい (構築後) |

### 2.1.1 `func NewEngineWithSeed(schema *topology.Schema, overlay *topology.FaultOverlay, syn synth.Synthesizer, seed uint64) *Engine`

| 項目 | 内容 |
|---|---|
| 引数 | `NewEngine` と同じ 3 つの non-nil 引数 + deterministic random seed |
| 戻り値 | `*Engine` (concrete 型は `*engineImpl`) |
| 副作用 | Plans の Build 1 回。random source は `seed` から構築され、U5 の per-VU seed 注入に使う |
| 失敗時 | schema invalid → panic (Schema は事前に Validate 済前提) |
| Thread-safe | はい (構築後) |

### 2.2 `func (e *Engine) BuildPlan(journeyName string) (*Plan, error)`

| 項目 | 内容 |
|---|---|
| 引数 | `journeyName`: Schema.Journeys に存在する name |
| 戻り値 | `*Plan` (non-nil) + nil error / 失敗時 nil Plan + `*PlanError` |
| 副作用 | Plan を構築 (cache する場合は Engine 内 map に追加、NFR Design) |
| Idempotent | はい (TP-U2-1: 同じ name で同じ Plan structure) |
| Thread-safe | はい (内部 map は構築時に read-only) |
| 失敗パターン | (1) unknown journey、(2) cycle detected、(3) Operation lookup failure |

### 2.3 `func (e *Engine) Execute(ctx context.Context, plan *Plan) error`

| 項目 | 内容 |
|---|---|
| 引数 | `ctx`: 任意 (cancel/deadline 尊重); `plan`: BuildPlan で得たもの (他 Engine のは不可) |
| 戻り値 | nil OR `*ExecuteError{Kind: "internal"}` (panic recovery 経由) |
| 副作用 | synth 経由で OTel signal を emit、time.Sleep で実時間消費 |
| Idempotent | いいえ (毎回独立な span tree を生成、replica idx は random) |
| Thread-safe | はい (同じ Plan を複数 goroutine が Execute 並行可) |
| 失敗パターン | nil plan / nil ctx → `*ExecuteError`、step-level failure は Outcome に閉じ込めるので戻り値 error にしない |

### 2.4 `func (e *Engine) ListJourneys() []string`

| 項目 | 内容 |
|---|---|
| 戻り値 | Sort 済 journey 名 slice (deterministic) |
| 副作用 | なし |
| Thread-safe | はい |

---

## 3. パッケージレイアウト (Q14=A)

```text
journey/
├── doc.go                    // パッケージドキュメント
├── engine.go                 // Engine struct + NewEngine + ListJourneys + lifecycle
├── plan.go                   // Plan / Node 型 + BuildPlan アルゴリズム
├── executor.go               // Execute 実装 + parallel/sequential dispatch + ctx cancellation
├── recovery.go               // Recovery flow (fallback chain + OnExhausted) 実装
├── fault.go                  // FaultOverlay lookup + apply ロジック
├── replica.go                // Replica 選択 (Q9 戦略) + InstanceIdx 生成
├── errors.go                 // PlanError / ExecuteError + AllowedErrorTypes const
│
├── engine_test.go            // NewEngine / ListJourneys
├── plan_test.go              // BuildPlan + Plan structure (TP-U2-1, TP-U2-2)
├── executor_test.go          // Execute (mock synth) + parallel/sequential + cascade (TP-U2-3, TP-U2-5)
├── recovery_test.go          // Recovery flow / OnExhausted 3 mode
├── fault_test.go             // Fault application precedence (Q8 order)
└── pbt_test.go               // TP-U2-1..5 (rapid-based)
```

= **8 production + 6 test files**。最大規模 unit (U4: 8+7、U3: 7+8 と比べて concern が多いため細分化)。

---

## 4. 公開 API シグネチャ一覧

```go
// Constructor
func NewEngine(schema *topology.Schema, overlay *topology.FaultOverlay, syn synth.Synthesizer) *Engine
func NewEngineWithSeed(schema *topology.Schema, overlay *topology.FaultOverlay, syn synth.Synthesizer, seed uint64) *Engine

// Engine methods
func (e *Engine) BuildPlan(journeyName string) (*Plan, error)
func (e *Engine) Execute(ctx context.Context, plan *Plan) error
func (e *Engine) ListJourneys() []string

// Types
type Plan struct { JourneyName string; Root *Node }
type Node struct { Service *topology.Service; Operation string; Edge *topology.Edge; Parallel []*Node; Children []*Node }
type Outcome struct { Success bool; Latency time.Duration; StatusCode int; ErrorType string; Cascaded bool; PrimaryFailed bool; FallbackAttempts []*topology.Edge; FallbackUsed *topology.Edge; DefaultUsed bool; SilentlySucceeded bool }

// Errors
type PlanError struct { Kind string; Path []string; Inner error }
type ExecuteError struct { Kind string; Inner error }

// Constants
var AllowedErrorTypes = []string{"timeout", "connection_refused", ...}  // see business-logic-model.md §9
```

---

## 5. 依存

### 5.1 import 依存

| 依存 | 用途 |
|---|---|
| `context` | ctx propagation |
| `sync` | WaitGroup, Mutex |
| `time` | Sleep, Now |
| `math/rand/v2` | fault probability / replica idx (Go 1.22+) |
| `github.com/ymotongpoo/xk6-otel-gen/topology` | Schema / Service / Edge / Operation / Journey / FaultOverlay / RecoveryPolicy |
| `github.com/ymotongpoo/xk6-otel-gen/synth` | Synthesizer interface, SpanInput / MetricInput / LogInput / Outcome / FinishSpanFunc |

### 5.2 import しない

- `go.opentelemetry.io/otel/*` — synth が抽象化、U2 は直接 OTel SDK を触らない
- `exporter/` (U4) — synth 経由で provider 注入、U2 は U4 を知らない
- 外部 utility library (errgroup 等) — sync.WaitGroup で十分

---

## 6. U7 への generator 追加リクエスト (Q13=A)

### 6.1 Request from U2 FD

| Generator | 概要 | 利用される TP |
|---|---|---|
| `ValidPlan` / `AnyPlan` | `*journey.Plan` 生成 (root から DFS で Node tree を構築) | TP-U2-1, TP-U2-2 |
| `ValidNode` / `AnyNode` | `*journey.Node` (Service / Edge / Operation / Parallel / Children) | TP-U2-2 |
| `ValidEngineOutcome` / `AnyEngineOutcome` | `journey.Outcome` (Success / Latency / StatusCode / ErrorType / Cascaded / recovery fields) | TP-U2-3, TP-U2-4 |
| `AllowedErrorTypes` (const) | 既に `journey/errors.go` で定義、generator では `rapid.SampledFrom(AllowedErrorTypes)` で参照 | TP-U2-4 |

**合計**: 3 pairs = 6 函数 + journey package 側 const slice の参照。

### 6.2 詳細仕様

```go
// ValidPlan returns a generator producing a structurally valid Plan:
//   - Root != nil
//   - Each Node has either Children or Parallel, not both (except virtual fan-out)
//   - All Service / Edge / Operation references are non-nil
//   - Depth ≤ 5, breadth ≤ 4 to keep generation tractable
func ValidPlan(opts ...PlanOption) *rapid.Generator[*journey.Plan]

// AnyPlan may produce structurally invalid Plans (nil Service in non-virtual
// nodes, cycle-like back-references, etc.) for negative-path testing.
func AnyPlan(opts ...PlanOption) *rapid.Generator[*journey.Plan]

// ValidNode returns a generator producing a Node usable as a subtree root.
func ValidNode(opts ...NodeOption) *rapid.Generator[*journey.Node]

// AnyNode same with relaxed invariants.
func AnyNode(opts ...NodeOption) *rapid.Generator[*journey.Node]

// ValidEngineOutcome returns a generator producing an Outcome that satisfies
// the invariants of business-rules.md §1.5 (e.g. Success → ErrorType=="";
// !Success && !Cascaded → ErrorType != ""; ErrorType ∈ AllowedErrorTypes ∪ {""}).
func ValidEngineOutcome(opts ...OutcomeOption) *rapid.Generator[journey.Outcome]

// AnyEngineOutcome may produce outcomes violating invariants for shrinking.
func AnyEngineOutcome(opts ...OutcomeOption) *rapid.Generator[journey.Outcome]
```

### 6.3 不変条件 (Valid 系)

- `Plan.JourneyName != ""`
- `Plan.Root != nil`
- `Node.Service != nil` (virtual fan-out 以外)
- `Node.Operation != ""` (virtual fan-out 以外)
- `Node.Parallel != nil` ⇔ `Node.Service == nil` (virtual)
- `Node.Children` and `Node.Parallel` mutually exclusive
- Outcome invariants は §1.5 通り

### 6.4 実装スケジュール

U2 Code Generation Planning にて 1 Phase として登録 (U3 と同パターン)。`testutil/generators/journey_*.go` ファイル群に追加。

---

## 7. Application Design `component-methods.md` §C2 からの修正点

| 修正 | 理由 |
|---|---|
| `Outcome` フィールド全項目維持 | 変更なし |
| `Engine.BuildPlan` を **明示的に** caching する (内部 `plans map`) | Idempotency (TP-U2-1) と効率 |
| `Engine.Execute` が emit する telemetry は synth に完全委譲 (U2 は直接 OTel を呼ばない) | 設計 (interface 注入) |
| `*PlanError` / `*ExecuteError` 型追加 | エラー分類 |
| `AllowedErrorTypes` const slice 追加 | TP-U2-4 / U3 への semconv string contract |
| Random source の管理を engine 内部化 | thread-safe + deterministic seeding |

---

## 8. Out of Scope (U2 では扱わない、再掲)

`business-logic-model.md` §11 と同じ。
