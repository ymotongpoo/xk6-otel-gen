# U6 k6output — Domain Entities & Method Contracts

本書は `k6output/` パッケージの **公開 API** の contract を確定する。

末尾に **U7 / U4 への coordination 要件** を含む。

---

## 1. ドメインエンティティ

### 1.1 `Output`

```go
package k6output

import (
    "context"
    "sync"
    "time"

    "go.k6.io/k6/metrics"
    "go.k6.io/k6/output"
    sdkmetric "go.opentelemetry.io/otel/sdk/metric"

    "github.com/ymotongpoo/xk6-otel-gen/exporter"
)

// Output implements go.k6.io/k6/output.Output for the "otel-gen" extension.
// It registers itself with k6 SDK via init(), shares the process-singleton
// exporter.Pipeline with U5, and converts k6 native metrics to OTLP via
// a dedicated MeterProvider with service.name="xk6-otel-gen-runner".
type Output struct {
    // unexported
}
```

### 1.2 `Params` (internal config representation)

```go
// Params is the parsed --out args representation.
type Params struct {
    Endpoint     string
    Protocol     exporter.Protocol
    Insecure     bool
    Headers      map[string]string
    Compression  string
    Timeout      time.Duration
    BatchSize    int
    BatchTimeout time.Duration
    MaxQueueSize int
}

// (intentionally distinct from exporter.Config to allow U6-specific
//  fields in the future; today it's a 1:1 subset.)
```

### 1.3 `ConfigError`

```go
// ConfigError is U6-specific configuration error raised during --out args
// parsing or Start().
type ConfigError struct {
    Kind  string  // "invalid_args" | "invalid_protocol" | "type_mismatch" | "invalid_url"
    Field string
    Value string
    Inner error
}

func (e *ConfigError) Error() string
func (e *ConfigError) Unwrap() error
```

---

## 2. 関数 / メソッド Contracts

### 2.1 `func New(params output.Params) (output.Output, error)`

| 項目 | 内容 |
|---|---|
| 引数 | `params`: k6 SDK が渡す Output 構築 params (ConfigArgument に args 文字列) |
| 戻り値 | `output.Output` interface (実態 `*Output`) / `*ConfigError` on args parse failure |
| 副作用 | args parse のみ、Pipeline 構築なし |
| Thread-safe | はい (構築は k6 SDK が 1 回しか呼ばない) |

### 2.2 `func (o *Output) Description() string`

| 項目 | 内容 |
|---|---|
| 戻り値 | 例: `"k6 native metrics → OTLP/Metrics via xk6-otel-gen (endpoint=https://...)"` |
| 副作用 | なし |

### 2.3 `func (o *Output) Start() error`

| 項目 | 内容 |
|---|---|
| 戻り値 | nil / wrapped error (fmt.Errorf) |
| 副作用 | (1) `exporter.GetShared(factory)` で Pipeline 取得、(2) runner MeterProvider 構築、(3) 既知 metric の instrument 構築、(4) flush goroutine 起動 |
| 失敗時 | error 返却 → k6 run abort |
| Idempotent | sync.Once で 1 回のみ実行 |

### 2.4 `func (o *Output) AddMetricSamples(samples []metrics.SampleContainer)`

| 項目 | 内容 |
|---|---|
| 引数 | `samples`: k6 SDK が渡す sample 配列 |
| 副作用 | queue に push (non-blocking) |
| Thread-safe | はい (k6 SDK が 1 goroutine で呼ぶが、内部 channel send は safe) |
| Stop 後の呼び出し | no-op (queue が closed) |
| Start 前の呼び出し | no-op (queue 未初期化なら) |

### 2.5 `func (o *Output) Stop() error`

| 項目 | 内容 |
|---|---|
| 戻り値 | **常に nil** (k6 lifecycle を crash させない、Q7=A) |
| 副作用 | (1) flushLoop 停止 + queue drain、(2) Pipeline.Shutdown(ctx) — 30s timeout |
| Idempotent | sync.Once で 1 回のみ実行 |
| Shutdown error | warn log のみ (戻り値には反映しない) |

---

## 3. パッケージレイアウト (Q13=A)

```text
k6output/
├── doc.go                    // パッケージドキュメント
├── output.go                 // Output struct + New + Description + Start + Stop + init() 登録
├── params.go                 // Params struct + parseOutArgs (`--out` args parser)
├── convert.go                // k6 Sample → OTel instrument 変換 (metric type 判定 + instrument cache + tag → attribute)
├── errors.go                 // ConfigError type
│
├── output_test.go            // New / Start / Stop lifecycle
├── params_test.go            // parseOutArgs table-driven
├── convert_test.go           // Sample → metric mapping table
└── pbt_test.go               // TP-U6-1..3
```

