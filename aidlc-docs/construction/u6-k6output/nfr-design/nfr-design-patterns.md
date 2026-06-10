# U6 k6output — NFR Design Patterns

本書は U6 (`k6output/`) の **「どう実装するか」** のパターン群を確定する。FD + NFR-R を受けて、Performance / Concurrency / Error / API / Documentation / Test の各カテゴリで実装パターンを決める。

参照:
- FD: `aidlc-docs/construction/u6-k6output/functional-design/`
- NFR-R: `aidlc-docs/construction/u6-k6output/nfr-requirements/`
- Plan + Answers: `aidlc-docs/construction/plans/u6-k6output-nfr-d-plan.md`

---

## 1. Performance パターン

### 1.1 Output struct layout (Q1=A)

```go
type Output struct {
    // Config (immutable after New)
    params         Params
    runnerResource *resource.Resource

    // Lifecycle (sync.Once guarded)
    startOnce sync.Once
    startErr  error
    stopOnce  sync.Once

    // Emission (set in Start, used by flush)
    pipeline      *exporter.Pipeline
    meterProvider *sdkmetric.MeterProvider
    instruments   instrumentMap   // metric name → instrument
    setCache      *tagSetCache    // attribute set cache

    // Queue (set in Start)
    queue     chan metrics.SampleContainer
    ctx       context.Context
    cancelFn  context.CancelFunc
    flushDone chan struct{}
    drops     atomic.Uint64

    // Logger for warn messages
    logger func(format string, args ...any)  // injected, defaults to log.Printf
}
```

理由:
- group 化で意図が読みやすい
- sync.Once は emit / shutdown を deterministic に
- `drops` を atomic.Uint64 で counter (no self-metric expose per Q6=A NFR-R)
- `logger` は injectable for testability

### 1.2 Instrument 構築戦略 (Q3=A + Q4=A)

```go
type instrumentMap struct {
    counters   sync.Map  // string → metric.Float64Counter
    histograms sync.Map  // string → metric.Float64Histogram
    gauges     sync.Map  // string → metric.Float64Gauge
    upDowns    sync.Map  // string → metric.Int64UpDownCounter (for k6 Gauge as UDC fallback)
}

var knownK6Metrics = []k6MetricSpec{
    {name: "http_req_duration", otelName: "k6.http.request.duration", unit: "ms", instType: tInstHistogram},
    {name: "http_req_failed",   otelName: "k6.http.request.failed.total", unit: "{request}", instType: tInstCounter},
    {name: "http_reqs",         otelName: "k6.http.requests.total", unit: "{request}", instType: tInstCounter},
    {name: "iterations",        otelName: "k6.iterations.total", unit: "{iteration}", instType: tInstCounter},
    {name: "iteration_duration", otelName: "k6.iteration.duration", unit: "ms", instType: tInstHistogram},
    {name: "vus",               otelName: "k6.vus", unit: "{vu}", instType: tInstGauge},
    {name: "vus_max",           otelName: "k6.vus.max", unit: "{vu}", instType: tInstGauge},
    {name: "data_sent",         otelName: "k6.data.sent.total", unit: "By", instType: tInstCounter},
    {name: "data_received",     otelName: "k6.data.received.total", unit: "By", instType: tInstCounter},
    {name: "checks",            otelName: "k6.checks.total", unit: "{check}", instType: tInstCounter},
    {name: "group_duration",    otelName: "k6.group.duration", unit: "ms", instType: tInstHistogram},
}

func (o *Output) buildKnownInstruments() error {
    meter := o.meterProvider.Meter("github.com/ymotongpoo/xk6-otel-gen/k6output")
    for _, spec := range knownK6Metrics {
        switch spec.instType {
        case tInstCounter:
            c, err := meter.Float64Counter(spec.otelName, metric.WithUnit(spec.unit))
            if err != nil { return fmt.Errorf("build %s: %w", spec.otelName, err) }
            o.instruments.counters.Store(spec.name, c)
        case tInstHistogram:
            h, err := meter.Float64Histogram(spec.otelName, metric.WithUnit(spec.unit))
            if err != nil { return fmt.Errorf("build %s: %w", spec.otelName, err) }
            o.instruments.histograms.Store(spec.name, h)
        case tInstGauge:
            // OTel SDK Go の Float64Gauge は v1.31+ で stable、Available なら使用
            g, err := meter.Float64Gauge(spec.otelName, metric.WithUnit(spec.unit))
            if err != nil { return fmt.Errorf("build %s: %w", spec.otelName, err) }
            o.instruments.gauges.Store(spec.name, g)
        }
    }
    return nil
}
```

