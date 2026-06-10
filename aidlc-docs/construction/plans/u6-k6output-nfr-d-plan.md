# U6 (k6output) — NFR Design Plan

## ユニットコンテキスト

- **Unit ID**: U6
- **パッケージ**: `k6output/`
- **FD**: `aidlc-docs/construction/u6-k6output/functional-design/` (committed 507c719)
- **NFR-R**: `aidlc-docs/construction/u6-k6output/nfr-requirements/` (committed 71b7943)
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → U2 ✓ → U5 ✓ → **U6 (this — NFR-D)** → U8

## NFR Design の焦点

FD で「何をする」、NFR-R で「何を達成するか」を確定済。NFR Design は **「どう実装するか」のパターン** を確定する:

- **Output struct 物理 layout** — queue / flush goroutine 関連 field
- **`--out args` parser の実装方式**
- **instrument 構築戦略** — eager (known metrics) vs lazy (dynamic discovery)
- **flush goroutine の cancel / done channel パターン**
- **queue full handling の具体 algorithm** — drop oldest の実装
- **U4 `Pipeline.MetricExporter()` patch の signature 詳細**
- **runner Resource builder の実装**
- **k6 Sample → OTel attribute conversion の hot path 最適化**
- **k6 metric type → instrument type の dispatch (switch / map)**
- **Test helper の構造** — k6 SDK mock approach
- **PBT 実装パターン**
- **Anti-patterns**

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u6-k6output/nfr-design/nfr-design-patterns.md`
- [ ] `aidlc-docs/construction/u6-k6output/nfr-design/logical-components.md`

---

## 設計確定のための質問

### Question 1: Output struct layout

`*Output` の field 物理配置:

A) **3 group: config + lifecycle + emission** (推奨):
```go
type Output struct {
    // Config (immutable after New)
    params  Params
    runnerResource *resource.Resource

    // Lifecycle (sync.Once guarded)
    startOnce sync.Once
    startErr  error
    stopOnce  sync.Once

    // Emission (set in Start, used by flush)
    pipeline   *exporter.Pipeline
    meterProvider *sdkmetric.MeterProvider
    instruments instrumentMap  // metric name → instrument
    setCache    sync.Map        // attribute set cache

    // Queue (set in Start)
    queue     chan metrics.SampleContainer
    cancelFn  context.CancelFunc
    flushDone chan struct{}
    drops     atomic.Uint64
}
```

B) **flat layout** — sync.Once / channel を直接並べる

C) **nested struct** — `Output{lifecycle lifecycleImpl; emission emissionImpl}` 等

X) Other

[Answer]: A

---

### Question 2: `--out args` parser 実装

`parseOutArgs(s string) (Params, error)` の実装:

A) **手書き parser**: `strings.Split(s, ",")` → 各 token を `strings.SplitN(t, "=", 2)` → switch で key 別 decode (推奨、シンプル)

B) **URL query-style**: `url.ParseQuery(s)` で key/value map 化 (`,` を `&` に置換)、その後 type assert
   - 利点: `url.QueryUnescape` で URL encode 対応
   - 欠点: k6 慣例から離れる

C) **regexp ベース**: `regexp.MustCompile` で token を抽出

X) Other

[Answer]: A

---

### Question 3: instrument 構築戦略

`Start()` で全 known k6 metric の instrument を eager build するか、dynamic に build するか:

A) **eager (known metric) + lazy (unknown metric)** (推奨):
   - `Start()` で `http_req_duration`, `iterations`, `vus`, `data_sent` 等の standard metric を pre-create
   - `AddMetricSamples` で初出 metric を見たら lazy build (sync.Map + Get-or-Create)
   - 利点: hot path 上の standard metric は lookup-only、unknown metric も動作する

B) **完全 eager**: Start() 時に k6 SDK の Metric registry を query して全 metric を pre-create
   - k6 SDK が enumeration API を提供していれば最適
   - 提供していなければ A に fall back

C) **完全 lazy**: 全 metric を dynamic build
   - hot path に metric build cost が乗る (初回のみ)

X) Other

[Answer]: A

---

### Question 4: instrument map の物理表現

instrument cache の concurrency-safe map:

A) **`sync.Map` keyed by k6 metric name** (推奨):
```go
type instrumentMap struct {
    counters   sync.Map  // k6 name → Float64Counter
    histograms sync.Map  // k6 name → Float64Histogram
    gauges     sync.Map  // k6 name → Float64Gauge
    // (k6 Rate は Counter 系として扱うので counters に統合)
}
```

B) **`map[string]any` + sync.RWMutex** — explicit lock

C) **typed map per metric type** — `map[string]metric.Float64Counter` 等の純粋 typed map (sync.RWMutex 必要)

X) Other

[Answer]: A

---

### Question 5: flush goroutine の cancel pattern

`Stop()` で flush goroutine を確実に停止させる pattern:

A) **`context.WithCancel` + `done chan struct{}`** (推奨、明示的):
```go
func (o *Output) flushLoop() {
    defer close(o.flushDone)
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()
    var batch []metrics.SampleContainer
    for {
        select {
        case <-o.ctx.Done():
            // drain remaining queue then return
            o.drainAndEmit(batch)
            return
        case s := <-o.queue:
            batch = append(batch, s)
            if len(batch) >= maxBatchSize { o.emitBatch(batch); batch = nil }
        case <-ticker.C:
            if len(batch) > 0 { o.emitBatch(batch); batch = nil }
        }
    }
}

