# U4 exporter — Logical Components

本書は `exporter/` パッケージ内の **論理コンポーネント** (LC) を確定する。
各 LC について 責務 / 公開 API / 実装スケッチ / 依存関係 を定義する。

参照:
- FD: `aidlc-docs/construction/u4-exporter/functional-design/`
- NFR Design Patterns: `nfr-design-patterns.md` (本ディレクトリ内)

---

## コンポーネント一覧

| LC | 名前 | ファイル | 責務 |
|---|---|---|---|
| LC-0 | Package Documentation | `doc.go` | パッケージレベル GoDoc |
| LC-1 | Config & Validation | `config.go` | Config 型、Validate、MergeWith、ConfigFromEnv、fillDefaults |
| LC-2 | Resource Builder | `resource.go` | OTel Resource 構築 (auto-detect + override merge) |
| LC-3 | Exporter Factory | `exporters.go` | trace / metric / log の OTLP exporter 構築 (protocol 分岐) |
| LC-4 | Stats & Instrumentation | `stats.go` | `pipelineStats` + 信号別 instrumented wrapper |
| LC-5 | Pipeline | `pipeline.go` | `*Pipeline` 構造体、New、Shutdown、Stats、Provider accessor |
| LC-6 | Shared Holder | `shared.go` | `GetShared` / `SetShared` / `ResetShared` |
| LC-7 | Errors | `errors.go` | `PipelineError` / `ConfigError` / `SharedError` |

---

## LC-0: Package Documentation (`doc.go`)

### 責務
- パッケージ全体の GoDoc コメント
- 利用例の overview
- Lifecycle 説明
- Configuration priority の明示

### 公開 API
- なし (パッケージ comment のみ)

### 実装スケッチ
```go
// Package exporter provides an OTLP exporter pipeline for traces,
// metrics, and logs. A single Pipeline instance manages one TracerProvider,
// one MeterProvider, and one LoggerProvider that share an endpoint, headers,
// and resource attributes.
//
// Typical usage from a k6 extension:
//
//   cfg := exporter.Config{
//       Endpoint: "localhost:4317",
//       Insecure: true,
//   }
//   p, err := exporter.New(cfg)
//   if err != nil { return err }
//   defer p.Shutdown(context.Background())
//
//   tracer := p.TracerProvider().Tracer("my-component")
//   _, span := tracer.Start(ctx, "operation")
//   span.End()
//
// Configuration priority (high to low):
//   1. Direct Config struct fields
//   2. OTEL_EXPORTER_OTLP_* environment variables (via ConfigFromEnv)
//   3. Built-in defaults (Endpoint=localhost:4317, Timeout=10s, ...)
//
// Use MergeWith to combine configs in priority order.
//
// For sharing a single Pipeline across a k6 VU lifecycle, see GetShared.
package exporter
```

### 依存
- なし (パッケージ comment のみ)

---

## LC-1: Config & Validation (`config.go`)

### 責務
- `Protocol` / `Config` 型定義
- `Config.Validate()` — 各 field の制約検証 (`*ConfigError` を `errors.Join`)
- `Config.MergeWith(override)` — Config の優先順位 merge
- `ConfigFromEnv()` — `OTEL_EXPORTER_OTLP_*` 環境変数から Config 組み立て
- `Config.fillDefaults()` — zero value を built-in default で埋める (内部 method)

### 公開 API
```go
type Protocol int

const (
    ProtocolGRPC Protocol = iota
    ProtocolHTTP
)

func (p Protocol) String() string

type Config struct {
    Protocol          Protocol
    Endpoint          string
    Headers           map[string]string
    Insecure          bool
    Compression       string
    Timeout           time.Duration
    BatchSize         int
    BatchTimeout      time.Duration
    MaxQueueSize      int
    ResourceOverrides map[string]string
}

func (c Config) Validate() error
func (c Config) MergeWith(override Config) Config
func ConfigFromEnv() Config
```

