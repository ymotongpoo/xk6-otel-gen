# U5 k6otelgen — Domain Entities & Method Contracts

本書は `k6otelgen/` パッケージの **公開 API + JS-callable API** の contract を確定する。

末尾に **U7 への generator 追加リクエスト** セクション (Q12=A) を含む。

---

## 1. Go-side ドメインエンティティ

### 1.1 `RootModule`

```go
package k6otelgen

import (
    "sync"

    "github.com/grafana/sobek"
    "go.k6.io/k6/js/modules"

    "github.com/ymotongpoo/xk6-otel-gen/exporter"
    "github.com/ymotongpoo/xk6-otel-gen/topology"
)

// RootModule is the process-singleton k6 extension module. It holds the
// shared Schema / FaultOverlay / Pipeline state initialized via Load and
// Configure JS APIs.
type RootModule struct {
    // unexported

    schemaOnce    sync.Once
    schemaErr     error
    schema        *topology.Schema
    overlay       *topology.FaultOverlay
    loadedPath    string
    handle        *TopologyHandle  // cached handle for the loaded path

    configureOnce sync.Once
    configureErr  error
    config        exporter.Config
    configured    bool
}

// New is called once by k6 during init() and returns the process singleton.
func New() *RootModule
```

### 1.2 `ModuleInstance`

```go
import (
    "github.com/ymotongpoo/xk6-otel-gen/journey"
    "github.com/ymotongpoo/xk6-otel-gen/synth"
)

// ModuleInstance is constructed once per k6 VU and holds per-VU state.
type ModuleInstance struct {
    vu      modules.VU
    root    *RootModule
    engine  *journey.Engine
    synth   synth.Synthesizer
    handle  *TopologyHandle  // bound to this VU's engine
    initErr error            // captured Pipeline / synth / engine build error
}

// Exports returns the JS-visible API surface for this VU.
func (i *ModuleInstance) Exports() modules.Exports
```

### 1.3 `TopologyHandle`

```go
// TopologyHandle is the JS-side object returned by otelgen.load(). It holds
// a reference to the per-VU Engine and exposes runJourney + journeys.
type TopologyHandle struct {
    // unexported

    name   string         // loaded path
    engine *journey.Engine // per-VU Engine reference
    module *RootModule    // back-reference for stats lookup
}
```

### 1.4 `Stats` (JS-visible)

```go
// Stats is a JS-side mirror of exporter.Stats, returned by otelgen.stats().
type Stats struct {
    TracesExported  int64 `js:"tracesExported"`
    TracesFailed    int64 `js:"tracesFailed"`
    MetricsExported int64 `js:"metricsExported"`
    MetricsFailed   int64 `js:"metricsFailed"`
    LogsExported    int64 `js:"logsExported"`
    LogsFailed      int64 `js:"logsFailed"`
}
```

(タグ `js:` は sobek の field naming convention 用、NFR Design で正確な spelling を確認)

### 1.5 Error 型

```go
// ConfigError is U5-specific configuration error (raised by load/configure).
type ConfigError struct {
    Kind    string  // "already_loaded" | "already_configured" | "not_loaded" | "path_mismatch" | "file_not_found" | "parse_error" | "validate_error"
    Path    string  // affected path (if applicable)
    Inner   error
}
func (e *ConfigError) Error() string
func (e *ConfigError) Unwrap() error
```

---

## 2. JS-callable API (Q1=A)

### 2.1 Top-level methods (otelgen 名前空間)

| JS method | Go method | 引数 | 戻り値 |
|---|---|---|---|
| `otelgen.configure(opts)` | `(*ModuleInstance).Configure(rt, opts)` | `sobek.Value` (object) | undefined / throws |
| `otelgen.load(path)` | `(*ModuleInstance).Load(rt, path)` | `string` | `*TopologyHandle` / throws |
| `otelgen.stats()` | `(*ModuleInstance).Stats(rt)` | none | `Stats` object / throws |
| `otelgen.journeys()` | `(*ModuleInstance).Journeys()` | none | `[]string` |

### 2.2 TopologyHandle methods

| JS method | Go method | 引数 | 戻り値 |
|---|---|---|---|
| `handle.runJourney(name)` | `(*TopologyHandle).RunJourney(rt, name)` | `string` | undefined / throws |
| `handle.journeys()` | `(*TopologyHandle).Journeys()` | none | `[]string` |

---

## 3. 関数 / メソッド Contracts

### 3.1 `func New() *RootModule`

| 項目 | 内容 |
|---|---|
| 引数 | なし |
| 戻り値 | `*RootModule` (state は zero、init phase でリセット済) |
| 副作用 | なし |
| Idempotent | はい (毎回 zero state を返す、ただし k6 SDK が 1 回しか呼ばない) |
| Thread-safe | はい |