### 1.3 Lazy instrument for unknown metric

```go
func (o *Output) lookupOrBuildInstrument(k6MetricName string, k6Type metrics.MetricType, unitHint string) any {
    switch k6Type {
    case metrics.Counter, metrics.Rate:
        if v, ok := o.instruments.counters.Load(k6MetricName); ok { return v }
        // lazy build
        meter := o.meterProvider.Meter("github.com/ymotongpoo/xk6-otel-gen/k6output")
        otelName := "k6." + dotted(k6MetricName)
        if k6Type == metrics.Rate { otelName += ".total" }
        c, err := meter.Float64Counter(otelName, metric.WithUnit(unitHint))
        if err != nil { o.logger("k6output: lazy build %s: %v", otelName, err); return nil }
        actual, _ := o.instruments.counters.LoadOrStore(k6MetricName, c)
        return actual
    // 同様に Histogram / Gauge
    }
    return nil
}

func dotted(s string) string {
    return strings.ReplaceAll(s, "_", ".")
}
```

### 1.4 tagSetCache (Q9=A + Q10=A)

```go
type tagSetCache struct {
    sets sync.Map  // hashed key → attribute.Set
}

func hashTags(tags map[string]string) string {
    if len(tags) == 0 { return "" }
    keys := make([]string, 0, len(tags))
    for k := range tags { keys = append(keys, k) }
    sort.Strings(keys)
    var b strings.Builder
    for _, k := range keys {
        b.WriteString(k); b.WriteByte('=')
        b.WriteString(tags[k]); b.WriteByte(';')
    }
    return b.String()
}

func (c *tagSetCache) get(tags map[string]string) attribute.Set {
    h := hashTags(tags)
    if v, ok := c.sets.Load(h); ok { return v.(attribute.Set) }
    kvs := make([]attribute.KeyValue, 0, len(tags))
    for k, v := range tags {
        kvs = append(kvs, attribute.String("k6.tag."+k, v))
    }
    set := attribute.NewSet(kvs...)
    actual, _ := c.sets.LoadOrStore(h, set)
    return actual.(attribute.Set)
}
```

### 1.5 Hot path 最適化の per-sample budget

NFR-U6-3 < 1 µs (queue push), < 5 µs (flush emit) を達成するため:
- queue push: select default で sync.Map / lock なし
- flush emit: tagSetCache hit → 0 allocation、instrument lookup → sync.Map.Load (lock free)
- attribute set 構築は initial cache miss のみ

---

## 2. Concurrency パターン

### 2.1 sync.Once-guarded lifecycle (Q5=A)

```go
func (o *Output) Start() error {
    o.startOnce.Do(func() {
        // 1. acquire Pipeline (lazy via GetShared)
        pipeline, err := exporter.GetShared(func() (*exporter.Pipeline, error) {
            return exporter.New(buildConfig(o.params))
        })
        if err != nil { o.startErr = fmt.Errorf("k6output: pipeline init: %w", err); return }
        o.pipeline = pipeline

        // 2. build runner MeterProvider with shared MetricExporter
        reader := sdkmetric.NewPeriodicReader(pipeline.MetricExporter())
        o.meterProvider = sdkmetric.NewMeterProvider(
            sdkmetric.WithResource(o.runnerResource),
            sdkmetric.WithReader(reader),
        )

        // 3. build known instruments
        if err := o.buildKnownInstruments(); err != nil {
            o.startErr = fmt.Errorf("k6output: instruments: %w", err); return
        }

        // 4. start flush goroutine
        ctx, cancel := context.WithCancel(context.Background())
        o.ctx = ctx
        o.cancelFn = cancel
        o.queue = make(chan metrics.SampleContainer, o.params.QueueSize)
        o.flushDone = make(chan struct{})
        go o.flushLoop()
    })
    return o.startErr
}

func (o *Output) Stop() error {
    o.stopOnce.Do(func() {
        if o.cancelFn != nil { o.cancelFn() }
        if o.flushDone != nil { <-o.flushDone }
        if o.pipeline != nil {
            ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            defer cancel()
            if err := o.pipeline.Shutdown(ctx); err != nil {
                o.logger("k6output: Shutdown: %v", err)
            }
        }
    })
    return nil  // always nil — k6 lifecycle protection
}
```