### 実装スケッチ
```go
// 内部: built-in defaults
var defaultConfig = Config{
    Protocol:     ProtocolGRPC,
    Endpoint:     "localhost:4317",
    Insecure:     false,
    Compression:  "",
    Timeout:      10 * time.Second,
    BatchSize:    512,
    BatchTimeout: time.Second,
    MaxQueueSize: 2048,
}

func (c Config) fillDefaults() Config {
    if c.Endpoint == ""        { c.Endpoint = defaultConfig.Endpoint }
    if c.Timeout == 0          { c.Timeout = defaultConfig.Timeout }
    if c.BatchSize == 0        { c.BatchSize = defaultConfig.BatchSize }
    if c.BatchTimeout == 0     { c.BatchTimeout = defaultConfig.BatchTimeout }
    if c.MaxQueueSize == 0     { c.MaxQueueSize = defaultConfig.MaxQueueSize }
    return c
}

func (c Config) Validate() error {
    var errs []error
    if c.Protocol != ProtocolGRPC && c.Protocol != ProtocolHTTP {
        errs = append(errs, &ConfigError{Field: "Protocol", Value: c.Protocol, Message: "must be ProtocolGRPC or ProtocolHTTP"})
    }
    if c.Endpoint == "" {
        errs = append(errs, &ConfigError{Field: "Endpoint", Value: c.Endpoint, Message: "must not be empty"})
    }
    if c.Timeout <= 0 {
        errs = append(errs, &ConfigError{Field: "Timeout", Value: c.Timeout, Message: "must be > 0"})
    }
    if c.BatchSize <= 0 {
        errs = append(errs, &ConfigError{Field: "BatchSize", Value: c.BatchSize, Message: "must be > 0"})
    }
    if c.MaxQueueSize < c.BatchSize {
        errs = append(errs, &ConfigError{Field: "MaxQueueSize", Value: c.MaxQueueSize, Message: "must be >= BatchSize"})
    }
    if c.Compression != "" && c.Compression != "gzip" {
        errs = append(errs, &ConfigError{Field: "Compression", Value: c.Compression, Message: `must be "" or "gzip"`})
    }
    // Headers / ResourceOverrides の key/value check も同様
    return errors.Join(errs...)
}

func (c Config) MergeWith(override Config) Config {
    if override.Protocol != ProtocolGRPC      { c.Protocol = override.Protocol }
    if override.Endpoint != ""                { c.Endpoint = override.Endpoint }
    if override.Headers != nil                { c.Headers = override.Headers }
    if override.Insecure                      { c.Insecure = override.Insecure }
    if override.Compression != ""             { c.Compression = override.Compression }
    if override.Timeout != 0                  { c.Timeout = override.Timeout }
    if override.BatchSize > 0                 { c.BatchSize = override.BatchSize }
    if override.BatchTimeout > 0              { c.BatchTimeout = override.BatchTimeout }
    if override.MaxQueueSize > 0              { c.MaxQueueSize = override.MaxQueueSize }
    if override.ResourceOverrides != nil      { c.ResourceOverrides = override.ResourceOverrides }
    return c
}

func ConfigFromEnv() Config {
    var c Config
    // OTEL_EXPORTER_OTLP_ENDPOINT / TRACES_ENDPOINT / METRICS_ENDPOINT / LOGS_ENDPOINT
    // OTEL_EXPORTER_OTLP_PROTOCOL ("grpc" / "http/protobuf")
    // OTEL_EXPORTER_OTLP_HEADERS ("key1=v1,key2=v2")
    // OTEL_EXPORTER_OTLP_INSECURE ("true" / "false")
    // OTEL_EXPORTER_OTLP_COMPRESSION ("gzip")
    // OTEL_EXPORTER_OTLP_TIMEOUT (ms)
    // signal-specific が同一値なら採用、不一致時は汎用を優先 (Lint で警告 -- 将来)
    ...
    return c
}
```

### 依存
- 標準ライブラリ (`errors`, `os`, `strconv`, `strings`, `time`)
- LC-7 (`ConfigError`)

