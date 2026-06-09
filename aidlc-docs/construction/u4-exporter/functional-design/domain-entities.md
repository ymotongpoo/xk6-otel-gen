# U4 exporter — Domain Entities & Method Contracts

本書は `exporter/` パッケージの **公開 API** (型 / 関数 / メソッド) の contract を確定する。

末尾に **U7 への generator 追加リクエスト** セクション (Q12=A) を含む。

---

## 1. ドメインエンティティ

### 1.1 `Protocol`

```go
type Protocol int

const (
    ProtocolGRPC Protocol = iota
    ProtocolHTTP
)

func (p Protocol) String() string  // "grpc" / "http"
```

### 1.2 `Config`

```go
type Config struct {
    Protocol          Protocol
    Endpoint          string
    Headers           map[string]string
    Insecure          bool
    Compression       string             // "" | "gzip"
    Timeout           time.Duration
    BatchSize         int
    BatchTimeout      time.Duration
    MaxQueueSize      int
    ResourceOverrides map[string]string
}
```

- **意味**: OTLP exporter pipeline の全設定
- **不変条件 (Validate 後)**:
  - `Protocol ∈ {ProtocolGRPC, ProtocolHTTP}`
  - `Endpoint != ""`
  - `Timeout > 0`, `BatchSize > 0`, `BatchTimeout > 0`, `MaxQueueSize > 0`
  - `MaxQueueSize >= BatchSize`
  - `Compression ∈ {"", "gzip"}`

### 1.3 `Pipeline`

```go
type Pipeline struct {
    // unexported fields
}

// 公開メソッド一覧 — 詳細は §2 で contract 化
func (p *Pipeline) TracerProvider() trace.TracerProvider
func (p *Pipeline) MeterProvider() metric.MeterProvider
func (p *Pipeline) LoggerProvider() log.LoggerProvider
func (p *Pipeline) Shutdown(ctx context.Context) error
func (p *Pipeline) Stats() Stats
```

- **意味**: 3 信号 (Traces / Metrics / Logs) の OTLP 送信を管理する singleton (per process, when shared) または per-instance
- **不変条件**:
  - `Pipeline.TracerProvider() != nil`、`MeterProvider() != nil`、`LoggerProvider() != nil` (New が成功した後)
  - Shutdown 後の TracerProvider 等の呼び出しは undefined behavior (OTel SDK の挙動に依存、典型的には no-op)

### 1.4 `Stats`

```go
type Stats struct {
    TracesExported  int64
    TracesFailed    int64
    TracesQueueLen  int64
    MetricsExported int64
    MetricsFailed   int64
    MetricsQueueLen int64
    LogsExported    int64
    LogsFailed      int64
    LogsQueueLen    int64
}
```

- **意味**: Pipeline の運用状態スナップショット
- **不変条件**:
  - `*Exported` / `*Failed` は monotonic increasing (per Pipeline lifetime)
  - `*QueueLen` は increase / decrease 両方ありうる

### 1.5 エラー型

```go
type PipelineError struct {
    Stage string // "resource" | "trace_exporter" | "metric_exporter" | "log_exporter" | "validate"
    Inner error
}
func (e *PipelineError) Error() string
func (e *PipelineError) Unwrap() error

type ConfigError struct {
    Field   string
    Value   any
    Message string
}
func (e *ConfigError) Error() string

type SharedError struct {
    Reason string  // "already_initialized" | "init_failed" | "not_set"
    Inner  error
}
func (e *SharedError) Error() string
func (e *SharedError) Unwrap() error
```

---

## 2. 関数 / メソッド Contracts

### 2.1 `func New(cfg Config) (*Pipeline, error)`

| 項目 | 内容 |
|---|---|
| 引数 | `cfg`: Config 値 (zero value OK、`fillDefaults` で埋まる) |
| 戻り値 | 成功時: `*Pipeline` (non-nil) + nil error / 失敗時: nil Pipeline + `*PipelineError` |
| 副作用 | 外部接続 (gRPC handshake は遅延、HTTP は新規接続なし) |
| Idempotent | いいえ (各呼び出しで新規 Pipeline インスタンス、3 つの新 Provider) |
| Thread-safe | はい (内部状態なし、呼び出しは独立) |
| 失敗パターン | (1) Config validate 失敗、(2) Resource 構築失敗、(3) Exporter 構築失敗 (3 信号いずれか) |

### 2.2 `func ConfigFromEnv() Config`

| 項目 | 内容 |
|---|---|
| 戻り値 | `OTEL_EXPORTER_OTLP_*` env から組み立てた Config。unset の env は zero value のまま |
| エラー | 戻り値型は `Config` のみ。pure (env が不正でも) — 検証は New 側で |
| Note | signal-specific (`_TRACES_` 等) が汎用と不一致なら、内部で priority があり最高優先のものを採用 |

実は、signal-specific 不一致をエラーにする選択肢もあるが (`business-logic-model.md` §2 で議論)、**戻り値型を `(Config, error)` に変える** ことになり利用者の便利さが下がる。Decision: signal-specific 不一致時は **silent 採用 (signal-specific 優先)** とし、`Lint` 系 API (将来) で警告する。