### 2.2 flush goroutine (Q5=A)

```go
func (o *Output) flushLoop() {
    defer close(o.flushDone)
    ticker := time.NewTicker(o.params.FlushInterval)  // default 1 sec
    defer ticker.Stop()

    var batch []metrics.SampleContainer
    const maxBatchSize = 1024

    flush := func() {
        if len(batch) == 0 { return }
        for _, container := range batch {
            o.emitContainer(container)
        }
        batch = batch[:0]
    }

    for {
        select {
        case <-o.ctx.Done():
            // drain remaining queue
            for {
                select {
                case s := <-o.queue:
                    batch = append(batch, s)
                default:
                    flush()
                    return
                }
            }
        case s := <-o.queue:
            batch = append(batch, s)
            if len(batch) >= maxBatchSize { flush() }
        case <-ticker.C:
            flush()
        }
    }
}
```

### 2.3 AddMetricSamples (Q6=A drop-oldest)

```go
func (o *Output) AddMetricSamples(samples []metrics.SampleContainer) {
    if o.queue == nil { return }  // Start not called yet
    select {
    case <-o.ctx.Done():
        return  // Stop called
    default:
    }
    for _, s := range samples {
        if !o.tryPush(s) {
            o.drops.Add(1)
        }
    }
}

func (o *Output) tryPush(s metrics.SampleContainer) bool {
    select {
    case o.queue <- s:
        return true
    default:
        // queue full — drop oldest, retry
        select {
        case <-o.queue:
            o.drops.Add(1)
        default:
        }
        select {
        case o.queue <- s:
            return true
        default:
            return false
        }
    }
}
```

### 2.4 Race-free invariants

- `Output` field は Start() 後は read-only (`startOnce.Do` で確定)
- queue channel は sync-safe
- flush goroutine と AddMetricSamples は channel 経由のみ通信
- ctx.Done() で flush goroutine が clean shutdown
- sync.Once により Start/Stop は idempotent

---

## 3. Error パターン

### 3.1 args parser error

```go
type ConfigError struct {
    Kind  string  // "invalid_args" | "invalid_protocol" | "type_mismatch" | "invalid_url"
    Field string
    Value string
    Inner error
}

func (e *ConfigError) Error() string {
    if e.Inner != nil {
        return fmt.Sprintf("k6output: %s (%s=%q): %v", e.Kind, e.Field, e.Value, e.Inner)
    }
    return fmt.Sprintf("k6output: %s (%s=%q)", e.Kind, e.Field, e.Value)
}

func (e *ConfigError) Unwrap() error { return e.Inner }
```

### 3.2 Start error

`Start()` 失敗時は wrapped error を return → k6 が run abort:

```go
return fmt.Errorf("k6output: pipeline init: %w", err)
return fmt.Errorf("k6output: build instrument %s: %w", name, err)
```

### 3.3 Stop の error 抑制 (NFR-U6-2)

Shutdown error は warn log のみ、戻り値は **常に nil**:

```go
if err := o.pipeline.Shutdown(ctx); err != nil {
    o.logger("k6output: Shutdown: %v", err)
}
return nil
```

---

## 4. API パターン

### 4.1 `--out args` parser (Q2=A)

