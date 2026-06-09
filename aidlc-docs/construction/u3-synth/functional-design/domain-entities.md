# U3 synth — Domain Entities & Method Contracts

本書は `synth/` パッケージの **公開 API** (型 / 関数 / メソッド) の contract を確定する。

末尾に **U7 への generator 追加リクエスト** セクション (Q12=C) を含む。

---

## 1. ドメインエンティティ

### 1.1 `Synthesizer` interface

```go
package synth

import (
    "context"

    "go.opentelemetry.io/otel/log"
    "go.opentelemetry.io/otel/metric"
    "go.opentelemetry.io/otel/sdk/resource"
    "go.opentelemetry.io/otel/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.27.0"

    "github.com/ymotongpoo/xk6-otel-gen/topology"
)

// Synthesizer is the interface seen by the Journey Engine.
// Implementations must be safe for concurrent use.
type Synthesizer interface {
    BeginSpan(ctx context.Context, in SpanInput) (context.Context, FinishSpanFunc)
    RecordMetric(ctx context.Context, in MetricInput)
    EmitLog(ctx context.Context, in LogInput)
}
```

### 1.2 入力型

```go
// SpanInput is the description of a journey node's span.
type SpanInput struct {
    Service     *topology.Service
    Edge        *topology.Edge   // nil for the journey entry node
    Operation   string           // Operation name within Service
    StartTime   time.Time        // engine-provided start timestamp
    InstanceIdx int              // 0..Service.Replicas-1; the chosen replica
}

// MetricInput is one metric data point to record after a step completes.
type MetricInput struct {
    Service     *topology.Service
    Edge        *topology.Edge   // nil for incoming-only operation at journey root
    Operation   string
    Latency     time.Duration
    Outcome     Outcome
    InstanceIdx int              // 0..Service.Replicas-1
}

// LogInput is one log record to emit.
type LogInput struct {
    Service    *topology.Service
    Severity   log.Severity      // optional; default Info on success, Error on failure
    Body       string            // optional; default is generated based on Outcome
    Attributes map[string]any    // merged into the record's attribute set
}

// Outcome describes the result of one operation invocation.
type Outcome struct {
    Success    bool
    StatusCode int               // HTTP / gRPC code; 0 if not applicable
    ErrorType  string            // semconv-compliant error.type value; empty on success
    EndTime    time.Time         // engine-provided end timestamp
}

// FinishSpanFunc must be called exactly once after the Outcome is known.
// Calling it more than once is a programmer error (may panic in race-detection builds).
type FinishSpanFunc func(outcome Outcome)
```

### 1.3 不変条件 (SpanInput / Outcome)

- `SpanInput.Service != nil`
- `SpanInput.Operation != ""`
- `SpanInput.InstanceIdx >= 0` and `< SpanInput.Service.Replicas`
- `Outcome.EndTime >= SpanInput.StartTime` (PBT で確認可能 — Engine 責務)
- `Outcome.Success == false` → `Outcome.ErrorType != ""` (`error.type` 必須、TP-U3-4)
- `LogInput.Service != nil`

### 1.4 内部型 (公開しない)

```go
// defaultSynthesizer implements Synthesizer using injected SDK providers.
type defaultSynthesizer struct {
    tp trace.TracerProvider
    mp metric.MeterProvider
    lp log.LoggerProvider

    // Pre-built per-namespace instruments (lazy or eager, NFR Design で確定).
    httpClientDur     metric.Float64Histogram
    httpServerDur     metric.Float64Histogram
    httpActiveReq     metric.Int64UpDownCounter
    rpcClientDur      metric.Float64Histogram
    rpcServerDur      metric.Float64Histogram
    rpcActiveReq      metric.Int64UpDownCounter
    dbClientDur       metric.Float64Histogram
    msgProducerDur    metric.Float64Histogram
    msgConsumerDur    metric.Float64Histogram

    tracer trace.Tracer
    logger log.Logger
}

// attributePolicy describes which semconv keys + SpanKind apply to a
// given (Service.Kind, Edge.Kind) combination.
type attributePolicy struct {
    SpanKind          trace.SpanKind
    AttributeNamespace string // "http" | "rpc" | "db" | "messaging" | ""
    MetricNamespace    string // same
}
```

---

## 2. 関数 / メソッド Contracts

### 2.1 `func NewDefault(tp trace.TracerProvider, mp metric.MeterProvider, lp log.LoggerProvider) Synthesizer`

| 項目 | 内容 |
|---|---|
| 引数 | OTel 公開 interface 型 3 個 |
| 戻り値 | `Synthesizer` (concrete 型は `*defaultSynthesizer`) |
| 副作用 | tp/mp/lp から tracer/meter/logger を取得、Histogram/UpDownCounter を生成 (lazy or eager は NFR Design) |
| 失敗時 | いずれかの引数が nil なら panic (programmer error) |
| Thread-safe | はい (構築は単一 goroutine 想定、構築後は thread-safe) |

