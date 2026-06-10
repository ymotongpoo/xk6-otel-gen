# U6 k6output — Logical Components

本書は `k6output/` 内の **論理コンポーネント** (LC) を確定する。各 LC について 責務 / 公開 API / 実装スケッチ / 依存関係 を定義。

参照:
- FD: `aidlc-docs/construction/u6-k6output/functional-design/`
- NFR Design Patterns: `nfr-design-patterns.md` (本ディレクトリ内)

---

## コンポーネント一覧

| LC | 名前 | ファイル | 責務 |
|---|---|---|---|
| LC-0 | Package Documentation | `doc.go` | パッケージ overview + `--out` args reference + dual-function 説明 |
| LC-1 | Output Lifecycle | `output.go` | Output struct + New + Description + Start + Stop + init() 登録 + flushLoop + runner Resource builder |
| LC-2 | Args Parser | `params.go` | Params struct + parseOutArgs + applyKV + parseHeaders + defaultParams |
| LC-3 | Sample Converter | `convert.go` | k6 Sample → OTel metric emit + tagSetCache + instrumentMap + lookupOrBuildInstrument |
| LC-4 | Errors | `errors.go` | ConfigError type + helpers |

---

## LC-0: Package Documentation (`doc.go`)

### 責務
- パッケージ overview
- `--out otel-gen=...` usage example + args reference table
- Dual-function explanation (shutdown trigger + k6 metric conversion)
- Config priority documentation
- High-cardinality risk note

### 実装スケッチ
NFR Design Patterns §5.1 参照。

### 依存
- なし

---

## LC-1: Output Lifecycle (`output.go`)

### 責務
- `Output` 構造体 (NFR-D §1.1)
- `New(params output.Params) (output.Output, error)` factory
- `init()` で `output.RegisterExtension("otel-gen", New)`
- `Description() string`
- `Start() error` (sync.Once-guarded, builds Pipeline + runner MeterProvider + instruments + queue + flush goroutine)
- `AddMetricSamples(samples []metrics.SampleContainer)` (non-blocking queue push)
- `Stop() error` (sync.Once-guarded, drain + Pipeline.Shutdown, always returns nil)
- `flushLoop()` goroutine body (NFR-D §2.2)
- `tryPush(s)` non-blocking queue push with drop-oldest
- `buildRunnerResource(params)` private helper

### 公開 API
```go
func New(params output.Params) (output.Output, error)

func (o *Output) Description() string
func (o *Output) Start() error
func (o *Output) AddMetricSamples(samples []metrics.SampleContainer)
func (o *Output) Stop() error
```

### 実装スケッチ
NFR Design Patterns §1.1, §2 全般、§4.2 参照。

### 依存
- LC-2 (Params)
- LC-3 (convert + instrumentMap + tagSetCache)
- LC-4 (ConfigError)
- `go.k6.io/k6/output`, `go.k6.io/k6/metrics`
- `go.opentelemetry.io/otel/sdk/metric`, `metric`, `sdk/resource`
- `go.opentelemetry.io/otel/semconv/v1.27.0`
- `github.com/ymotongpoo/xk6-otel-gen/exporter` (with U4 patch: `Pipeline.MetricExporter()`)
- stdlib: `context`, `sync`, `time`, `os`, `runtime`, `fmt`, `log`, `path/filepath`

---

## LC-2: Args Parser (`params.go`)

### 責務
- `Params` 構造体
- `parseOutArgs(s string) (Params, error)` の main parser
- `applyKV(p *Params, key, val string) error` per-key dispatch
- `parseHeaders(s string) (map[string]string, error)`
- `defaultParams() Params`
- `queueSize` range validate ([10, 10000])

### 公開 API
- なし (internal helpers、LC-1 のみが呼ぶ)

### Params 型
```go
type Params struct {
    // OTLP exporter config (subset of exporter.Config)
    Endpoint     string
    Protocol     exporter.Protocol
    Insecure     bool
    Headers      map[string]string
    Compression  string
    Timeout      time.Duration
    BatchSize    int
    BatchTimeout time.Duration
    MaxQueueSize int

    // U6-specific config
    QueueSize     int           // queue channel buffer size
    FlushInterval time.Duration // flush ticker interval (NFR Design で default 1s)

    // k6 SDK-provided context
    ScriptPath string  // from output.Params.ScriptOptions or similar
}
```

### 実装スケッチ
NFR Design Patterns §4.1 参照。

### 依存
- LC-4 (ConfigError)
- `github.com/ymotongpoo/xk6-otel-gen/exporter` (Protocol const)
- `strings`, `strconv`, `time`, `fmt`

---

## LC-3: Sample Converter (`convert.go`)

