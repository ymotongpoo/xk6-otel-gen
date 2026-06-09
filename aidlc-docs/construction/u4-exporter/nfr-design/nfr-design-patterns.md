# U4 exporter — NFR Design Patterns

本書は U4 (`exporter/`) パッケージの **「どう実装するか」** のパターン群を確定する。
FD (`what`) と NFR-R (`what to achieve`) を受けて、Performance / Concurrency / Error / API / Documentation / Test の各カテゴリで実装パターンを決める。

参照:
- FD: `aidlc-docs/construction/u4-exporter/functional-design/`
- NFR-R: `aidlc-docs/construction/u4-exporter/nfr-requirements/`
- Plan + Answers: `aidlc-docs/construction/plans/u4-exporter-nfr-design-plan.md`

---

## 1. Performance パターン

### 1.1 Stats カウンタ更新 (NFR-U4-3 / Q1=A / Q2=A)

**パターン**: **per-signal wrapper + atomic counter**

3 つの信号別 wrapper 型を定義する (Q1=A)。Generics による汎用化は採用しない — SDK 側の interface 型 (`SpanExporter` / `Exporter` / `Exporter`) が異なり、自然な分離が信号別。

```go
// stats.go

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

// tracingExporter は sdktrace.SpanExporter をラップして stats を更新する。
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

// metricExporter / loggingExporter も同じ構造で signal 別実装。
```

**更新タイミング (Q2=A)**:
- `Export*(ctx, batch)` が `nil` 返却 → `*Exported.Add(len(batch))` (item 数を加算)
- `Export*` が `!= nil` 返却 → `*Failed.Add(1)` (batch 単位で 1 加算)
- これにより `Exported` counter は item 数、`Failed` counter は batch 数の意味

**理由**:
- `Exported` を item 数にすることで「何 span/metric/log を成功送信したか」が直接見える
- `Failed` を batch 数にすることで失敗頻度が分かる (item 数だと巨大 batch の失敗が誇張される)

### 1.2 Snapshot read (NFR-U4-7 / Q9=A)

`Stats()` は 6 個の `atomic.Int64.Load()` を順次呼んで構造体を返す。Mutex なし。field 間 atomic 一貫性は保証しない (`business-rules.md` §4.2)。

- 計算量: 6 × atomic.Load (~ ns オーダ) → 期待 < 1 µs (NFR-U4-7 §11)
- アロケーション: `Stats` struct 1 個のみ (escape しない場合 stack)

### 1.3 New 所要時間 (NFR-U4-7 / Q8=A)

`BenchmarkNew` は **固定 Config** をテスト先頭で定義し、毎 iteration 使う (Q8=A):

```go
var benchConfig = exporter.Config{
    Protocol:     exporter.ProtocolGRPC,
    Endpoint:     "localhost:4317",
    Insecure:     true,
    Timeout:      5 * time.Second,
    BatchSize:    512,
    BatchTimeout: time.Second,
    MaxQueueSize: 2048,
}

func BenchmarkNew(b *testing.B) {
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        p, err := exporter.New(benchConfig)
        if err != nil { b.Fatal(err) }
        _ = p.Shutdown(context.Background())
    }
}
```

理由: `rapid` を bench で使うと code generation の overhead が bias になる。Single-shape input で **「Pipeline 構築コスト」だけ** を測る。

### 1.4 QueueLen 取得 (Q3=X verified)

**決定**: Stats に `*QueueLen` フィールドを追加しない (FD で既に削除済)。

検証結果 (verified 2026-06, open-telemetry/opentelemetry-go upstream):
- `BatchSpanProcessor`: `OnStart` / `OnEnd` / `Shutdown` / `ForceFlush` / `MarshalLog` のみ exported。`Len()` 等の queue 観測 API なし
- `BatchProcessor` (log): `Enabled` / `OnEmit` / `Shutdown` / `ForceFlush`、`Len()` なし
- `PeriodicReader` (metric): pull-based、queue 概念なし

将来 SDK が公式 API を露出した時点で再評価。**「将来の互換性のため」という理由で構造体にフィールドを残さない**。

---

## 2. Concurrency パターン

### 2.1 Stats フィールド (NFR-U4-7)