```go
func parseOutArgs(s string) (Params, error) {
    p := defaultParams()
    if s == "" { return p, nil }
    tokens := strings.Split(s, ",")
    for _, t := range tokens {
        t = strings.TrimSpace(t)
        if t == "" { continue }
        kv := strings.SplitN(t, "=", 2)
        if len(kv) != 2 {
            return p, &ConfigError{Kind: "invalid_args", Field: t, Value: "(missing =)"}
        }
        key := strings.TrimSpace(kv[0])
        val := strings.TrimSpace(kv[1])
        if err := applyKV(&p, key, val); err != nil {
            return p, err
        }
    }
    return p, nil
}

func applyKV(p *Params, key, val string) error {
    switch key {
    case "endpoint":
        p.Endpoint = val
    case "protocol":
        switch val {
        case "grpc": p.Protocol = exporter.ProtocolGRPC
        case "http": p.Protocol = exporter.ProtocolHTTP
        default: return &ConfigError{Kind: "invalid_protocol", Field: key, Value: val}
        }
    case "insecure":
        b, err := strconv.ParseBool(val)
        if err != nil { return &ConfigError{Kind: "type_mismatch", Field: key, Value: val, Inner: err} }
        p.Insecure = b
    case "headers":
        m, err := parseHeaders(val)  // "key1:val1;key2:val2"
        if err != nil { return &ConfigError{Kind: "type_mismatch", Field: key, Value: val, Inner: err} }
        p.Headers = m
    case "compression":
        if val != "" && val != "gzip" {
            return &ConfigError{Kind: "invalid_args", Field: key, Value: val}
        }
        p.Compression = val
    case "timeout":
        d, err := time.ParseDuration(val)
        if err != nil { return &ConfigError{Kind: "type_mismatch", Field: key, Value: val, Inner: err} }
        p.Timeout = d
    case "batchSize", "maxQueueSize", "queueSize":
        n, err := strconv.Atoi(val)
        if err != nil { return &ConfigError{Kind: "type_mismatch", Field: key, Value: val, Inner: err} }
        switch key {
        case "batchSize": p.BatchSize = n
        case "maxQueueSize": p.MaxQueueSize = n
        case "queueSize":
            if n < 10 || n > 10000 {
                return &ConfigError{Kind: "invalid_args", Field: key, Value: val}
            }
            p.QueueSize = n
        }
    case "batchTimeout":
        d, err := time.ParseDuration(val)
        if err != nil { return &ConfigError{Kind: "type_mismatch", Field: key, Value: val, Inner: err} }
        p.BatchTimeout = d
    default:
        // unknown — warn + ignore (forward-compat)
        // (logger 未 init かも、Start 後 warn する仕組みは NFR Design で確定)
    }
    return nil
}

func parseHeaders(s string) (map[string]string, error) {
    if s == "" { return nil, nil }
    out := make(map[string]string)
    for _, pair := range strings.Split(s, ";") {
        pair = strings.TrimSpace(pair)
        if pair == "" { continue }
        kv := strings.SplitN(pair, ":", 2)
        if len(kv) != 2 {
            return nil, fmt.Errorf("invalid header pair %q (expected key:value)", pair)
        }
        out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
    }
    return out, nil
}

func defaultParams() Params {
    return Params{
        Protocol:     exporter.ProtocolGRPC,
        Endpoint:     "localhost:4317",
        Timeout:      10 * time.Second,
        BatchSize:    512,
        BatchTimeout: time.Second,
        MaxQueueSize: 2048,
        QueueSize:    100,  // U6 内 queue (channel buffered size)
    }
}
```

### 4.2 runner Resource builder (Q8=A)

```go
func buildRunnerResource(params output.Params) *resource.Resource {
    attrs := []attribute.KeyValue{
        semconv.ServiceName("xk6-otel-gen-runner"),
        semconv.TelemetrySDKName("opentelemetry"),
        semconv.TelemetrySDKLanguage("go"),
        semconv.TelemetrySDKVersion(getOTelSDKVersion()),
        semconv.ProcessRuntimeName("go"),
        semconv.ProcessRuntimeVersion(runtime.Version()),
    }
    if buildVersion != "" {  // ldflags injection
        attrs = append(attrs, semconv.ServiceVersion(buildVersion))
    }
    if params.ScriptPath != "" {
        attrs = append(attrs, attribute.String("k6.test.name", filepath.Base(params.ScriptPath)))
    }
    if host, err := os.Hostname(); err == nil {
        attrs = append(attrs, attribute.String("host.name", host))
    }
    return resource.NewSchemaless(attrs...)
}

var buildVersion = ""  // set via ldflags at build time
```