### 責務
- `instrumentMap` 型 (sync.Map × 4: counters, histograms, gauges, upDowns)
- `tagSetCache` 型 (sync.Map keyed by tag-set hash)
- `hashTags(tags map[string]string) string`
- `lookupOrBuildInstrument(k6Name, k6Type, unitHint) any`
- `emitContainer(container metrics.SampleContainer)` — flush goroutine から呼ばれる
- `emitSample(sample metrics.Sample)` — 1 sample を instrument に Record
- `metricNameToOTel(k6Name string) string` — `http_req_duration` → `k6.http.request.duration`
- `k6MetricSpec` table + `knownK6Metrics` slice

### 公開 API
- なし (internal、LC-1 が呼ぶ)

### 実装スケッチ
NFR Design Patterns §1.2, §1.3, §1.4 参照。

```go
type k6MetricSpec struct {
    k6Name   string
    otelName string
    unit     string
    instType instrumentType
}

type instrumentType int

const (
    tInstCounter instrumentType = iota
    tInstHistogram
    tInstGauge
)

type instrumentMap struct {
    counters   sync.Map  // string → metric.Float64Counter
    histograms sync.Map  // string → metric.Float64Histogram
    gauges     sync.Map  // string → metric.Float64Gauge
}

// emitContainer flushes a single k6 SampleContainer's samples to OTel instruments.
func (o *Output) emitContainer(container metrics.SampleContainer) {
    for _, sample := range container.GetSamples() {
        o.emitSample(sample)
    }
}

func (o *Output) emitSample(s metrics.Sample) {
    if s.Metric == nil { return }
    k6Type := s.Metric.Type
    name := s.Metric.Name

    inst := o.lookupOrBuildInstrument(name, k6Type, k6UnitHint(name))
    if inst == nil { return }

    attrSet := o.setCache.get(s.Tags)
    ctx := context.Background()

    switch v := inst.(type) {
    case metric.Float64Counter:
        if k6Type == metrics.Rate && s.Value != 1 {
            return  // Rate: only count failures (value=1)
        }
        v.Add(ctx, s.Value, metric.WithAttributeSet(attrSet))
    case metric.Float64Histogram:
        v.Record(ctx, s.Value, metric.WithAttributeSet(attrSet))
    case metric.Float64Gauge:
        v.Record(ctx, s.Value, metric.WithAttributeSet(attrSet))
    }
}

func k6UnitHint(name string) string {
    // hardcoded mapping for known metrics (see NFR Design Patterns §1.2)
    switch name {
    case "http_req_duration", "iteration_duration", "group_duration":
        return "ms"
    case "data_sent", "data_received":
        return "By"
    case "iterations":
        return "{iteration}"
    case "vus", "vus_max":
        return "{vu}"
    }
    return ""  // unknown — no unit hint
}
```

### 依存
- LC-1 (Output struct を method receiver で)
- `go.k6.io/k6/metrics`
- `go.opentelemetry.io/otel/metric`, `attribute`
- `sync`, `sort`, `strings`

---

## LC-4: Errors (`errors.go`)

### 責務
- `ConfigError` 型 (FD §1.3、NFR-D §3.1)
- `Error()` + `Unwrap()`

### 公開 API
```go
type ConfigError struct {
    Kind  string  // 4-value enum
    Field string
    Value string
    Inner error
}

func (e *ConfigError) Error() string
func (e *ConfigError) Unwrap() error
```

### 実装スケッチ
NFR Design Patterns §3.1 参照。

### 依存
- `fmt`

---

## コンポーネント間依存図

```text
              ┌──────────────────┐
              │ LC-0 doc.go      │
              └──────────────────┘

              ┌──────────────────┐
              │ LC-4 errors.go   │ ◄──── (LC-1, LC-2 で使用)
              │ - ConfigError    │
              └──────────────────┘
                       ▲
                       │
              ┌────────┴─────────┐
              │ LC-2 params.go   │
              │ - Params         │
              │ - parseOutArgs   │
              │ - applyKV        │
              │ - parseHeaders   │
              │ - defaultParams  │
              └────────┬─────────┘
                       │
                       │ (LC-3 indirectly uses Params via LC-1)
                       │
              ┌────────▼─────────┐
              │ LC-3 convert.go  │
              │ - instrumentMap  │
              │ - tagSetCache    │
              │ - hashTags       │
              │ - emit{Container,│
              │   Sample}        │
              │ - lookupOrBuild- │
              │   Instrument     │
              │ - knownK6Metrics │
              └────────┬─────────┘
                       │
              ┌────────▼─────────┐
              │ LC-1 output.go   │
              │ - Output struct  │
              │ - New            │
              │ - Description    │
              │ - Start          │
              │ - AddMetric-     │
              │   Samples        │
              │ - Stop           │
              │ - flushLoop      │
              │ - tryPush        │
              │ - buildRunner-   │
              │   Resource       │
              │ - init() register│
              └──────────────────┘
```