各 counter は `atomic.Int64` を使用 (`sync.Mutex` ベースより速い)。Go 1.19+ の type-safe atomic を採用。

### 2.2 Shared Pipeline Holder (NFR-U4-6 / Q6=A)

```go
// shared.go

var (
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
    if sharedPipeline != nil {
        return &SharedError{Reason: "already_initialized"}
    }
    var ok bool
    sharedOnce.Do(func() {
        sharedPipeline = p
        ok = true
    })
    if !ok {
        return &SharedError{Reason: "already_initialized"}
    }
    return nil
}

// ResetShared はテスト用。Once と shared 状態を完全に再初期化。
func ResetShared() {
    sharedOnce = sync.Once{}
    sharedPipeline = nil
    sharedInitErr = nil
}
```

**Q6=A の決定理由**:
- `ResetShared()` を export し、各テストの冒頭で呼ぶ explicit pattern
- `testing.T.Cleanup` 自動登録ヘルパー (案 B) は便利だがマジック度が高く、テスト失敗時の debug が難しい
- production code 上 `ResetShared` が見えるのはやや汚いが、`Reset*ForTest()` という命名規約で意図を伝える

### 2.3 Pipeline.Shutdown の冪等性 (NFR-U4-3)

`*Pipeline` 内に `shutdownOnce sync.Once` と `shutdownErr error` を持つ:

```go
type Pipeline struct {
    tp *sdktrace.TracerProvider
    mp *sdkmetric.MeterProvider
    lp *sdklog.LoggerProvider
    res *sdkresource.Resource
    stats *pipelineStats

    shutdownOnce sync.Once
    shutdownErr  error
}

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
```

2 回目以降の Shutdown は first error を返す (Q5 / NFR-U4-3 §SLA)。

---

## 3. Error パターン

### 3.1 partial failure 時の cleanup (Q10=A)

`New(cfg)` 内で第 N 信号の Exporter 構築が失敗したら、既に作った N-1 個を Shutdown。

```go
func New(cfg Config) (*Pipeline, error) {
    cfg = cfg.fillDefaults()
    if err := cfg.Validate(); err != nil {
        return nil, &PipelineError{Stage: "validate", Inner: err}
    }
    res, err := buildResource(cfg)
    if err != nil {
        return nil, &PipelineError{Stage: "resource", Inner: err}
    }

    stats := &pipelineStats{}

    traceExp, err := buildTraceExporter(cfg, stats)
    if err != nil {
        return nil, &PipelineError{Stage: "trace_exporter", Inner: err}
    }
    metricExp, err := buildMetricExporter(cfg, stats)
    if err != nil {
        _ = traceExp.Shutdown(context.Background()) // cleanup, error 破棄 (Q10=A)
        return nil, &PipelineError{Stage: "metric_exporter", Inner: err}
    }
    logExp, err := buildLogExporter(cfg, stats)
    if err != nil {
        _ = traceExp.Shutdown(context.Background())
        _ = metricExp.Shutdown(context.Background())
        return nil, &PipelineError{Stage: "log_exporter", Inner: err}
    }
    // ... Provider 構築
}
```

**Q10=A の決定理由**:
- cleanup error を捨てる (Inner には主要 error のみ)
- 主要 error をマスクしない (`errors.Join` するとどれが root cause か分かりにくい)
- cleanup は best-effort、SDK の Shutdown は side-effect の cleanup 用で再試行する必要性は薄い

### 3.2 Error 型階層

3 種類の error 型 (`PipelineError` / `ConfigError` / `SharedError`) は全て `errors.As` で抽出可能。`errors.Join` も活用 (Shutdown の 3-provider 結果統合等)。

---

## 4. API パターン

### 4.1 Config 構築スタイル (Q7=A)

**plain struct literal** を主流に。Go の慣例的、シンプル:

```go
cfg := exporter.Config{
    Endpoint: "localhost:4317",
    Timeout:  5 * time.Second,
    Headers:  map[string]string{"api-key": "..."},
}
p, err := exporter.New(cfg)
```

理由:
- U7 generator も struct literal を生成する (Q12=A の `ValidConfig`)
- functional options (案 B) は便利だが Go では mature な struct literal を優先
- 両方サポート (案 C) は API 表面積が増える割に便益が小さい