### 2.3 `func (c Config) MergeWith(override Config) Config`

| 項目 | 内容 |
|---|---|
| 引数 | レシーバ `c` (base)、`override` |
| 戻り値 | merge 後の Config |
| 副作用 | なし (純粋関数) |
| Idempotent | はい (`c.MergeWith(c) == c`) (TP-U4-2) |
| Thread-safe | はい (pure function) |
| Merge ルール | `business-rules.md` §2 参照 |

### 2.4 `func (c Config) Validate() error`

| 項目 | 内容 |
|---|---|
| 戻り値 | 違反なし: nil / 違反あり: `errors.Join` で集約された `*ConfigError` 群 |
| 副作用 | なし |
| Idempotent | はい (純粋判定) |
| Note | `New(cfg)` 内で自動呼び出し。利用者が pre-flight したい場合用に公開 |

### 2.5 `func (p *Pipeline) TracerProvider() trace.TracerProvider`

| 項目 | 内容 |
|---|---|
| 戻り値 | OTel `*sdktrace.TracerProvider` (interface 戻り) |
| 不変 | 同じ Pipeline インスタンスから常に同じ Provider を返す |
| 副作用 | なし |

(MeterProvider / LoggerProvider も同様)

### 2.6 `func (p *Pipeline) Shutdown(ctx context.Context) error`

| 項目 | 内容 |
|---|---|
| 引数 | `ctx`: deadline / cancel 尊重 |
| 戻り値 | `errors.Join` で集約された 3 Provider の Shutdown error。全成功なら nil |
| 副作用 | 3 Provider の Shutdown 呼び出し |
| Idempotent | はい (`sync.Once` で初回結果キャッシュ、2 回目以降は同じ error/nil 返却) |
| Thread-safe | はい (Once + 内部キャッシュ) |

### 2.7 `func (p *Pipeline) Stats() Stats`

| 項目 | 内容 |
|---|---|
| 戻り値 | 現時点の Stats スナップショット (atomic load) |
| 副作用 | なし |
| Thread-safe | はい (per-field `atomic.Load`) |
| Note | field 間の atomic 一貫性は保証しない (`business-rules.md` §4.2 参照) |

### 2.8 `func GetShared(factory func() (*Pipeline, error)) (*Pipeline, error)`

| 項目 | 内容 |
|---|---|
| 引数 | `factory`: 初回呼び出し時のみ実行される構築関数 |
| 戻り値 | 初回成功時の `*Pipeline` (キャッシュ済み) / 初回失敗時の error (キャッシュ済み) |
| 副作用 | 初回呼び出し時のみ factory 実行、結果を package-level singleton にキャッシュ |
| Idempotent | はい (`sync.Once`) |
| Thread-safe | はい (`sync.Once`) |
| 失敗時 | 初回失敗で error をキャッシュ、再試行しない (k6 ラン全体 fail fast、Q9=A) |

### 2.9 `func SetShared(p *Pipeline) error`

| 項目 | 内容 |
|---|---|
| 引数 | `p`: pre-built `*Pipeline` (non-nil) |
| 戻り値 | shared がまだ未初期化なら nil、初期化済みなら `*SharedError{Reason: "already_initialized"}` |
| 用途 | **テスト用のみ**。production code では使わない (linter で flag) |

### 2.10 `func ResetShared()`

| 項目 | 内容 |
|---|---|
| 戻り値 | なし |
| 用途 | **テスト用のみ**。`sync.Once` を新規化、shared を nil |
| 副作用 | 既に初期化済み Pipeline があれば、参照は orphan 化 (caller が Shutdown する責任) |

---

## 3. パッケージレイアウト (NFR Design 詳細化前)

```text
exporter/
├── doc.go                    // パッケージドキュメント
├── config.go                 // Config / Protocol / fillDefaults / Validate / MergeWith / ConfigFromEnv
├── pipeline.go               // Pipeline 構造体 / New / Shutdown / Stats / TracerProvider / MeterProvider / LoggerProvider
├── shared.go                 // GetShared / SetShared / ResetShared / 内部 var sharedOnce etc.
├── resource.go               // buildResource (Q10=A)
├── exporters.go              // buildTraceExporter / buildMetricExporter / buildLogExporter (Protocol 別分岐)
├── stats.go                  // pipelineStats / instrumented exporter wrappers
├── errors.go                 // PipelineError / ConfigError / SharedError
├── config_test.go            // example-based + TP-U4-1 / TP-U4-2
├── pipeline_test.go          // example-based for New / Shutdown / Stats
├── shared_test.go            // GetShared / SetShared / ResetShared invariants
├── otlp_roundtrip_test.go    // TP-U4-3
├── stats_monotonic_test.go   // TP-U4-4
└── bench_test.go             // BenchmarkNew (任意)
```

詳細は NFR Design ステージで確定。

---

## 4. 公開 API シグネチャ一覧