= **5 production + 4 test files** + `helpers_test.go` (NFR Design で確定) + `doc_test.go` + `bench_test.go` + `integration/` subdir。

---

## 4. 公開 API シグネチャ一覧

```go
// k6 SDK contract (the only externally-callable surface from k6 itself)
func New(params output.Params) (output.Output, error)

// Output implementation (k6 SDK calls these via output.Output interface)
func (o *Output) Description() string
func (o *Output) Start() error
func (o *Output) AddMetricSamples(samples []metrics.SampleContainer)
func (o *Output) Stop() error

// Internal types exposed for test
type ConfigError struct { Kind, Field, Value string; Inner error }
```

> **NOTE**: Go-side API surface は **`New` 1 関数のみが strict public** (k6 SDK contract のため)。`Output` の各 method は `output.Output` interface 経由で呼ばれるが、struct 自体は opaque (lowercase fields)。

---

## 5. 依存

### 5.1 import 依存

| 依存 | 用途 |
|---|---|
| `go.k6.io/k6/output` | `output.Output` interface, `output.Params` |
| `go.k6.io/k6/metrics` | `metrics.Sample`, `metrics.SampleContainer`, `metrics.Metric`, `metrics.MetricType` 列挙 |
| `go.opentelemetry.io/otel/sdk/metric` | 独自 MeterProvider 構築 |
| `go.opentelemetry.io/otel/metric` | Float64Counter / Histogram / UpDownCounter interface |
| `go.opentelemetry.io/otel/sdk/resource` | Runner Resource 構築 |
| `go.opentelemetry.io/otel/attribute` | `k6.tag.*` attribute key |
| `go.opentelemetry.io/otel/semconv/v1.27.0` | `service.name`, `telemetry.sdk.*` const |
| `github.com/ymotongpoo/xk6-otel-gen/exporter` | Config, GetShared, Pipeline, **NEW: `Pipeline.MetricExporter()`** |
| stdlib `context`, `sync`, `time`, `strings`, `strconv`, `fmt`, `os`, `runtime` | |

### 5.2 import しない

- U1 `topology` — k6 native metrics は topology 非依存
- U2 `journey` — synth signal とは別 path
- U3 `synth` — 別 instrumentation
- U5 `k6otelgen` — 共通の `exporter` package 経由で Pipeline を共有するのみ、直接依存しない

---

## 6. Coordination 要件

### 6.1 U4 patch: `Pipeline.MetricExporter()` 追加

§7 (`business-logic-model.md`) で必要性確認済。U6 が独自 MeterProvider を構築するため、Pipeline の internal MetricExporter を取得する API が必要:

```go
// In exporter package (NEW):
// MetricExporter returns the underlying OTLP metric exporter used by this
// Pipeline. Intended for k6output to construct an additional MeterProvider
// with a different Resource (xk6-otel-gen-runner) while sharing the same
// network connection.
func (p *Pipeline) MetricExporter() sdkmetric.Exporter
```

- minor SemVer bump
- internal exporter は private field、新 accessor を expose
- U6 が `sdkmetric.NewMeterProvider(WithResource(runner), WithReader(NewPeriodicReader(p.MetricExporter())))` で構築

### 6.2 U7 への generator 追加 (Q12=A)

| Generator | 概要 | 利用される TP |
|---|---|---|
| `ValidK6Sample` / `AnyK6Sample` | `metrics.Sample` 生成 (Time, Metric, Value, Tags) | TP-U6-2, TP-U6-3 |
| `ValidOutputParams` / `AnyOutputParams` | `k6output.Params` 生成 (endpoint / protocol / args) | TP-U6-1 |

合計 4 関数。詳細は Code Generation Plan で実装スケジュール。

---

## 7. Application Design `component-methods.md` §C6 からの修正点

| 修正 | 理由 |
|---|---|
| `Params struct { OutputParams output.Params; Endpoint string; ... }` を内部型化 | k6 SDK convention に合わせ、`New(params output.Params)` で受ける |
| Output に **U4 patch (`Pipeline.MetricExporter`) 前提** を明示 | 独自 Resource を持つ MeterProvider 構築のため |
| Pipeline shutdown 順序を明示 (`Stop()` で 30s timeout、warn log) | k6 lifecycle 保護 |
| AddMetricSamples の non-blocking queue 戦略を明示 | high throughput 対応 |
| ConfigError type を明示 | エラー判別 |

---

## 8. Out of Scope (U6 では扱わない、再掲)

`business-logic-model.md` §14 と同じ。
