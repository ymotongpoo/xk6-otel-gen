# U6 k6output — Business Logic Model

本書は `k6output/` パッケージのビジネスロジック (k6 Output module の動作) を確定する。

参照: Application Design (`component-methods.md` §C6)、`plans/u6-k6output-fd-plan.md` の Q1..Q13 回答。

---

## 1. パッケージの責務

`k6output/` は **k6 Output SDK 経由で `--out otel-gen=...` を登録** する unit。デュアル機能:
- **(a) Pipeline shutdown trigger** — U5 (or U6 自身) が構築した `exporter.Pipeline` を `Output.Stop()` で flush + shutdown
- **(b) k6 native metrics の OTLP 変換** — k6 runner が観測する実測値 (http_req_*, vus, iterations 等) を OTel Metrics として `k6.*` namespace で OTLP に送信

### 1.1 「k6 runner の実測値」と「合成テレメトリー」の区別

| 種別 | 出所 | Resource | namespace |
|---|---|---|---|
| **k6 runner 実測値** (U6 担当) | k6 が load test 実行中に観測する http_req_duration 等 | `service.name="xk6-otel-gen-runner"` | `k6.*` |
| **合成テレメトリー** (U3 担当) | topology に定義された simulated service の擬似 span/metric/log | `service.name=<svc.Name>` (e.g., "checkout") | semconv standard (`http.*`, `rpc.*`, `db.*`, `messaging.*`) |

両者は **同一 Pipeline (OTLP endpoint) を共有** するが、Resource attribute で完全に分離される。OTel Collector や backend dashboard で別の service として認識される。

### 1.2 関連 unit との関係

```text
              ┌────────────────────────────────┐
              │ k6 CLI / k6 binary             │
              │   k6 run --out otel-gen=...    │
              │     script.js                  │
              └────┬────────────────────┬──────┘
                   │                    │
                   │ JS module          │ Output module
                   ▼                    ▼
              ┌─────────────┐    ┌─────────────────────┐
              │ U5          │    │ U6 (this unit)      │
              │ k6otelgen   │    │ k6output            │
              │ - load      │    │ - Output (k6 SDK)   │
              │ - configure │    │ - Start/Stop        │
              │ - runJourney│    │ - AddMetricSamples  │
              └──────┬──────┘    └─────────┬───────────┘
                     │                     │
                     └─────────┬───────────┘
                               │ exporter.GetShared() — 同一 Pipeline 共有
                               ▼
                         ┌──────────────┐
                         │ U4 exporter  │
                         │ Pipeline     │
                         │ - Tracer     │
                         │ - Meter      │
                         │ - Logger     │
                         └──────┬───────┘
                                │ OTLP gRPC/HTTP
                                ▼
                         ┌──────────────┐
                         │ OTel         │
                         │ Collector    │
                         └──────────────┘
```

---

## 2. k6 Output SDK lifecycle と U6 動作

### 2.1 k6 phase ごとの動作

```text
k6 process start
    │
    │ init code が走る (--out flag 解析含む)
    ▼
[U6] init() in output.go が k6 SDK に register
    │ output.RegisterExtension("otel-gen", New)
    │
    ▼
[k6] --out otel-gen=<args> が指定されている場合
    │ k6 SDK が U6 の New(params) を呼ぶ
    │ ← *Output instance 構築 (heavy init はしない)
    │
    ▼
[k6] setup() を 1 回実行 (single VU)
    │ U5 が load() / configure() を呼ぶ
    │ → exporter.GetShared(factory) で Pipeline 構築 (U5 経由)
    │
    ▼
[k6] Output.Start() を呼ぶ
    │ U6 が exporter.GetShared(factory) を呼ぶ
    │   - U5 が先に呼んでいれば cache hit (U5 の config が使われる)
    │   - U5 が未使用なら U6 の --out args + env が使われる
    │ U6 が k6 metric → OTel meter の instrument を eager build
    │ U6 が flush goroutine を起動 (1 sec or queue 閾値ベース)
    │ ← err != nil なら k6 run abort (fail-fast)
    │
    ▼
[k6] per-VU iteration 開始
    │ k6 SDK が Sample (http_req_*, vus, iterations 等) を内部 channel で
    │ U6 に渡す。U6.AddMetricSamples(samples) が呼ばれる
    │ U6 は queue に push (non-blocking)
    │
    │ また U5 が runJourney() を呼ぶたびに synth が OTel signal を emit
    │ → 同じ Pipeline で OTLP 送信
    │
    ▼
[U6 flush goroutine] 定期的に queue を drain、OTel meter に Record
    │
    ▼
[k6] iteration / scenario 完了
    │
    ▼
[k6] Output.Stop() を呼ぶ
    │ U6:
    │   (1) flush goroutine 停止 + 残 queue を drain
    │   (2) Pipeline.Shutdown(ctx) を呼ぶ (timeout 30s)
    │       - synth が emit した signals も flush される
    │       - U6 自身の metrics も flush される
    │   (3) Shutdown error は warn log、Stop() は nil 返却 (k6 を crash させない)
    │
    ▼
k6 process exit
```