---

## LC-2: Resource Builder (`resource.go`)

### 責務
- OTel Resource の構築
- 自動 detector (`FromEnv`, `Host`, `Process`, `OS`) の有効化
- `cfg.ResourceOverrides` を override として merge (override 優先)

### 公開 API
- なし (内部関数 `buildResource(cfg) (*sdkresource.Resource, error)`)

### 実装スケッチ
```go
func buildResource(ctx context.Context, cfg Config) (*sdkresource.Resource, error) {
    detected, err := sdkresource.New(ctx,
        sdkresource.WithFromEnv(),
        sdkresource.WithHost(),
        sdkresource.WithProcess(),
        sdkresource.WithOS(),
    )
    if err != nil {
        return nil, fmt.Errorf("resource: auto-detect: %w", err)
    }

    if len(cfg.ResourceOverrides) == 0 {
        return detected, nil
    }

    attrs := make([]attribute.KeyValue, 0, len(cfg.ResourceOverrides))
    for k, v := range cfg.ResourceOverrides {
        attrs = append(attrs, attribute.String(k, v))
    }
    override := sdkresource.NewSchemaless(attrs...)

    merged, err := sdkresource.Merge(detected, override)
    if err != nil {
        return nil, fmt.Errorf("resource: merge: %w", err)
    }
    return merged, nil
}
```

### 依存
- `go.opentelemetry.io/otel/sdk/resource`
- `go.opentelemetry.io/otel/attribute`

---

## LC-3: Exporter Factory (`exporters.go`)

### 責務
- 3 信号 × 2 protocol = 6 種類の OTel SDK exporter を構築
- `cfg.Protocol` で分岐、Endpoint / Headers / Insecure / Compression / Timeout を SDK option として渡す
- 内側の exporter を LC-4 の wrapper で包む (Stats 更新のため)

### 公開 API
- なし (内部関数群)

### 実装スケッチ
```go
func buildTraceExporter(ctx context.Context, cfg Config, stats *pipelineStats) (sdktrace.SpanExporter, error) {
    var inner sdktrace.SpanExporter
    var err error
    switch cfg.Protocol {
    case ProtocolGRPC:
        opts := []otlptracegrpc.Option{
            otlptracegrpc.WithEndpoint(cfg.Endpoint),
            otlptracegrpc.WithHeaders(cfg.Headers),
            otlptracegrpc.WithTimeout(cfg.Timeout),
        }
        if cfg.Insecure { opts = append(opts, otlptracegrpc.WithInsecure()) }
        if cfg.Compression == "gzip" { opts = append(opts, otlptracegrpc.WithCompressor("gzip")) }
        inner, err = otlptracegrpc.New(ctx, opts...)
    case ProtocolHTTP:
        opts := []otlptracehttp.Option{
            otlptracehttp.WithEndpoint(cfg.Endpoint),
            otlptracehttp.WithHeaders(cfg.Headers),
            otlptracehttp.WithTimeout(cfg.Timeout),
        }
        if cfg.Insecure { opts = append(opts, otlptracehttp.WithInsecure()) }
        if cfg.Compression == "gzip" { opts = append(opts, otlptracehttp.WithCompression(otlptracehttp.GzipCompression)) }
        inner, err = otlptracehttp.New(ctx, opts...)
    }
    if err != nil { return nil, err }
    return &tracingExporter{inner: inner, stats: stats}, nil
}

// buildMetricExporter / buildLogExporter も同様 (型が異なるだけ)
```