func (o *Output) Stop() error {
    o.stopOnce.Do(func() {
        o.cancelFn()
        <-o.flushDone  // wait for flush
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        if err := o.pipeline.Shutdown(ctx); err != nil {
            log.Printf("k6output: Shutdown: %v", err)
        }
    })
    return nil
}
```

B) **time.Timer + atomic.Bool** — simpler でも error-prone

C) **close(queue) で goroutine が自然終了** — ただし queue close 後の AddMetricSamples が panic

X) Other

[Answer]: A

---

### Question 6: queue full handling の具体 algo

queue 100 entries full の時の **drop oldest** 実装:

A) **non-blocking try-send、失敗時 drain one + retry** (推奨):
```go
func (o *Output) AddMetricSamples(samples []metrics.SampleContainer) {
    for _, s := range samples {
        select {
        case o.queue <- s:
            // OK
        default:
            // queue full, drop oldest and try again
            select {
            case <-o.queue:
                o.drops.Add(1)
            default:
            }
            select {
            case o.queue <- s:
            default:
                // still full? drop new
                o.drops.Add(1)
            }
        }
    }
}
```

B) **常に `select default` で drop new** — シンプルだが latest を捨てる (古いが直近の傾向は drop)

C) **ring buffer + atomic head/tail** — 高速だが実装複雑

X) Other

[Answer]: A

---

### Question 7: U4 `Pipeline.MetricExporter()` の signature

U4 patch で追加する accessor の signature:

A) **`func (p *Pipeline) MetricExporter() sdkmetric.Exporter`** (推奨、シンプル):
   - U6 が `sdkmetric.NewPeriodicReader(exp)` で wrap して MeterProvider に注入

B) **`func (p *Pipeline) MetricReader() sdkmetric.Reader`** — Reader を直接 expose
   - U6 が Reader を MeterProvider に attach
   - U4 Pipeline 内の Reader が runner Resource を受け付けるか? — 多分受け付けない (Reader は Provider に attach、Resource は Provider 単位)
   - 案 A の方がクリーン

C) **`func (p *Pipeline) BuildMeterProvider(res *resource.Resource) (*sdkmetric.MeterProvider, error)`** — Resource を渡して MeterProvider を Pipeline 側で構築
   - U6 から見ると interface clean、ただし Pipeline が複数 MeterProvider lifecycle を管理する責務発生

X) Other

[Answer]: A

---

### Question 8: runner Resource builder の場所

`buildRunnerResource()` の配置:

A) **`k6output/output.go` 内 private helper** (推奨、output 専用):
```go
func buildRunnerResource(params output.Params) *resource.Resource {
    return resource.NewSchemaless(
        semconv.ServiceName("xk6-otel-gen-runner"),
        semconv.TelemetrySDKName("opentelemetry"),
        semconv.TelemetrySDKLanguage("go"),
        // ...
    )
}
```

B) **`exporter/resource.go` に共有 helper** として配置 — synth とも shared
   - 不要、synth は per-service Resource、共有点なし

C) **`testutil/resourcehelpers/` 等の共有 package** — 過剰

X) Other

[Answer]: A

---

### Question 9: k6 tag → attribute conversion の最適化

`k6 sample.Tags map[string]string` を `attribute.Set` に変換する hot path:

A) **per-call build + sync.Map cache by tag-set hash** (推奨、U3 と同パターン):
```go
type tagSetCache struct {
    sets sync.Map  // string (hash of sorted tags) → attribute.Set
}