### 2.2 `func BuildResource(svc *topology.Service, instanceIdx int) *resource.Resource`

| 項目 | 内容 |
|---|---|
| 引数 | `svc`: 非 nil の Service、`instanceIdx`: 0..Replicas-1 |
| 戻り値 | `*resource.Resource` (semconv 準拠 attribute set + custom) |
| 副作用 | なし (pure function、UUID v5 計算のみ) |
| Idempotent | はい (TP-U3-1) |
| Thread-safe | はい |
| 失敗時 | svc nil → panic、instanceIdx < 0 → panic、svc.Name 空 → panic |

### 2.3 `func (s *defaultSynthesizer) BeginSpan(ctx, in SpanInput) (context.Context, FinishSpanFunc)`

| 項目 | 内容 |
|---|---|
| 引数 | `ctx`: 親 span を含む可能性あり、`in`: SpanInput |
| 戻り値 | `ctx2`: in に対応する span を含む新コンテキスト、`FinishSpanFunc`: 終了時に呼ぶ closure |
| 副作用 | span 開始 (SDK 経由)、`active_requests +1` (server/consumer の場合) |
| Idempotent | いいえ (各呼び出しで新規 span) |
| Thread-safe | はい |
| 失敗時 | in 不正なら panic |

### 2.4 `func (s *defaultSynthesizer) RecordMetric(ctx, in MetricInput)`

| 項目 | 内容 |
|---|---|
| 引数 | `ctx`: 任意、`in`: MetricInput |
| 副作用 | Histogram.Record |
| Thread-safe | はい (SDK Histogram は thread-safe) |
| 失敗時 | in 不正なら panic |

### 2.5 `func (s *defaultSynthesizer) EmitLog(ctx, in LogInput)`

| 項目 | 内容 |
|---|---|
| 引数 | `ctx`: span context を含む可能性 (自動的に span_id/trace_id 付与)、`in`: LogInput |
| 副作用 | log record 1 件発出 |
| Thread-safe | はい |
| 失敗時 | in 不正なら panic |

### 2.6 `FinishSpanFunc(outcome Outcome)`

| 項目 | 内容 |
|---|---|
| 引数 | `outcome`: Outcome |
| 副作用 | span 終了、最終 attribute 設定、Span Status 設定、`active_requests -1` (server/consumer 時) |
| 呼び出し回数 | **exactly once**。2 回目以降は no-op で OK (race detection 環境では panic 検討) |
| Thread-safe | はい (異なる goroutine から呼んでも OK) |

---

## 3. パッケージレイアウト (Q13=A)

```text
synth/
├── doc.go                    // パッケージドキュメント
├── interface.go              // Synthesizer interface + SpanInput / MetricInput / LogInput / Outcome / FinishSpanFunc
├── synthesizer.go            // defaultSynthesizer struct + NewDefault + BeginSpan / RecordMetric / EmitLog 実装
├── resource.go               // BuildResource (per-service-instance Resource)
├── attributes.go             // (ServiceKind, EdgeKind) → policy マッピング + semconv attribute build helpers + import semconv/v1.27.0
├── errors.go                 // (もし必要なら) synth-specific error 型
│
├── interface_test.go         // SpanInput / Outcome 不変条件 + 構築可能性
├── synthesizer_test.go       // BeginSpan / RecordMetric / EmitLog example-based tests with mock provider
├── resource_test.go          // BuildResource example-based + TP-U3-1
├── attributes_test.go        // (ServiceKind, EdgeKind) policy mapping + TP-U3-2
└── pbt_test.go               // TP-U3-1, TP-U3-3, TP-U3-4 (TP-U3-2 は attributes_test.go に同居)
```

詳細は NFR Design ステージで確定。

---

## 4. 公開 API シグネチャ一覧

```text
// Interface
type Synthesizer interface { ... }

// Constructor
func NewDefault(tp trace.TracerProvider, mp metric.MeterProvider, lp log.LoggerProvider) Synthesizer

// Resource helper
func BuildResource(svc *topology.Service, instanceIdx int) *resource.Resource

// Input / Output value types
type SpanInput struct { ... }
type MetricInput struct { ... }
type LogInput struct { ... }
type Outcome struct { ... }

// Finish closure type
type FinishSpanFunc func(outcome Outcome)
```

---

## 5. 依存

### 5.1 import 依存

| 依存 | 用途 |
|---|---|
| `go.opentelemetry.io/otel/trace` | TracerProvider / Tracer interface |
| `go.opentelemetry.io/otel/metric` | MeterProvider / Meter interface |
| `go.opentelemetry.io/otel/log` | LoggerProvider / Logger interface |
| `go.opentelemetry.io/otel/sdk/resource` | Resource type for BuildResource 戻り値 |
| `go.opentelemetry.io/otel/attribute` | KeyValue 構築 |
| `go.opentelemetry.io/otel/semconv/v1.27.0` | semconv key constants (Q1=B) |
| `github.com/google/uuid` | UUID v5 for service.instance.id (deterministic) |
| `github.com/ymotongpoo/xk6-otel-gen/topology` | Service / Edge / Operation 型 |