### 依存
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/{otlptracegrpc,otlptracehttp}`
- `go.opentelemetry.io/otel/exporters/otlp/{otlpmetricgrpc,otlpmetrichttp}`
- `go.opentelemetry.io/otel/exporters/otlp/otlplog/{otlploggrpc,otlploghttp}`
- LC-4 (wrapper 型)

---

## LC-4: Stats & Instrumentation (`stats.go`)

### 責務
- `pipelineStats` 構造体 (atomic counter 群)
- 信号別 instrumented wrapper (`tracingExporter`, `metricExporter`, `loggingExporter`)
- 各 wrapper の `Export*` メソッドで Stats を更新 (Q1=A, Q2=A)

### 公開 API
- なし (内部型のみ。`Stats` は LC-5 で公開)

### 実装スケッチ
```go
type pipelineStats struct {
    tracesExported  atomic.Int64
    tracesFailed    atomic.Int64
    metricsExported atomic.Int64
    metricsFailed   atomic.Int64
    logsExported    atomic.Int64
    logsFailed      atomic.Int64
}

func (s *pipelineStats) snapshot() Stats {
    return Stats{
        TracesExported:  s.tracesExported.Load(),
        TracesFailed:    s.tracesFailed.Load(),
        MetricsExported: s.metricsExported.Load(),
        MetricsFailed:   s.metricsFailed.Load(),
        LogsExported:    s.logsExported.Load(),
        LogsFailed:      s.logsFailed.Load(),
    }
}

// --- Traces ---
type tracingExporter struct {
    inner sdktrace.SpanExporter
    stats *pipelineStats
}

func (e *tracingExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
    if err := e.inner.ExportSpans(ctx, spans); err != nil {
        e.stats.tracesFailed.Add(1)
        return err
    }
    e.stats.tracesExported.Add(int64(len(spans)))
    return nil
}
func (e *tracingExporter) Shutdown(ctx context.Context) error { return e.inner.Shutdown(ctx) }

// --- Metrics ---
type metricExporter struct {
    inner sdkmetric.Exporter
    stats *pipelineStats
}

func (e *metricExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
    n := countMetricDataPoints(rm) // ResourceMetrics 内の data point 数
    if err := e.inner.Export(ctx, rm); err != nil {
        e.stats.metricsFailed.Add(1)
        return err
    }
    e.stats.metricsExported.Add(int64(n))
    return nil
}
// Aggregation / Temporality / ForceFlush / Shutdown は inner に委譲
func (e *metricExporter) Aggregation(k sdkmetric.InstrumentKind) sdkmetric.Aggregation { return e.inner.Aggregation(k) }
func (e *metricExporter) Temporality(k sdkmetric.InstrumentKind) metricdata.Temporality { return e.inner.Temporality(k) }
func (e *metricExporter) ForceFlush(ctx context.Context) error { return e.inner.ForceFlush(ctx) }
func (e *metricExporter) Shutdown(ctx context.Context) error   { return e.inner.Shutdown(ctx) }

// --- Logs ---
type loggingExporter struct {
    inner sdklog.Exporter
    stats *pipelineStats
}

func (e *loggingExporter) Export(ctx context.Context, records []sdklog.Record) error {
    if err := e.inner.Export(ctx, records); err != nil {
        e.stats.logsFailed.Add(1)
        return err
    }
    e.stats.logsExported.Add(int64(len(records)))
    return nil
}
func (e *loggingExporter) ForceFlush(ctx context.Context) error { return e.inner.ForceFlush(ctx) }
func (e *loggingExporter) Shutdown(ctx context.Context) error   { return e.inner.Shutdown(ctx) }
```

### 依存
- `sync/atomic`
- OTel SDK 型 (`sdktrace.SpanExporter`, `sdkmetric.Exporter`, `sdklog.Exporter`)

---

## LC-5: Pipeline (`pipeline.go`)

### 責務
- `*Pipeline` 構造体定義
- `New(cfg)` — Config → Pipeline 構築 (partial failure 時は cleanup)
- `(*Pipeline).Shutdown(ctx)` — 3 Provider を一括 Shutdown (`sync.Once`)
- `(*Pipeline).Stats()` — `pipelineStats.snapshot()` を呼び出して返す
- `(*Pipeline).TracerProvider()` / `MeterProvider()` / `LoggerProvider()` — accessor
- `Stats` 型 (公開)

### 公開 API
```go
type Stats struct {
    TracesExported  int64
    TracesFailed    int64
    MetricsExported int64
    MetricsFailed   int64
    LogsExported    int64
    LogsFailed      int64
}