func (c *tagSetCache) get(tags map[string]string) attribute.Set {
    h := hashTags(tags)
    if v, ok := c.sets.Load(h); ok { return v.(attribute.Set) }
    kvs := make([]attribute.KeyValue, 0, len(tags))
    for k, v := range tags {
        kvs = append(kvs, attribute.String("k6.tag."+k, v))
    }
    set := attribute.NewSet(kvs...)
    c.sets.Store(h, set)
    return set
}
```

B) **毎回 build (cache なし)** — シンプルだが allocation 多

C) **per-Output single attribute.Set (immutable 1 個)** — tag 違いを表現できない

X) Other

[Answer]: A

---

### Question 10: tag hash function

`hashTags(map[string]string) string` の実装:

A) **sorted keys を joined string で生成** (推奨、deterministic + simple):
```go
func hashTags(tags map[string]string) string {
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
```

B) **FNV / xxHash の uint64** — 高速だが collision risk

C) **`fmt.Sprintf` で key=value;... 形式** — A と等価だが alloc 多

X) Other

[Answer]: A

---

### Question 11: PBT 実装パターン

TP-U6-1 (Robustness) の `rapid.Run` based stateful PBT 採用?

A) **採用しない、example-based の plain test** (推奨、初期実装の複雑度回避)
   - 状態遷移 (uninitialized / started / stopped) は table-driven test で網羅

B) **採用、`rapid.Run` で random sequence**
   - {Start, AddMetricSamples, Stop} の random sequence を生成
   - panic なし、final state が consistent
   - test 実装コスト中

X) Other

[Answer]: A

---

### Question 12: Test helper の構造

`k6output/helpers_test.go` の utility:

A) **`output.Params` builder + ManualReader-backed Pipeline mock** (推奨):
```go
func newTestOutputParams(t *testing.T) output.Params { ... }
func newTestOutput(t *testing.T, args string) *Output { ... }
func newMockPipeline(t *testing.T) (*exporter.Pipeline, *sdkmetric.ManualReader) { ... }
```

B) **k6 SDK の test fixture を使う** (`go.k6.io/k6/lib/testutils` が提供している場合)

C) **mock interface based** — `output.Params` interface 化

X) Other

[Answer]: A

---

### Question 13: ファイル分割の最終確認

FD §3 提案の 5 production + 4 test:

A) **そのまま採用** (推奨):
   - `doc.go`, `output.go`, `params.go`, `convert.go`, `errors.go`
   - tests: `output_test.go`, `params_test.go`, `convert_test.go`, `pbt_test.go`
   - + `helpers_test.go`, `doc_test.go`, `bench_test.go`
   - + `integration/integration_test.go`, `integration/helpers.go`, `integration/testdata/{script.js,topology.yaml,collector-config.yaml,docker-compose.yaml}`

B) **`convert.go` を分割**: `convert_metric.go` (k6 metric → OTel instrument) + `convert_attr.go` (k6 tag → OTel attribute)

C) **`errors.go` 省略** — ConfigError を `output.go` 内 inline

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR Design アーティファクトを生成して承認ゲートへ進みます。