### 2.2 init() の登録

`k6output/output.go` 内:
```go
func init() {
    output.RegisterExtension("otel-gen", New)
}
```

`New(params output.Params) (output.Output, error)` を新規 instance 作る関数として登録。

---

## 3. `--out otel-gen=<args>` の解析 (Q1=A)

### 3.1 syntax

```bash
k6 run --out otel-gen=endpoint=https://otel.example.com:4317,protocol=grpc,insecure=false script.js
```

- `=` で `otel-gen` と args 部分を分離 (k6 SDK が自動)
- args は `,` で split → 各 token を `=` で key/value に分解
- 順序は free、unknown key は warn + ignore (forward compat)

### 3.2 supported keys

| key | value type | example |
|---|---|---|
| `endpoint` | string | `https://otel.example.com:4317` |
| `protocol` | `"grpc"` or `"http"` | `grpc` |
| `insecure` | `"true"` or `"false"` | `false` |
| `headers` | `key1:val1;key2:val2` | `api-key:abc;x-tenant:foo` |
| `compression` | `"gzip"` or `""` | `gzip` |
| `timeout` | duration string (e.g., `10s`) | `5s` |
| `batchSize` | int | `512` |
| `batchTimeout` | duration | `1s` |
| `maxQueueSize` | int | `2048` |

### 3.3 args 解析の error

- 不正 syntax (`endpoint` 値の URL parse 失敗等) → `*ConfigError{Kind: "invalid_args", Field, Value}` を return から `Output.New` に渡し、k6 run abort
- 未知 key → silently ignore + log

---

## 4. Config 優先順位 (Q2=A)

### 4.1 merge 順 (高 → 低)

```
JS API (otelgen.configure())  >  --out args  >  env (OTEL_EXPORTER_OTLP_*)  >  built-in defaults
```

### 4.2 動作

```text
U6.New(params):
    outArgsConfig := parseOutArgs(params.ConfigArgument)  // --out args → Config
    U6.outConfig = outArgsConfig
    return *Output

U6.Start():
    pipeline, err := exporter.GetShared(func() (*Pipeline, error) {
        // factory called only once across U5 + U6
        builtIn := exporter.Config{}
        envCfg := exporter.ConfigFromEnv()
        merged := builtIn.MergeWith(envCfg).MergeWith(U6.outConfig)
        // U5 が JS configure() を先に呼んでいた場合は GetShared cache hit、
        // この factory は呼ばれない (U5 の Config が使われる)
        return exporter.New(merged)
    })
```

### 4.3 U5 と U6 のどちらが先に GetShared を呼ぶか?

- 通常: setup() → JS configure() / load() → GetShared (U5 経由)
- Edge case: setup なしの k6 run → U5 は呼ばれず、`Output.Start()` で初構築 (U6 factory が動く)
- これは `exporter.GetShared` の sync.Once で **どちらが先でも race-free**

---

## 5. Pipeline 取得 (Q3=A)

§4.2 通り `Output.Start()` 内で `exporter.GetShared(factory)`。

### 5.1 失敗時

`exporter.GetShared` が error を返す場合 (factory 内 `exporter.New` failure):
- `Output.Start()` が error を return → k6 run abort (Q8=A)
- User は `--out` args / env を確認して再実行

---

## 6. k6 native metrics → OTel metric mapping (Q4=A)

### 6.1 mapping table

| k6 metric type | OTel instrument | OTel name template | unit |
|---|---|---|---|
| `Trend` | `Float64Histogram` | `k6.<metric_name>` | metric.WithUnit による (e.g., `ms` / `s` / `bytes`) |
| `Counter` | `Float64Counter` | `k6.<metric_name>` | `{count}` or specific |
| `Gauge` | `Int64ObservableGauge` (or `Float64Gauge`) | `k6.<metric_name>` | specific |
| `Rate` | `Float64Counter` (failed のみ +1) | `k6.<metric_name>.total` | `{count}` |

### 6.2 代表的な k6 metric の mapping