### 3.2 `func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance`

| 項目 | 内容 |
|---|---|
| 引数 | `vu`: k6 SDK の VU context |
| 戻り値 | `modules.Instance` (実態は `*ModuleInstance`) |
| 副作用 | Engine / Synthesizer 構築 (Pipeline は遅延構築) |
| Thread-safe | はい (各 VU が独立 goroutine から呼ぶ、内部は read-only RootModule にアクセス) |
| 失敗時 | initErr に保存、Exports() 内で JS exception 化 |

### 3.3 `(*ModuleInstance).Exports() modules.Exports`

JS-callable function を `modules.Exports.Named` に配置。

```go
func (i *ModuleInstance) Exports() modules.Exports {
    return modules.Exports{
        Named: map[string]any{
            "configure": i.jsConfigure,
            "load":      i.jsLoad,
            "stats":     i.jsStats,
            "journeys":  i.jsJourneys,
        },
    }
}
```

`i.jsConfigure` 等は `sobek.FunctionCall` を取る wrapper で、引数を取り出し、Go method を呼び、戻り値/エラーを JS exception に変換する。

### 3.4 `(*ModuleInstance).Load(path string) (*TopologyHandle, error)`

| 項目 | 内容 |
|---|---|
| 引数 | `path string` |
| 戻り値 | `*TopologyHandle` / `*ConfigError` |
| 副作用 | RootModule の schemaOnce.Do を起動、schema/overlay/handle を構築 (初回のみ) |
| Idempotent | はい (同じ path で再呼び出し → 同じ handle) |
| Thread-safe | はい (sync.Once) |

### 3.5 `(*ModuleInstance).Configure(opts map[string]any) error`

| 項目 | 内容 |
|---|---|
| 引数 | `opts`: JS object を decoded した map |
| 戻り値 | nil / `*ConfigError{Kind: "already_configured"}` |
| 副作用 | RootModule の configureOnce.Do で config 構築 |
| Idempotent | いいえ (2 回目以降は error) |

### 3.6 `(*ModuleInstance).Stats() (Stats, error)`

| 項目 | 内容 |
|---|---|
| 戻り値 | Stats / *ConfigError (pipeline 未構築の場合) |
| 副作用 | Pipeline を lazy build (まだなら) |

### 3.7 `(*ModuleInstance).Journeys() []string`

| 項目 | 内容 |
|---|---|
| 戻り値 | sort 済 journey name の slice (schema 未 load なら empty) |
| Idempotent | はい |

### 3.8 `(*TopologyHandle).RunJourney(name string) error`

| 項目 | 内容 |
|---|---|
| 引数 | `name`: journey 名 |
| 戻り値 | nil / `*PlanError` / `*ExecuteError` |
| 副作用 | journey.Engine.BuildPlan + Execute (synth 経由で OTel signal emit) |
| Ctx source | `handle.module.instance.vu.Context()` (per-VU iteration ctx) |
| Thread-safe | はい (1 VU = 1 goroutine、Engine 自体 thread-safe) |
| 失敗時 | journey error を JS exception 化 |

### 3.9 `(*TopologyHandle).Journeys() []string`

`(*ModuleInstance).Journeys()` のエイリアス (handle 経由でも同じ schema)。

---

## 4. パッケージレイアウト (Q13=A)

```text
k6otelgen/
├── doc.go                    // パッケージドキュメント + `--out` warning
├── module.go                 // RootModule struct + New + NewModuleInstance + init() 登録
├── instance.go               // ModuleInstance struct + Exports + jsXxx wrappers
├── handle.go                 // TopologyHandle struct + RunJourney + Journeys
├── config.go                 // opts → exporter.Config 変換ロジック
├── errors.go                 // ConfigError + JS exception 化ヘルパー
│
├── module_test.go            // RootModule 構築 / sync.Once / Configure / Load
├── instance_test.go          // Per-VU instance 構築 / Exports
├── handle_test.go            // RunJourney 経由の Engine.Execute 呼び出し
└── config_test.go            // opts decode + merge
```

= **6 production + 4 test files**。

### 4.1 共通テスト helper

`module_test.go` の冒頭 (or 別ファイル `helpers_test.go` を NFR Design で確定) で:
- `newTestRuntime(t)` — `modulestest.NewRuntime(t)` wrapper
- `newTestRootModule(t)` — 各 test で独立 RootModule
- `newTestModuleInstance(t)` — VU instance
- `loadTestSchema(t, yaml)` — temp file 経由で load

---

## 5. 公開 API シグネチャ一覧