type Pipeline struct { /* unexported */ }

func New(cfg Config) (*Pipeline, error)
func (p *Pipeline) TracerProvider() trace.TracerProvider
func (p *Pipeline) MeterProvider() metric.MeterProvider
func (p *Pipeline) LoggerProvider() log.LoggerProvider
func (p *Pipeline) Shutdown(ctx context.Context) error
func (p *Pipeline) Stats() Stats
```

### 実装スケッチ
```go
type Pipeline struct {
    tp    *sdktrace.TracerProvider
    mp    *sdkmetric.MeterProvider
    lp    *sdklog.LoggerProvider
    res   *sdkresource.Resource
    stats *pipelineStats

    shutdownOnce sync.Once
    shutdownErr  error
}

func New(cfg Config) (*Pipeline, error) {
    cfg = cfg.fillDefaults()
    if err := cfg.Validate(); err != nil {
        return nil, &PipelineError{Stage: "validate", Inner: err}
    }

    ctx := context.Background()
    res, err := buildResource(ctx, cfg)
    if err != nil {
        return nil, &PipelineError{Stage: "resource", Inner: err}
    }

    stats := &pipelineStats{}

    traceExp, err := buildTraceExporter(ctx, cfg, stats)
    if err != nil {
        return nil, &PipelineError{Stage: "trace_exporter", Inner: err}
    }
    metricExp, err := buildMetricExporter(ctx, cfg, stats)
    if err != nil {
        _ = traceExp.Shutdown(context.Background()) // best-effort cleanup, Q10=A
        return nil, &PipelineError{Stage: "metric_exporter", Inner: err}
    }
    logExp, err := buildLogExporter(ctx, cfg, stats)
    if err != nil {
        _ = traceExp.Shutdown(context.Background())
        _ = metricExp.Shutdown(context.Background())
        return nil, &PipelineError{Stage: "log_exporter", Inner: err}
    }

    tp := sdktrace.NewTracerProvider(
        sdktrace.WithResource(res),
        sdktrace.WithBatcher(traceExp,
            sdktrace.WithMaxQueueSize(cfg.MaxQueueSize),
            sdktrace.WithMaxExportBatchSize(cfg.BatchSize),
            sdktrace.WithBatchTimeout(cfg.BatchTimeout),
        ),
    )
    mp := sdkmetric.NewMeterProvider(
        sdkmetric.WithResource(res),
        sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp,
            sdkmetric.WithInterval(cfg.BatchTimeout),
        )),
    )
    lp := sdklog.NewLoggerProvider(
        sdklog.WithResource(res),
        sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp,
            sdklog.WithMaxQueueSize(cfg.MaxQueueSize),
            sdklog.WithExportMaxBatchSize(cfg.BatchSize),
            sdklog.WithExportInterval(cfg.BatchTimeout),
        )),
    )

    return &Pipeline{
        tp: tp, mp: mp, lp: lp, res: res, stats: stats,
    }, nil
}

func (p *Pipeline) TracerProvider() trace.TracerProvider { return p.tp }
func (p *Pipeline) MeterProvider() metric.MeterProvider  { return p.mp }
func (p *Pipeline) LoggerProvider() log.LoggerProvider   { return p.lp }

func (p *Pipeline) Shutdown(ctx context.Context) error {
    p.shutdownOnce.Do(func() {
        p.shutdownErr = errors.Join(
            p.tp.Shutdown(ctx),
            p.mp.Shutdown(ctx),
            p.lp.Shutdown(ctx),
        )
    })
    return p.shutdownErr
}