| k6 metric name | OTel name | OTel type |
|---|---|---|
| `http_req_duration` | `k6.http.request.duration` | Histogram (ms) |
| `http_req_failed` | `k6.http.request.failed.total` | Counter |
| `http_reqs` | `k6.http.requests.total` | Counter |
| `iterations` | `k6.iterations.total` | Counter |
| `iteration_duration` | `k6.iteration.duration` | Histogram (ms) |
| `vus` | `k6.vus` | UpDownCounter or Gauge |
| `vus_max` | `k6.vus.max` | UpDownCounter or Gauge |
| `data_sent` | `k6.data.sent.total` | Counter (bytes) |
| `data_received` | `k6.data.received.total` | Counter (bytes) |
| `checks` | `k6.checks.total` | Counter |
| `group_duration` | `k6.group.duration` | Histogram (ms) |

> **NOTE**: k6 SDK の `metrics.Metric` は `Type` field を持つ (`metrics.Counter`, `metrics.Trend`, `metrics.Gauge`, `metrics.Rate`)。U6 は metric type を見て上記 mapping を適用。

### 6.3 namespace `k6.*` の意義

- semconv 標準 (`http.*` 等) と区別 — synth が emit するシグナルが `http.request.method` で k6 runner が `k6.http.request.duration` を出す、両立可能
- backend ダッシュボードで「k6 runner 由来」が即座に識別できる

---

## 7. Resource attributes for k6 metrics (Q5=A)

### 7.1 専用 Resource

```text
{
    service.name             = "xk6-otel-gen-runner"
    service.version          = "<xk6-otel-gen Build version>"  (e.g., "v0.1.0")
    telemetry.sdk.name       = "opentelemetry"
    telemetry.sdk.language   = "go"
    telemetry.sdk.version    = <OTel SDK version>
    process.runtime.name     = "go"
    process.runtime.version  = <runtime.Version()>
    k6.test.name             = <script filename, if k6 SDK exposes>
    k6.test.id               = <test run ID, if k6 SDK exposes>
}
```

### 7.2 synth Resource との分離

`exporter.Pipeline` は **複数 service の Resource** を扱えない (OTel SDK の TracerProvider/MeterProvider は 1 Resource per provider)。

→ **U6 が独自に MeterProvider を構築**:
```go
runnerResource := buildRunnerResource()
mp := sdkmetric.NewMeterProvider(
    sdkmetric.WithResource(runnerResource),
    sdkmetric.WithReader(sdkmetric.NewPeriodicReader(<shared exporter>)),
)
```

問題: `exporter.Pipeline` は内部で MeterProvider 1 個しか持たない (synth 用、`service.name=<svc>`)。`AddMetricSamples` を別 Resource で送信するには **別 MeterProvider** が必要。

#### 7.2.1 設計選択

**Option α**: U6 が **独自の MeterProvider** を構築 (Pipeline の exporter (OTLP Exporter) を共有)
   - `exporter.Pipeline` に `MetricExporter()` accessor を追加してもらう (U4 minor bump 要)
   - 利点: 明確な separation、Resource が clean
   - 欠点: U4 patch 必要

**Option β**: synth Resource を resource detector で動的切り替え
   - 実装難、Resource は基本 immutable

**Option γ**: `service.name` を attribute として metric 側に手動付与 (Resource ではなく Sample attribute)
   - 利点: Pipeline そのまま使える
   - 欠点: OTel semantic 違反 (`service.name` は Resource-level)

→ **推奨 Option α**: U4 に `Pipeline.MetricExporter() sdkmetric.Exporter` (or 同等の internal exporter accessor) を追加。U6 が独自 MeterProvider を構築して runnerResource を attach。これは U4 minor SemVer bump (新規 method 追加、backward-compatible)。

> **NOTE**: NFR Design で U4 patch を独立 phase 化する必要あり (U2 で `NewEngineWithSeed` を patch した pattern と同じ)。

---

## 8. AddMetricSamples の batching (Q6=A)

### 8.1 queue + flush goroutine

```text
Output 構造体:
    queue chan metrics.SampleContainer  // buffered channel
    flushDone chan struct{}             // shutdown signal

Output.Start():
    ctx, cancel := context.WithCancel(context.Background())
    go flushLoop(ctx)
    Output.cancelFunc = cancel
    Output.ctx = ctx

flushLoop(ctx):
    ticker := time.NewTicker(1 * time.Second)
    var batch []metrics.SampleContainer
    for {
        select {
        case s := <-Output.queue:
            batch = append(batch, s)
            if len(batch) >= 1024 {
                emitBatch(batch); batch = nil
            }
        case <-ticker.C:
            if len(batch) > 0 { emitBatch(batch); batch = nil }
        case <-ctx.Done():
            // drain remaining
            for {
                select {
                case s := <-Output.queue:
                    batch = append(batch, s)
                default:
                    if len(batch) > 0 { emitBatch(batch) }
                    Output.flushDone <- struct{}{}
                    return
                }
            }
        }
    }

Output.AddMetricSamples(samples):
    select {
    case Output.queue <- samples:
    default:
        // queue full — drop oldest? log warn?
        // option: synchronous wait with timeout
    }

Output.Stop():
    Output.cancelFunc()  // signal flushLoop to drain + exit
    <-Output.flushDone   // wait
    Pipeline.Shutdown(ctx with 30s timeout)
```