```text
// Constructor
func New(cfg Config) (*Pipeline, error)

// Config helpers
func ConfigFromEnv() Config
func (c Config) MergeWith(override Config) Config
func (c Config) Validate() error

// Pipeline methods
func (p *Pipeline) TracerProvider() trace.TracerProvider
func (p *Pipeline) MeterProvider() metric.MeterProvider
func (p *Pipeline) LoggerProvider() log.LoggerProvider
func (p *Pipeline) Shutdown(ctx context.Context) error
func (p *Pipeline) Stats() Stats

// Shared holder
func GetShared(factory func() (*Pipeline, error)) (*Pipeline, error)
func SetShared(p *Pipeline) error
func ResetShared()

// Stringer
func (p Protocol) String() string

// Errors (concrete types)
type PipelineError struct { Stage string; Inner error }
type ConfigError struct { Field string; Value any; Message string }
type SharedError struct { Reason string; Inner error }
```

---

## 5. 依存

### 5.1 import 依存

| 依存 | 用途 |
|---|---|
| `go.opentelemetry.io/otel/sdk/trace` | TracerProvider 構築 |
| `go.opentelemetry.io/otel/sdk/metric` | MeterProvider 構築 |
| `go.opentelemetry.io/otel/sdk/log` | LoggerProvider 構築 |
| `go.opentelemetry.io/otel/sdk/resource` | Resource 構築 |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace`, `otlptracegrpc`, `otlptracehttp` | Traces exporter |
| `go.opentelemetry.io/otel/exporters/otlp/otlpmetricgrpc`, `otlpmetrichttp` | Metrics exporter |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog`, `otlploggrpc`, `otlploghttp` | Logs exporter |
| `go.opentelemetry.io/otel/attribute` | Resource attributes |

### 5.2 内部 import なし

- `topology/` を import しない (Resource attribute は `map[string]string` で受け取る、ドメインモデル非依存)
- `journey/` / `synth/` を import しない (Domain inversion: U3 が U4 を import するのではなく、Provider interface を inject される)

---

## 6. U7 への generator 追加リクエスト (Q12=A)

### Request from U4 FD

U4 のテストで必要となる generator (`testutil/generators/` への追加):

| Generator | 概要 | 利用される TP |
|---|---|---|
| `ValidConfig` / `AnyConfig` | `exporter.Config` の Valid / Any 系 generator。Functional options で `WithProtocol`, `WithEndpoint`, `WithHeaders`, `WithTimeout` 等 | TP-U4-1, TP-U4-2 |

**合計**: 1 ペア × 2 = **2 関数**。

### 詳細仕様

```go
// ValidConfig returns a generator producing an exporter.Config that
// passes exporter.Config.Validate. All required fields are set within
// realistic ranges (Endpoint as scheme://host:port, Timeout in [1s, 30s],
// BatchSize in [128, 8192], MaxQueueSize >= BatchSize, etc.).
func ValidConfig(opts ...ConfigOption) *rapid.Generator[exporter.Config]

// AnyConfig returns a generator that may produce Configs with invalid
// or extreme values (negative timeouts, empty endpoints, unknown
// compression strings, MaxQueueSize < BatchSize, etc.).
func AnyConfig(opts ...ConfigOption) *rapid.Generator[exporter.Config]

// ConfigOption tunes the Config generator.
type ConfigOption func(*configOpts)

// Examples (functional options):
//   WithFixedEndpoint("https://test.example.com:4317")
//   WithProtocol(exporter.ProtocolGRPC)
//   WithMinTimeout(500 * time.Millisecond)
```

### 不変条件 (Valid 系)

- `Protocol ∈ {ProtocolGRPC, ProtocolHTTP}` (Q12 spec)
- `Endpoint` is `host:port` or `scheme://host:port`
- `Headers` map: 0〜5 entries, key と value はそれぞれ HTTP header 制約 (key: `[A-Za-z0-9-]+`, value: printable ASCII)
- `Insecure` bool
- `Compression ∈ {"", "gzip"}`
- `Timeout ∈ [1s, 30s]`
- `BatchSize ∈ [128, 8192]`
- `BatchTimeout ∈ [100ms, 30s]`
- `MaxQueueSize >= BatchSize`, ∈ `[BatchSize, BatchSize * 4]`
- `ResourceOverrides`: 0〜10 entries with realistic Semantic Conventions keys (e.g., `service.name`, `service.namespace`, `deployment.environment`)

### 実装スケジュール

U4 Code Generation Planning にて Phase 13 (or 別 Phase) として登録。Cursor batch で追加するのが自然 (既存 U7 generator スタイルに合わせる)。

---

## 7. Out of Scope (U4 では扱わない)

- **YAML defaults section** の parse — U1 の `topology.Schema.Exporter` (将来) が担う想定。本 unit は Config を受け取るのみ
- **Sampler のカスタマイズ** — `AlwaysSample` 既定。`TraceIDRatioBased` 等は将来オプション
- **OTel SDK 外部 metrics** (Provider 内部の dropped count 等) — 直接読まない、Q6=A 最小スコープ
- **Lint API** — `topology.Lint` のような warning 集約 API。本 unit では `Validate` が error を返すのみ
- **JSON serialization for testing** — `exporter` パッケージは YAML を扱わない (topology の責任)