### 4.2 Pipeline 内部表現 (Q9=A)

**直接 SDK Provider 型を保持**:

```go
type Pipeline struct {
    tp  *sdktrace.TracerProvider
    mp  *sdkmetric.MeterProvider
    lp  *sdklog.LoggerProvider
    res *sdkresource.Resource
    stats *pipelineStats

    shutdownOnce sync.Once
    shutdownErr  error
}
```

理由:
- interface ベース (案 B) は test の mock 差し込みやすさはあるが、SDK の concrete メソッド (Shutdown 等) に type assertion 必要
- mock 系のテスト要件は `pipelineStats` の wrap 構造で十分カバー (test では mockExporter を使う)

### 4.3 公開 API の最小性

FD §4 の API 一覧:
- Constructor: `New`
- Config: `ConfigFromEnv` / `MergeWith` / `Validate`
- Pipeline methods: `TracerProvider` / `MeterProvider` / `LoggerProvider` / `Shutdown` / `Stats`
- Shared: `GetShared` / `SetShared` / `ResetShared`
- Errors: `PipelineError` / `ConfigError` / `SharedError`

**追加禁止**:
- `NewWithFactory(cfg, factory)` のような test hook (Q4 案 B、API 汚染)
- functional options (Q7=A により不採用)

---

## 5. Documentation パターン

### 5.1 Example function (Q12=A)

`exporter/doc_test.go` に **3 件のみ**:

```go
// ExampleNew は exporter.New の基本利用例を示す。
func ExampleNew() {
    cfg := exporter.Config{
        Endpoint: "localhost:4317",
        Insecure: true,
    }
    p, err := exporter.New(cfg)
    if err != nil { log.Fatal(err) }
    defer p.Shutdown(context.Background())

    tracer := p.TracerProvider().Tracer("example")
    _, span := tracer.Start(context.Background(), "demo")
    span.End()

    // Output:
}

// ExampleConfig_MergeWith は Config の優先順位 merge を示す。
func ExampleConfig_MergeWith() {
    base := exporter.Config{Endpoint: "default:4317", Timeout: 10 * time.Second}
    override := exporter.Config{Endpoint: "override:4317"}
    merged := base.MergeWith(override)
    fmt.Println(merged.Endpoint, merged.Timeout)
    // Output: override:4317 10s
}

// ExampleGetShared は shared Pipeline の取得を示す。
func ExampleGetShared() {
    p, err := exporter.GetShared(func() (*exporter.Pipeline, error) {
        return exporter.New(exporter.Config{Endpoint: "localhost:4317", Insecure: true})
    })
    if err != nil { log.Fatal(err) }
    _ = p
}
```

**Q12=A の理由**: top-level の主要 API のみ。`ExamplePipeline_Shutdown` 等は doc.go の説明文で十分カバー。`Stats` は単純な struct return なので Example 過剰。

### 5.2 doc.go の構成

```go
// Package exporter provides an OTLP exporter pipeline for traces,
// metrics, and logs, sharing a single endpoint and resource across
// all three signals.
//
// Typical usage from a k6 extension:
//   ...
//
// Lifecycle:
//   ...
//
// Configuration priority (high to low):
//   1. JS API options
//   2. OTEL_EXPORTER_OTLP_* environment variables
//   3. YAML defaults (future)
//   4. Built-in defaults
package exporter
```

### 5.3 GoDoc の網羅性

`golint -set_exit_status` 相当 (NFR-U4-10): すべての exported identifier に doc comment。CI で `revive` を使って enforce。

---

## 6. Test パターン

### 6.1 Mock Exporter for Unit Test (Q4=A)

`exporter_test` パッケージ内に `mockExporter` を定義:

```go
// pipeline_test.go (package exporter_test)

type mockSpanExporter struct {
    mu      sync.Mutex
    spans   []sdktrace.ReadOnlySpan
    failNext bool
}

func (m *mockSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.failNext {
        m.failNext = false
        return errors.New("mock failure")
    }
    m.spans = append(m.spans, spans...)
    return nil
}
func (m *mockSpanExporter) Shutdown(ctx context.Context) error { return nil }

// テストで Pipeline を mock 構成で作るためのヘルパー (test 専用)。
func buildMockPipeline(t *testing.T) *exporter.Pipeline {
    // mockExporter を含む Pipeline を直接構築 (NewWithFactory ではなく、
    // SetShared(buildMockPipeline()) のようなパターンで差し込む)
    ...
}
```