### 5.2 import しない

- `exporter/` (U4) — interface 注入により直接依存しない
- `journey/` (U2) — Engine が synth を import するのみ、逆参照は禁止
- `go.opentelemetry.io/otel/propagation` — in-process telemetry 合成のため不要
- `go.opentelemetry.io/otel/baggage` — 同上
- `go.opentelemetry.io/otel/sdk/trace` 等の SDK concrete 型 — interface 経由のみ参照

---

## 6. U7 への generator 追加リクエスト (Q12=C)

### 6.1 Request from U3 FD

ユーザー回答 Q12=C: "ValidErrorType() もあったほうがいいように思うけど、正直テストを見ないと判断できない"

→ **解釈**: A 案 (4 ペア = 8 関数) を base にしつつ、`ValidErrorType()` を **追加候補** として明記。実装着手時に PBT テストコードを書きながら必要なら採用。

### 6.2 生成 generator 一覧

| Generator | 概要 | 利用される TP |
|---|---|---|
| `ValidSpanInput` / `AnySpanInput` | `synth.SpanInput` 生成 (Service / Edge / Operation / StartTime / InstanceIdx) | TP-U3-2, TP-U3-4 |
| `ValidMetricInput` / `AnyMetricInput` | `synth.MetricInput` 生成 | TP-U3-3 |
| `ValidLogInput` / `AnyLogInput` | `synth.LogInput` 生成 | (補助) |
| `ValidOutcome` / `AnyOutcome` | `synth.Outcome` 生成 (Success / StatusCode / ErrorType / EndTime) | TP-U3-4 |
| `ValidErrorType` | semconv 準拠 error.type 値 (`"timeout"`, `"http.500"` 等の固定セットからサンプリング) | TP-U3-4 (optional; 実装時に必要なら追加) |

**合計**: 4 ペア × 2 = **8 関数**、+ オプションで `ValidErrorType` 1 関数 = 最大 **9 関数**

### 6.3 詳細仕様

```go
// ValidSpanInput returns a generator producing a SpanInput where:
//   - Service is drawn from generators.ValidService()
//   - Edge is either nil (10% probability for journey entry) or a valid edge
//     drawn from generators.ValidEdge() with Edge.To.Operation == in.Operation
//   - Operation is one of Service.Operations
//   - StartTime is a recent timestamp (within the last hour)
//   - InstanceIdx is in [0, Service.Replicas)
func ValidSpanInput(opts ...SpanInputOption) *rapid.Generator[synth.SpanInput]

// AnySpanInput allows extreme/invalid values (nil Service, out-of-range
// InstanceIdx, future StartTime, etc.).
func AnySpanInput(opts ...SpanInputOption) *rapid.Generator[synth.SpanInput]

// (analogous for MetricInput / LogInput / Outcome)

// ValidErrorType samples from a fixed semconv-compliant set.
func ValidErrorType() *rapid.Generator[string]

var SemconvErrorTypes = []string{
    "timeout", "connection_refused", "dns_failure",
    "http.500", "http.502", "http.503", "http.504",
    "grpc.unavailable", "grpc.deadline_exceeded", "grpc.unauthenticated",
    "db.connection_lost", "db.constraint_violation",
}
```

### 6.4 実装スケジュール

U3 Code Generation Planning にて 1 Phase として登録。U1/U7/U4 の generator 追加と同様、`testutil/generators/synth_*.go` ファイル群に追加。

---

## 7. Out of Scope (U3 では扱わない)

- Provider lifecycle (U4 の責務 — Pipeline.Shutdown)
- Replica 選択 (U2 Journey Engine の責務)
- Fault injection の検出 / 適用 (U2 が timestamp / status code で表現済)
- JS API 露出 (U5 の責務)
- k6 native metric 変換 (U6 の責務)
- Propagation (in-process synthesis)
- Custom span processor (U4 の責務)

---

## 8. Application Design `component-methods.md` §C3 からの修正点

| 修正 | 理由 |
|---|---|
| `SpanInput` に `InstanceIdx int` を追加 | Q5=A: Engine が replica index を提供する |
| `MetricInput` に `InstanceIdx int` を追加 | per-instance metric breakdown を可能に |
| `Outcome.Success`, `StatusCode`, `ErrorType`, `EndTime` をそのまま採用 | 変更なし |
| `BuildResource` 引数 `instanceIdx int` を追加 | Q5=A |
| `NewDefault` signature 変更なし | Application Design のまま |

これらは U2 Journey Engine FD でも反映する必要あり (Engine が `SpanInput` を組み立てる責務)。