---

## ビルド時の依存外部パッケージ

| 用途 | パッケージ |
|---|---|
| k6 Output SDK | `go.k6.io/k6/output` |
| k6 Metrics | `go.k6.io/k6/metrics` |
| OTel sdk/metric | `go.opentelemetry.io/otel/sdk/metric` (NewMeterProvider, NewPeriodicReader) |
| OTel metric interface | `go.opentelemetry.io/otel/metric` (Float64Counter, Histogram, Gauge) |
| OTel sdk/resource | `go.opentelemetry.io/otel/sdk/resource` (NewSchemaless) |
| OTel attribute | `go.opentelemetry.io/otel/attribute` (KeyValue, Set) |
| OTel semconv | `go.opentelemetry.io/otel/semconv/v1.27.0` (ServiceName 等) |
| Exporter | `github.com/ymotongpoo/xk6-otel-gen/exporter` (Config, GetShared, Pipeline, **NEW MetricExporter**) |
| stdlib | `context`, `sync`, `time`, `strings`, `strconv`, `sort`, `fmt`, `os`, `runtime`, `path/filepath`, `log`, `errors` |

**Excluded**: topology / journey / synth / k6otelgen, OTLP exporters (exporter package が抽象化), errgroup, JS runtime libs.

---

## テストコンポーネント (Code Generation 時に詳細化)

| テストファイル | LC 対象 | テスト形式 |
|---|---|---|
| `output_test.go` | LC-1 | example-based (lifecycle: New / Description / Start / Stop) |
| `params_test.go` | LC-2 | table-driven (各 key の decode、unknown key warn-ignore) |
| `convert_test.go` | LC-3 | table-driven (k6 metric → OTel instrument mapping、tag conversion) |
| `pbt_test.go` | LC-1, LC-3 | TP-U6-1 (example-based)、TP-U6-2, TP-U6-3 (rapid) |
| `helpers_test.go` | (全 LC 共通) | newTestParams / newTestOutput / mock ManualReader |
| `doc_test.go` | LC-0..LC-4 | 1 Example (ExampleNew) |
| `bench_test.go` | LC-1, LC-3 | BenchmarkAddMetricSamples / BenchmarkFlushLoop |
| `integration/integration_test.go` | LC 全体 (E2E) | `//go:build integration`、xk6 build + Docker Collector + 実 k6 run |
| `integration/helpers.go` | (integration 共通) | requireDocker / requireXK6 / buildK6Binary (U5 と類似 logic、コピー) |
| `integration/testdata/script.js` | (test fixture) | minimal k6 script with iterations / http_req simulation |
| `integration/testdata/topology.yaml` | (test fixture) | (optional) topology for full E2E with U5 + U6 |
| `integration/testdata/collector-config.yaml` | (test fixture) | Collector config |
| `integration/testdata/docker-compose.yaml` | (test fixture) | Docker compose |

---

## U4 coordination 要件 (再掲)

U6 NFR Design は U4 に対し以下の patch を要求:

```go
// In exporter/pipeline.go (NEW method):

// MetricExporter returns the underlying OTLP metric exporter used by this
// Pipeline. Intended for k6output to construct an additional MeterProvider
// with a different Resource while sharing the same OTLP connection.
//
// The returned exporter is owned by the Pipeline; callers must NOT call
// Shutdown on it directly — use Pipeline.Shutdown for unified lifecycle.
func (p *Pipeline) MetricExporter() sdkmetric.Exporter {
    return p.metricExp  // existing internal field
}
```

- **U4 への影響**: 新規 method 追加のみ (minor SemVer bump、backward-compatible)
- **U4 docs update**: NFR-D `logical-components.md` §LC-5 (Pipeline) で API table に追加
- **Code Generation Plan**: 「Phase: Add Pipeline.MetricExporter to exporter」を独立 phase として登録 (U2 NewEngineWithSeed pattern と同じ)

---

## まとめ

- **5 production files** (FD §3 と一致)
- **9 test files** (helpers / doc / bench + integration 子 dir 含む)
- 各 LC は Single Responsibility
- 依存関係は単方向 (LC-0/4 → LC-2 / LC-3 → LC-1)
- 公開 API は k6 SDK contract (`New`, `Output.{Description,Start,AddMetricSamples,Stop}`) のみ
- **U4 patch (`Pipeline.MetricExporter()`)** が Code Generation Plan の Phase で前提となる
- runner Resource (`service.name="xk6-otel-gen-runner"`) で synth の per-service Resource と完全分離
- hot path 最適化 (tagSetCache + instrumentMap sync.Map) で NFR-U6-3 strict budget 達成