### 8.2 queue size

- buffered channel capacity: 100 (small, drop on full は最後の手段)
- typical k6 high throughput では 1 sec の flush interval で間に合う想定
- NFR Design で bench 後に調整

### 8.3 backpressure

- queue full 時の挙動: warn log + drop oldest sample (NFR Design で確定)
- 代替: blocking push (k6 が遅くなるが loss なし)

---

## 9. Stop() lifecycle (Q7=A)

```text
Output.Stop():
    1. flushLoop に shutdown signal
    2. flushLoop が remaining queue を drain + return
    3. Pipeline.Shutdown(ctx) を呼ぶ
       - ctx は内部で 30s timeout
       - synth が emit した OTLP signals も flush される (Pipeline 共有のため)
       - U6 自身の MetricProvider も同時に shutdown
    4. Shutdown error は warn log
    5. return nil (always — k6 lifecycle を crashed させない)
```

---

## 10. Start() error 処理 (Q8=A)

```text
Output.Start():
    pipeline, err := exporter.GetShared(...)
    if err != nil { return fmt.Errorf("k6output: pipeline init: %w", err) }
    
    runnerMP := buildRunnerMeterProvider(pipeline)
    Output.runnerMP = runnerMP
    Output.instruments = buildAllInstruments(runnerMP)  // k6 metric → instrument map
    
    if err := Output.startFlushLoop(); err != nil {
        return fmt.Errorf("k6output: start flush: %w", err)
    }
    return nil
```

各 step で error 発生 → k6 run abort (fail-fast)。

---

## 11. Rate metric 表現 (Q9=A)

k6 `http_req_failed` は Rate metric:
- 成功時 sample.Value=0
- 失敗時 sample.Value=1

OTel 表現:
- **sample.Value == 1 → Counter `k6.http.request.failed.total` を +1**
- **sample.Value == 0 → emit しない** (rate は backend で `failed.total / iterations.total` で算出)

namespace: `k6.<metric_name>.total` で Counter 系であることを suffix で示す。

---

## 12. k6 tag → OTel attribute (Q10=A)

### 12.1 mapping

k6 sample の Tags (`map[string]string`) を OTel attribute set に `k6.tag.*` prefix で変換:

```text
k6 sample.Tags:
    method = "GET"
    name   = "https://example.com/api"
    status = "200"
    group  = "default"

OTel attributes:
    k6.tag.method = "GET"
    k6.tag.name   = "https://example.com/api"
    k6.tag.status = "200"
    k6.tag.group  = "default"
```

### 12.2 attribute set caching

同じ tag set は何度も繰り返される (typical k6 では同じ URL に多数 request) → U3 と同様に **static set cache** で sync.Map + key=tag map のハッシュ。NFR Design で詳細。

### 12.3 attribute cardinality

k6 tag は通常 cardinality 制御済 (`name` は URL pattern 化されることが多い)。U6 では explicit cardinality limit は設けない (Collector 側で必要なら制限)。

---

## 13. PBT properties (Q11=A)

| ID | 名前 | 種別 |
|---|---|---|
| TP-U6-1 | AddMetricSamples robustness | PBT-03 — nil pipeline / Stop 後でも panic しない |
| TP-U6-2 | Counter monotonicity | PBT-03 — k6 Counter sample → OTel Counter monotonic |
| TP-U6-3 | Tag → Attribute round-trip | PBT-03 — k6 sample.Tags が `k6.tag.<key>` で attribute attached |

詳細は `business-rules.md` §10。

---

## 14. Out of Scope (U6 では扱わない)

- **k6 native metrics 自体の計算**: k6 SDK の責務 (Sample が U6 に渡る前段)
- **OTLP 送信プロトコル**: U4 Pipeline の責務
- **End-of-run summary**: k6 標準機構 (k6 が stdout に出す)
- **synth signal の variant**: U3 / U2 の責務
- **JS API**: U5 の責務
- **Test scenario configuration**: k6 script 側
- **Cardinality control / sampling**: OTel Collector 側