### 4.3 U4 patch: `Pipeline.MetricExporter()` (Q7=A)

`exporter/pipeline.go` に新規 method 追加 (U6 が依存):

```go
// MetricExporter returns the underlying OTLP metric exporter used by this
// Pipeline. Intended for k6output to construct an additional MeterProvider
// with a different Resource (e.g., xk6-otel-gen-runner) while sharing the
// same OTLP connection.
//
// The returned exporter is owned by the Pipeline; callers must NOT call
// Shutdown on it directly — use Pipeline.Shutdown for unified lifecycle.
func (p *Pipeline) MetricExporter() sdkmetric.Exporter {
    return p.metricExp  // internal field, currently used by p.mp
}
```

NFR-D-U4 改訂 + U4 NFR-D に追記 (Code Generation Plan で patch phase 化)。

---

## 5. Documentation パターン

### 5.1 `doc.go` の structure

```go
// Package k6output registers the "otel-gen" k6 output extension. Use as:
//
//   k6 run --out otel-gen=endpoint=https://...,protocol=grpc script.js
//
// It serves two purposes:
//
//   - (a) Pipeline shutdown trigger: Output.Stop() flushes the shared
//     exporter.Pipeline and invokes Shutdown.
//   - (b) k6 native metric → OTLP conversion: AddMetricSamples translates
//     k6 Trend/Counter/Gauge/Rate samples to OTel Histogram/Counter/
//     Gauge/Counter under the k6.* namespace, attached to a dedicated
//     Resource (service.name="xk6-otel-gen-runner") so they are distinct
//     from the simulated-service signals emitted by package synth.
//
// Supported --out args (key=value pairs separated by commas):
//
//   endpoint=<URL>
//   protocol=grpc|http
//   insecure=true|false
//   headers=key1:val1;key2:val2
//   compression=gzip
//   timeout=10s
//   batchSize=512
//   batchTimeout=1s
//   maxQueueSize=2048
//   queueSize=100   (range [10, 10000])
//
// Configuration priority (high to low):
//   JS API (otelgen.configure) > --out args > OTEL_EXPORTER_OTLP_* env vars > defaults
package k6output
```

### 5.2 Example function (NFR-R Q10=A)

1 件のみ:

```go
func ExampleNew() {
    // k6 SDK calls this internally when --out otel-gen=... is given;
    // user code does not invoke New directly.
    params := output.Params{ConfigArgument: "endpoint=localhost:4317,protocol=grpc"}
    out, err := k6output.New(params)
    if err != nil { log.Fatal(err) }
    _ = out
    // Output:
}
```

### 5.3 GoDoc 網羅性

全 exported (`New`, `Description`, `Start`, `Stop`, `AddMetricSamples`, `Params`, `ConfigError`) に doc comment、`revive` enforce。

---

## 6. Test パターン

### 6.1 Helper (Q12=A)

`k6output/helpers_test.go`:

```go
// newTestParams builds an output.Params with sensible defaults for tests.
func newTestParams(t *testing.T, args string) output.Params {
    t.Helper()
    return output.Params{
        ConfigArgument: args,
        ScriptPath:     "test.js",
    }
}

// newTestOutput constructs an Output with mock Pipeline.
func newTestOutput(t *testing.T, args string) (*Output, *sdkmetric.ManualReader) {
    t.Helper()
    params := newTestParams(t, args)
    out, err := New(params)
    require.NoError(t, err)
    o := out.(*Output)
    // Replace pipeline with mock — but exporter.GetShared is hard to mock per-test.
    // Use a sub-process / package-level reset helper exposed by exporter (or
    // build a real Pipeline with insecure dummy endpoint).
    // Strategy: use exporter.ResetShared() before each test, then Start()
    // builds a real Pipeline against a dummy endpoint that doesn't actually
    // connect (insecure=true, batchTimeout=long enough that nothing flushes).
    return o, /* reader from somewhere */
}

// newMockPipeline constructs a stand-in Pipeline backed by a ManualReader.
// (Requires exporter package to support test injection, see Q12=A.)
func newMockPipeline(t *testing.T) (*exporter.Pipeline, *sdkmetric.ManualReader) {
    t.Helper()
    // ... use exporter's test-only constructor ...
}
```