**Q4=A の理由**:
- production API に test hook (`NewWithFactory`) を加えない (API 汚染回避)
- Real Collector を unit test で起動するのは integration test との境界が曖昧
- `mockExporter` は SDK の `SpanExporter` interface を満たすだけで、`exporter` パッケージとは疎結合

### 6.2 Integration Test Harness (Q5=A)

Collector が `/var/log/otel/*.json` に書き出す JSON を test 内で `os.ReadFile` する。

```text
exporter/
├── testdata/
│   └── collector-config.yaml      // file_exporter で /var/log/otel/*.json に書き出す設定
├── integration/
│   ├── docker-compose.yaml        // collector + volume mount /var/log/otel
│   ├── integration_test.go        // testcontainers or os/exec で compose 起動・停止
│   └── helpers.go                 // ReadCollectorTraces / ReadCollectorMetrics / ReadCollectorLogs
```

**Q5=A の理由**:
- file_exporter を中継するのは最もシンプル
- CI / 開発者ローカルで動作が共通 (test host 内で Docker volume を共有)
- 案 B/C は別途 server 実装が必要で複雑度増

実装は **integration build tag**:

```go
//go:build integration
// +build integration

package integration
```

`go test ./...` (default) では skip、`go test -tags=integration ./...` で実行。

### 6.3 Shared holder の test 分離 (Q6=A)

`ResetShared()` を export。各テストの冒頭で呼ぶ:

```go
func TestGetShared_CachesError(t *testing.T) {
    exporter.ResetShared()
    // ... テスト本体
}
```

Helper を活用するなら test-side で:

```go
func freshShared(t *testing.T) {
    exporter.ResetShared()
    t.Cleanup(exporter.ResetShared)
}
```

(これは test side の helper、production helper としては提供しない)

### 6.4 shared.go の sync.Once test 間リセット

`ResetShared()` の実装 (上記 §2.2) で `sharedOnce = sync.Once{}` と新規 instance に置換。
**注意**: package-level var を直接置換するので、別 goroutine が同時に `GetShared` を呼んでいる時 race の可能性 — テスト用なので serial 実行前提 (`-parallel 1` または subtest 内で慎重に)。

### 6.5 PBT (Property-Based Testing) パターン

- TP-U4-1 / TP-U4-2: `config_test.go` 内で `rapid.Check` + `generators.ValidConfig().Draw()`
- TP-U4-3: `otlp_roundtrip_test.go` で `go.opentelemetry.io/proto/otlp` の型を直接 generate
- TP-U4-4: `stats_monotonic_test.go` で stateful PBT (simulateExport を組み合わせ)

詳細実装は Code Generation Plan 段階で確定。

### 6.6 ファイル分割 (Q11=A)

8 production files + 5 test files をそのまま採用 (FD §3 で提案済):

```text
exporter/
├── doc.go
├── config.go
├── pipeline.go
├── shared.go
├── resource.go
├── exporters.go
├── stats.go
├── errors.go
├── config_test.go            // TP-U4-1, TP-U4-2 + example-based
├── pipeline_test.go          // example-based + mockExporter
├── shared_test.go            // shared holder invariants
├── otlp_roundtrip_test.go    // TP-U4-3
├── stats_monotonic_test.go   // TP-U4-4
├── doc_test.go               // Example functions (3 件)
├── bench_test.go             // BenchmarkNew
└── integration/              // -tags=integration
    ├── docker-compose.yaml
    └── integration_test.go
```

**Q11=A の理由**: 既に FD で議論した粒度がちょうど良い。`pipeline.go` の責務 (struct + New + Shutdown + Stats + Provider accessor) は ~250 行程度に収まる見込みで、分割不要。

---

## 7. 拡張ポイント (将来)