```go
// Go-side public surface (k6 SDK contract)
func New() *RootModule
func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance
func (i *ModuleInstance) Exports() modules.Exports

// Internal/test-only (lowercase or internal package candidate)
// func (i *ModuleInstance) Load(path string) (*TopologyHandle, error)
// func (i *ModuleInstance) Configure(opts map[string]any) error
// func (i *ModuleInstance) Stats() (Stats, error)
// func (i *ModuleInstance) Journeys() []string
// func (h *TopologyHandle) RunJourney(name string) error
// func (h *TopologyHandle) Journeys() []string

// Error type
type ConfigError struct { Kind, Path string; Inner error }
```

> **NOTE**: Go-side で `Load` / `Configure` 等を public method にするかは選択肢:
> - public: testability 高、kebab API user が直接呼ぶ可能性も
> - lowercase: JS-side からのみ呼ばれる、Go API surface 最小化
>
> NFR Design で選択 (default: public で testability 重視)。

---

## 6. 依存

### 6.1 import 依存

| 依存 | 用途 |
|---|---|
| `go.k6.io/k6/js/modules` | k6 module SDK |
| `go.k6.io/k6/js/modules/k6test` または `modulestest` | test 用 sobek runtime (test-only) |
| `github.com/grafana/sobek` | sobek runtime (JS value 操作) |
| `github.com/ymotongpoo/xk6-otel-gen/topology` | Schema / Validate / ApplyFaults |
| `github.com/ymotongpoo/xk6-otel-gen/exporter` | Config / GetShared / Pipeline / Stats |
| `github.com/ymotongpoo/xk6-otel-gen/synth` | NewDefault / Synthesizer |
| `github.com/ymotongpoo/xk6-otel-gen/journey` | NewEngine / BuildPlan / Execute |
| `sync` (stdlib) | sync.Once |
| `os`, `path/filepath` (stdlib) | YAML file load |

### 6.2 import しない

- `go.opentelemetry.io/otel/*` — synth/exporter 経由のみ
- 直接 OTLP exporter — exporter 経由

---

## 7. U7 への generator 追加リクエスト (Q12=A)

### 7.1 Request from U5 FD

| Generator | 概要 | 利用される TP |
|---|---|---|
| `ValidConfigureOpts` / `AnyConfigureOpts` | k6otelgen.configure(opts) の opts を JS object 相当の `map[string]any` で生成 (endpoint / protocol / insecure / headers / timeout 等) | TP-U5-2 |
| `ValidLoadPath` / `AnyLoadPath` | YAML file path string (relative / absolute / with traversal) | TP-U5-1 |

**合計**: 2 pairs × 2 = **4 関数**。

### 7.2 詳細仕様

```go
// ValidConfigureOpts returns a generator producing a configure(opts) input
// where each populated field has the JS-side type expected by U5 decode rules.
//   - endpoint: scheme://host:port string
//   - protocol: "grpc" or "http"
//   - insecure: bool
//   - headers: map[string]string
//   - timeout: number (ms) or string ("10s")
//   - batchSize: positive int
//   - batchTimeout: number (ms)
//   - maxQueueSize: positive int >= batchSize
//   - resourceOverrides: map[string]string
func ValidConfigureOpts(opts ...ConfigureOptsOption) *rapid.Generator[map[string]any]

// AnyConfigureOpts may produce invalid values (negative timeout, unknown
// protocol string, malformed headers) for negative-path testing.
func AnyConfigureOpts(opts ...ConfigureOptsOption) *rapid.Generator[map[string]any]

// ValidLoadPath returns a generator producing a file path string usable
// with otelgen.load(). Defaults to relative path under a temp dir.
func ValidLoadPath(opts ...LoadPathOption) *rapid.Generator[string]

// AnyLoadPath may produce traversal patterns / unicode / very long paths.
func AnyLoadPath(opts ...LoadPathOption) *rapid.Generator[string]
```

### 7.3 実装スケジュール

U5 Code Generation Planning にて 1 Phase として登録。`testutil/generators/k6otelgen_inputs.go` ファイル群に追加。

---

## 8. Application Design `component-methods.md` §C5 からの修正点

| 修正 | 理由 |
|---|---|
| `Stats` field naming を JS-friendly に明示 (`tracesExported` 等) | sobek の field name JS export 慣例 |
| `RunJourney` の ctx 取得を `vu.Context()` で確定 | NFR-U2-4 ctx cancellation 連動 |
| `Configure` の優先順位を明示 | U4 と整合 |
| `Load` の cache semantics を明示 | NFR-D で confusion 回避 |
| `*ConfigError{Kind}` の値域を明示 | Error 種別 grep / handling 用 |
| Pipeline Shutdown は U6 担当を明示 | Application Design §C6 と整合 |

---

## 9. Out of Scope (U5 では扱わない、再掲)

`business-logic-model.md` §13 と同じ。