> **NOTE**: U4 (`exporter` package) は GetShared をテスト用に reset する `ResetShared` を NFR-D で export 済。ただし MetricExporter() で内部 exporter を取得しても **test では mock を inject できない** (Pipeline は real OTLP exporter を build する)。
>
> Test 戦略 (NFR Design で確定):
> - **Option α**: integration test に依存、unit test では `New`/`Description`/`AddMetricSamples` まで test、`Start`/`Stop` の Pipeline 連動部分は integration で
> - **Option β**: U4 に test-only helper `exporter.NewWithMockMetricExporter(exp sdkmetric.Exporter)` 等を追加 — U4 NFR-D 改訂が必要
> - **採用 Option α**: U4 NFR-D 改訂は最小限、unit test scope を絞る

### 6.2 unit test 構造

| ファイル | 対象 | 形式 |
|---|---|---|
| `output_test.go` | New / Description / lifecycle (limited Start/Stop) | example-based |
| `params_test.go` | parseOutArgs | table-driven |
| `convert_test.go` | k6 metric → OTel mapping + tag conversion | table-driven |
| `pbt_test.go` | TP-U6-1..3 | rapid |
| `doc_test.go` | 1 Example function | output check |
| `bench_test.go` | per-sample bench | benchmark |
| `integration/integration_test.go` | E2E with xk6 + Docker | `//go:build integration` |

### 6.3 PBT (Q11=A — example-based + rapid for invariants only)

```go
// TP-U6-1: Robustness — example-based table covering state transitions
func TestOutput_Robustness_AllStates(t *testing.T) {
    t.Parallel()
    cases := []struct {
        name string
        ops  []func(*Output) error  // sequence of {Start, AddSamples, Stop} variants
    }{
        {"Start_AddSamples_Stop", []func(*Output) error{...}},
        {"AddSamples_BeforeStart", ...},
        {"Stop_BeforeStart", ...},
        {"Stop_AfterStop_NoOp", ...},
        {"AddSamples_AfterStop", ...},
    }
    for _, c := range cases {
        c := c
        t.Run(c.name, func(t *testing.T) {
            t.Parallel()
            out := makeTestOutput(t)
            for _, op := range c.ops {
                require.NotPanics(t, func() { _ = op(out) })
            }
        })
    }
}

// TP-U6-2: Counter Monotonicity (rapid)
func TestCounter_Monotonic_Property(t *testing.T) {
    t.Parallel()
    rapid.Check(t, func(t *rapid.T) {
        samples := generators.ValidK6Sample().
            Filter(func(s metrics.Sample) bool {
                return s.Metric.Type == metrics.Counter && s.Value >= 0
            }).
            DrawN(t, "samples", 10)
        // ... emit, ManualReader.Collect, assert sum
    })
}

// TP-U6-3: Tag round-trip (rapid)
func TestTag_Attribute_RoundTrip_Property(t *testing.T) {
    t.Parallel()
    rapid.Check(t, func(t *rapid.T) {
        // generate sample with random Tags, emit, assert k6.tag.* attrs present
    })
}
```

### 6.4 Integration test (Q11=A NFR-R)

`k6output/integration/integration_test.go`:
- `//go:build integration`
- xk6 build (U5 と共有 logic、コピー)
- 実 `k6 run --out otel-gen=endpoint=<collector>` を exec
- Collector の file_exporter 出力を読み、`k6.*` namespace の metric が含まれることを assert

---

## 7. NFR-R Open Questions の解消

| Open Question | 確定 |
|---|---|
| queue size default | §1.1 / §4.1: default 100、`--out queueSize=N` で調整 |
| flush ticker 間隔 | default 1 sec (fix)、将来 configurable |
| OTel exemplar 機能 | future revisit |
| k6 group/scenario hierarchy in Resource | future revisit |
| queueSize validate 範囲 | §4.1 で [10, 10000] 固定 |