| 領域 | 現状 | 将来検討 |
|---|---|---|
| Sampler | `AlwaysSample` 固定 | `TraceIDRatioBased` 等を Config に追加 |
| Compression | `"" / "gzip"` | `"zstd"` 等 (SDK サポート次第) |
| QueueLen | `Stats` に含めない | SDK が `Len()` 公開時に再追加 |
| Functional options | 非採用 | 利用パターンを見て検討 |
| Lint API | なし | `Config.Lint() []Warning` |
| YAML defaults section | 非対応 | `topology.Schema.Exporter` で受領 |

---

## 8. NFR-R 各項目との対応

| NFR-R | 対応する Design パターン |
|---|---|
| NFR-U4-1 (API stability) | §4.1 struct literal、§4.3 公開 API 最小性 |
| NFR-U4-2 (Lifecycle) | §2.3 Shutdown 冪等性 |
| NFR-U4-3 (Concurrency) | §2.1 atomic counter、§2.2 sync.Once |
| NFR-U4-4 (Error contract) | §3.1 partial cleanup、§3.2 error 型階層 |
| NFR-U4-5 (Config merge) | §4.1 struct literal、§6.5 PBT TP-U4-1/2 |
| NFR-U4-6 (Shared holder) | §2.2 ResetShared export |
| NFR-U4-7 (Performance) | §1.2 Snapshot read、§1.3 BenchmarkNew |
| NFR-U4-8 (Observability) | §1.1 Stats 更新タイミング |
| NFR-U4-9 (Resource) | §3.1 buildResource ステージ |
| NFR-U4-10 (Documentation) | §5 Example + GoDoc |
| NFR-U4-11 (Testability) | §6.1 mockExporter、§6.2 integration harness |
| NFR-U4-12 (Compatibility) | §7 拡張ポイント (将来の API 互換維持戦略) |

---

## 9. 依存外部パッケージ (確認)

| Package | 用途 |
|---|---|
| `go.opentelemetry.io/otel/sdk/trace` | TracerProvider 構築 |
| `go.opentelemetry.io/otel/sdk/metric` | MeterProvider 構築 |
| `go.opentelemetry.io/otel/sdk/log` | LoggerProvider 構築 |
| `go.opentelemetry.io/otel/sdk/resource` | Resource 構築 |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/{otlptracegrpc,otlptracehttp}` | Trace exporter |
| `go.opentelemetry.io/otel/exporters/otlp/{otlpmetricgrpc,otlpmetrichttp}` | Metric exporter |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog/{otlploggrpc,otlploghttp}` | Log exporter |
| `pgregory.net/rapid` | PBT (test-only) |
| `github.com/stretchr/testify` | assertion (test-only) |

`propagation` は **import しない** (in-process telemetry 生成、ユーザー RFC 確認済 — このツールより先にトレースを伝搬させる必要がない)。

---

## 10. アンチパターン (採用しない)

| アンチパターン | 不採用理由 |
|---|---|
| Generics 汎用 wrapper (Q1 案 B) | SDK の interface 型差が大きく、汎用化のメリットが薄い |
| `NewWithFactory(cfg, factory)` (Q4 案 B) | production API に test hook |
| Functional options + struct literal 両対応 (Q7 案 C) | API 表面積増、merit 不足 |
| QueueLen wrapper で並列カウンタ (Q3 案 B) | 実態とズレた値を公開すると誤解を招く |
| OTel SDK 内部 metric 読み (Q3 案 C) | バージョン依存、保守コスト高 |
| Real Collector unit test (Q4 案 C) | integration test との境界が曖昧 |
| `testing.T.Cleanup` 自動ヘルパー (Q6 案 B) | マジック度高い、debug 困難 |
| `errors.Join` で cleanup error 集約 (Q10 案 B) | 主要 error がマスクされる |
| `pipeline.go` を build と struct で分割 (Q11 案 B) | 規模的に分割不要 |
| 信号別ファイル分割 (Q11 案 C) | 共通の wrapper パターン重複が増える |

---

## 11. 設計確定 — Open Items なし

12 質問すべて確定。Q3 は SDK API 検証済 (open-telemetry/opentelemetry-go upstream)。`logical-components.md` で各論理コンポーネントの詳細を確定する。