func (p *Pipeline) Stats() Stats { return p.stats.snapshot() }
```

### 依存
- LC-1 (Config / Validate / fillDefaults)
- LC-2 (buildResource)
- LC-3 (build*Exporter)
- LC-4 (pipelineStats)
- LC-7 (PipelineError)
- OTel SDK (TracerProvider / MeterProvider / LoggerProvider)

---

## LC-6: Shared Holder (`shared.go`)

### 責務
- package-level singleton `*Pipeline` の管理
- `GetShared(factory)` — `sync.Once` で初回構築
- `SetShared(p)` — 事前構築 `*Pipeline` の差し込み (テスト用)
- `ResetShared()` — テスト用に状態リセット (Q6=A)

### 公開 API
```go
func GetShared(factory func() (*Pipeline, error)) (*Pipeline, error)
func SetShared(p *Pipeline) error
func ResetShared()
```

### 実装スケッチ
```go
var (
    sharedMu        sync.Mutex
    sharedOnce      sync.Once
    sharedPipeline  *Pipeline
    sharedInitErr   error
)

func GetShared(factory func() (*Pipeline, error)) (*Pipeline, error) {
    sharedOnce.Do(func() {
        sharedPipeline, sharedInitErr = factory()
    })
    return sharedPipeline, sharedInitErr
}

func SetShared(p *Pipeline) error {
    if p == nil {
        return &SharedError{Reason: "not_set"}
    }
    sharedMu.Lock()
    defer sharedMu.Unlock()
    if sharedPipeline != nil {
        return &SharedError{Reason: "already_initialized"}
    }
    var set bool
    sharedOnce.Do(func() {
        sharedPipeline = p
        set = true
    })
    if !set {
        return &SharedError{Reason: "already_initialized"}
    }
    return nil
}

// ResetShared resets the package-level Pipeline holder.
// This is intended for tests only; do not call from production code.
// Any in-flight goroutines holding the previous Pipeline reference must
// take responsibility for Shutdown.
func ResetShared() {
    sharedMu.Lock()
    defer sharedMu.Unlock()
    sharedOnce = sync.Once{}
    sharedPipeline = nil
    sharedInitErr = nil
}
```

### 依存
- LC-5 (`*Pipeline`)
- LC-7 (`SharedError`)
- `sync`

---

## LC-7: Errors (`errors.go`)

### 責務
- `PipelineError` / `ConfigError` / `SharedError` 型定義
- `Error()` / `Unwrap()` の実装

### 公開 API
```go
type PipelineError struct {
    Stage string // "validate" | "resource" | "trace_exporter" | "metric_exporter" | "log_exporter"
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
    Reason string // "already_initialized" | "init_failed" | "not_set"
    Inner  error
}
func (e *SharedError) Error() string
func (e *SharedError) Unwrap() error
```

### 実装スケッチ
```go
type PipelineError struct {
    Stage string
    Inner error
}
func (e *PipelineError) Error() string {
    return fmt.Sprintf("exporter: %s: %v", e.Stage, e.Inner)
}
func (e *PipelineError) Unwrap() error { return e.Inner }

type ConfigError struct {
    Field   string
    Value   any
    Message string
}
func (e *ConfigError) Error() string {
    return fmt.Sprintf("exporter: invalid Config.%s = %v: %s", e.Field, e.Value, e.Message)
}