---

## 8. NFR-R 各項目との対応

| NFR-R | 対応 Design パターン |
|---|---|
| NFR-U6-1 (API stability) | §4.1 args contract、`Params` struct 安定 |
| NFR-U6-2 (Lifecycle) | §2.1 sync.Once、Stop always nil |
| NFR-U6-3 (Per-sample budget) | §1.4 tagSetCache、§1.2 instrument map sync.Map、hot path zero-alloc |
| NFR-U6-4 (Soft lifecycle latency) | §2.1 で objective 監視のみ |
| NFR-U6-5 (Formula memory) | §1.1 struct + sync.Map による pay-as-you-grow |
| NFR-U6-6 (Configurable queue) | §4.1 queueSize key + range check |
| NFR-U6-7 (Concurrency) | §2 全般 |
| NFR-U6-8 (No self-metric) | atomic.Uint64 drops は internal、accessor なし |
| NFR-U6-9 (Documentation) | §5 doc.go + Example |
| NFR-U6-10 (Testability) | §6 全般 |
| NFR-U6-11 (Compatibility) | direct OTel SDK + k6 SDK pin |
| NFR-U6-12 (PBT) | §6.3 example + rapid hybrid |
| NFR-U6-13 (Cardinality) | §1.4 tagSetCache に upper bound なし (user/Collector 責務) |

---

## 9. Anti-patterns (採用しない)

| アンチパターン | 不採用理由 |
|---|---|
| `*Output` flat layout (Q1 案 B) | group 化で読みやすい |
| nested struct (Q1 案 C) | over-engineering |
| URL query-style parser (Q2 案 B) | k6 慣例から逸脱 |
| regexp parser (Q2 案 C) | overhead |
| 完全 eager instrument (Q3 案 B) | k6 SDK enumeration API なし、forward-compat 不能 |
| 完全 lazy (Q3 案 C) | hot path cost |
| `map[string]any` + RWMutex (Q4 案 B) | type assertion overhead |
| typed map per type + RWMutex (Q4 案 C) | sync.Map で十分 |
| time.Timer + atomic.Bool (Q5 案 B) | error-prone |
| close(queue) で自然終了 (Q5 案 C) | post-close push panic |
| 単純 drop new (Q6 案 B) | latest 重視の k6 metric semantic 違反 |
| Ring buffer (Q6 案 C) | 過剰 |
| `MetricReader()` accessor (Q7 案 B) | Resource attach 困難 |
| `BuildMeterProvider(res)` accessor (Q7 案 C) | Pipeline が複数 MP lifecycle を管理 |
| `exporter/resource.go` 共有 helper (Q8 案 B) | synth と shared 点なし |
| 別 testutil package (Q8 案 C) | 過剰 |
| 毎回 tag set build (Q9 案 B) | allocation 多 |
| 単一 attribute.Set (Q9 案 C) | tag バリエーション失う |
| FNV/xxHash uint64 hash (Q10 案 B) | collision risk |
| fmt.Sprintf hash (Q10 案 C) | allocation 多 |
| stateful PBT rapid.Run (Q11 案 B) | 初期実装複雑 |
| k6 SDK testutils (Q12 案 B) | available か不明、ad-hoc にする |
| mock interface based (Q12 案 C) | 過剰 |
| `convert.go` 2 分割 (Q13 案 B) | 共通 helper の重複 |
| `errors.go` を `output.go` 内に inline (Q13 案 C) | 種別の独立性失う |

---

## 10. U4 coordination 要件

§4.3 で確定: `exporter.Pipeline.MetricExporter() sdkmetric.Exporter` を追加。

- minor SemVer bump
- internal field `metricExp` の getter
- Pipeline.Shutdown が exporter lifecycle を所有、caller は Shutdown しない

Code Generation Plan で **「Phase: Add Pipeline.MetricExporter to exporter」** を独立 phase 化 (U2 NewEngineWithSeed と同 pattern)。