type SharedError struct {
    Reason string
    Inner  error
}
func (e *SharedError) Error() string {
    if e.Inner != nil {
        return fmt.Sprintf("exporter: shared pipeline: %s: %v", e.Reason, e.Inner)
    }
    return fmt.Sprintf("exporter: shared pipeline: %s", e.Reason)
}
func (e *SharedError) Unwrap() error { return e.Inner }
```

### 依存
- `fmt`

---

## コンポーネント間依存図

```text
                    ┌──────────────────┐
                    │ LC-0 doc.go      │
                    └──────────────────┘
                            (no deps)

  ┌──────────────────┐    ┌──────────────────┐
  │ LC-1 config.go   │    │ LC-7 errors.go   │
  │ - Config         │◄───┤ - ConfigError    │
  │ - Validate       │    │ - PipelineError  │
  │ - MergeWith      │    │ - SharedError    │
  │ - ConfigFromEnv  │    └──────────────────┘
  └────────┬─────────┘             ▲
           │                       │
           ▼                       │
  ┌──────────────────┐    ┌──────────────────┐
  │ LC-2 resource.go │    │ LC-4 stats.go    │
  │ - buildResource  │    │ - pipelineStats  │
  └────────┬─────────┘    │ - tracingExp     │
           │              │ - metricExp      │
           │              │ - loggingExp     │
           │              └────────┬─────────┘
           │                       │
           ▼                       ▼
  ┌──────────────────────────────────────┐
  │ LC-3 exporters.go                    │
  │ - buildTraceExporter                 │
  │ - buildMetricExporter                │
  │ - buildLogExporter                   │
  └──────────────────┬───────────────────┘
                     │
                     ▼
  ┌──────────────────────────────────────┐
  │ LC-5 pipeline.go                     │
  │ - Pipeline / New / Shutdown / Stats  │
  │ - TracerProvider / Meter / Logger    │
  └──────────────────┬───────────────────┘
                     │
                     ▼
  ┌──────────────────────────────────────┐
  │ LC-6 shared.go                       │
  │ - GetShared / SetShared / ResetShared│
  └──────────────────────────────────────┘
```

---

## ビルド時の依存外部パッケージ

| 用途 | パッケージ |
|---|---|
| Trace SDK | `go.opentelemetry.io/otel/sdk/trace` |
| Metric SDK | `go.opentelemetry.io/otel/sdk/metric` |
| Log SDK | `go.opentelemetry.io/otel/sdk/log` |
| Resource | `go.opentelemetry.io/otel/sdk/resource` |
| Trace exporter (gRPC) | `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` |
| Trace exporter (HTTP) | `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` |
| Metric exporter (gRPC) | `go.opentelemetry.io/otel/exporters/otlp/otlpmetricgrpc` |
| Metric exporter (HTTP) | `go.opentelemetry.io/otel/exporters/otlp/otlpmetrichttp` |
| Log exporter (gRPC) | `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc` |
| Log exporter (HTTP) | `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp` |
| Public interfaces | `go.opentelemetry.io/otel/trace`, `metric`, `log`, `attribute` |
| PBT (test-only) | `pgregory.net/rapid` |
| Assertion (test-only) | `github.com/stretchr/testify` |

**Excluded**: `go.opentelemetry.io/otel/propagation` (in-process telemetry synthesis、外部リクエストへの context 伝搬不要)。

---

## テストコンポーネント (詳細は Code Generation Plan で確定)

| テストファイル | LC 対象 | テスト形式 |
|---|---|---|
| `config_test.go` | LC-1 | example-based + PBT (TP-U4-1, TP-U4-2) |
| `pipeline_test.go` | LC-5 | example-based (mockExporter 使用) |
| `shared_test.go` | LC-6 | example-based (ResetShared 活用) |
| `otlp_roundtrip_test.go` | (LC 外部、protobuf 確認) | PBT (TP-U4-3) |
| `stats_monotonic_test.go` | LC-4, LC-5 | stateful PBT (TP-U4-4) |
| `doc_test.go` | LC-0..LC-7 | Example function × 3 (Q12=A) |
| `bench_test.go` | LC-5 | BenchmarkNew (Q8=A, 固定 Config) |
| `integration/integration_test.go` | LC-5 全体 (E2E) | `-tags=integration`、Collector 起動・file 読み取り (Q5=A) |

---

## まとめ

- 8 production files + 7 test files (FD §3 の 5 test files + `doc_test.go` + `bench_test.go`)
- 各コンポーネントは Single Responsibility に従う
- 依存関係は単方向 (LC-0/7 → LC-1 → LC-2/LC-4 → LC-3 → LC-5 → LC-6)
- 公開 API は FD §4 で確定済の最小セット
- すべての設計判断が `nfr-design-patterns.md` の 12 質問回答と整合
