# U6 k6output — Business Rules

本書は U6 (`k6output/`) の業務規則・不変条件・Testable Properties を確定する。

---

## 1. Output Registration

### 1.1 Output name

- 登録 name: `"otel-gen"` (k6 標準: `--out <name>=<args>`)
- `init()` で `output.RegisterExtension("otel-gen", New)`
- `New(params output.Params) (output.Output, error)` を factory として渡す

### 1.2 New(params) の不変条件

- `params.ConfigArgument` が argument 文字列 (例: `"endpoint=https://...,protocol=grpc"`)
- `params.ScriptOptions`, `params.ScriptPath` 等を opportunistic に Resource attribute に
- nil params は来ない (k6 SDK 保証)
- New() は heavy init をしない (Pipeline 構築は Start() で)

### 1.3 1 process 1 Output instance

`--out otel-gen=... --out otel-gen=...` の重複指定は k6 SDK が拒否 (or 2 個別 instance 構築)。U6 は **同一 process 内で複数 Output instance が動く可能性** を考慮し:
- `exporter.GetShared` は process 全体で sync.Once → 2 個目 Output は cache hit
- queue / flushLoop は per-Output (独立)

---

## 2. `--out args` parsing 規則 (Q1=A)

### 2.1 syntax

```
<arg> := <key>=<value>
<args> := <arg> ("," <arg>)*
```

### 2.2 supported keys と decode

| key | decode |
|---|---|
| `endpoint` | as-is string |
| `protocol` | `"grpc"` → `ProtocolGRPC`, `"http"` → `ProtocolHTTP`, else `*ConfigError{Kind: "invalid_protocol"}` |
| `insecure` | `"true"` / `"false"` parse、他は `*ConfigError{Kind: "type_mismatch"}` |
| `headers` | `key1:val1;key2:val2` 形式、`;` で entry split、`:` で kv split |
| `compression` | `"gzip"` or `""` |
| `timeout` | Go `time.ParseDuration` (e.g., `5s`, `1m`) |
| `batchSize`, `maxQueueSize` | int parse |
| `batchTimeout` | duration parse |
| unknown | warn log + ignore |

### 2.3 不正値

- Type 不一致 → `*ConfigError{Kind: "type_mismatch", Field, Value}` (parsing 段階で)
- 値域外 (e.g., negative timeout) → `*exporter.ConfigError` (Validate 段階で、Pipeline 構築失敗 → k6 run abort)

---

## 3. Config 優先順位 (Q2=A)

```
JS API (U5 otelgen.configure)  >  --out args (U6)  >  env (OTEL_EXPORTER_OTLP_*)  >  built-in defaults
```

`exporter.GetShared` の sync.Once により、**最初に呼ばれた caller の factory が cache される**:
- 通常: U5 setup() で先に `exporter.GetShared(JS-priority factory)` が走る → JS が勝つ
- U5 未使用: U6 `Start()` で `exporter.GetShared(--out args factory)` が走る → `--out` が勝つ
- 両方未指定: env のみ
- env もなければ built-in defaults

---

## 4. Pipeline 取得規則 (Q3=A)

- `Output.Start()` 内で `exporter.GetShared(factory)` を呼ぶ
- factory 内で:
  1. parse 済 `outConfig` を持つ
  2. env config を取得 (`exporter.ConfigFromEnv()`)
  3. built-in defaults
  4. merge: built-in → env → outConfig
  5. `exporter.New(merged)` を呼ぶ

### 4.1 Pipeline 構築失敗

`Output.Start()` が error を return → k6 run abort (Q8=A)。

### 4.2 Pipeline が U5 経由で既構築済

`exporter.GetShared` 内部 sync.Once が cache を返し、U6 factory は呼ばれない。`Output.Start()` は cache から取得した Pipeline を保持。

---

## 5. k6 metric → OTel mapping 規則 (Q4=A)

### 5.1 metric name 変換

`k6.<metric_name>` (e.g., `http_req_duration` → `k6.http.request.duration`)
- `_` → `.` (snake_case → dot.notation)
- prefix `k6.`

### 5.2 metric type → instrument type

| k6 metrics.MetricType | OTel instrument |
|---|---|
| `metrics.Counter` | `Float64Counter` |
| `metrics.Gauge` | `Float64Gauge` (Go SDK v1.x 以降に Gauge type あり)、または `Int64UpDownCounter` で代用 |
| `metrics.Trend` | `Float64Histogram` |
| `metrics.Rate` | `Float64Counter` (failed のみ +1) |

### 5.3 unit

k6 metric には unit info がない場合多い → known metric ごとに hardcoded mapping table を持つ (NFR Design で詳細):

```text
http_req_duration → "ms"
data_sent, data_received → "By" (UCUM)
iterations → "{iteration}"
vus → "{vu}"
```

### 5.4 既知 metric の事前 instrument 構築

k6 が emit する全 metric を Start() 時に発見的に instrument 化することはできない (k6 SDK は metric registry を持つが U6 が enumerate しないかも) → 戦略:
- **dynamic registration**: AddMetricSamples で初出 metric 名を見たら instrument を on-demand 作る (sync.Map cache)
- 既知 metric (`http_req_duration` 等) は Start() で eager build

NFR Design で確定。

---

## 6. Resource attribute 規則 (Q5=A)

### 6.1 必須 attributes

```text
service.name = "xk6-otel-gen-runner"   (固定)
telemetry.sdk.name = "opentelemetry"
telemetry.sdk.language = "go"
telemetry.sdk.version = <runtime detected>
process.runtime.name = "go"
```

### 6.2 任意 attributes

```text
service.version = <build version, if injectable via ldflags>
k6.test.name = <params.ScriptPath last segment>
k6.test.id = <params.RunID, if SDK provides>
host.name = <os.Hostname()>
```

### 6.3 synth Resource との分離

§7 (`business-logic-model.md`) で議論済の通り、U6 は **独自 MeterProvider** を構築し runner Resource を attach。Pipeline の MetricExporter を共有 (= OTLP connection 共有、Resource は別)。

→ **U4 patch (`Pipeline.MetricExporter() sdkmetric.Exporter` 追加)** が必要 (NFR-D で確定)。

---

## 7. Sample batching 規則 (Q6=A)

### 7.1 queue size

- buffered channel 100 entries
- AddMetricSamples が non-blocking
- queue full → warn log + drop oldest (configurable in NFR-D)

### 7.2 flush trigger

- 1 sec ticker
- queue length 閾値 (e.g., 1024 entries 蓄積)
- どちらが先か、whichever 先

### 7.3 flush 内容

- queue から drain した SampleContainer を loop
- 各 Sample について metric type 判定 + instrument 取得 + Record

### 7.4 ordering

- AddMetricSamples 内の順序は保持
- batch 間の順序は flush タイミングに依存 (k6 SDK が ordering を要求しない前提)

---

## 8. Stop() 規則 (Q7=A)

### 8.1 順序

1. `cancelFunc()` で flushLoop に shutdown signal
2. `<-Output.flushDone` で flush 完了を待つ (timeout 5 sec)
3. `Pipeline.Shutdown(ctx)` を timeout 30 sec で呼ぶ
4. Shutdown error は warn log
5. return nil

### 8.2 timeout の意味

- flush 5 sec timeout 過ぎたら強制 break (sample loss 警告)
- Shutdown 30 sec timeout 過ぎたら強制 close

### 8.3 2 度目以降の Stop

`sync.Once` で初回のみ実行。2 度目は no-op。

---

## 9. Start() error 規則 (Q8=A)

| 失敗要因 | error message format |
|---|---|
| outConfig parse failure | `"k6output: invalid --out args: <details>"` |
| Pipeline 構築失敗 | `"k6output: pipeline init: <inner error>"` |
| Runner MeterProvider 構築失敗 | `"k6output: runner meter provider: <inner error>"` |
| Instrument 構築失敗 | `"k6output: instrument <name>: <inner error>"` |

すべて `error` 戻り値で k6 SDK に伝播、k6 run abort。

---

## 10. Testable Properties (Q11=A)

### TP-U6-1: AddMetricSamples Robustness (PBT-03)

```text
For any sequence of:
    Output.Start() → Output.AddMetricSamples(...) → Output.Stop()
    or Output.AddMetricSamples(...)  (without Start)
    or Output.Stop() → Output.AddMetricSamples(...)
No panic occurs.
```

実装: rapid で sequence を生成、各操作後 panic recover で検証。

### TP-U6-2: Counter Monotonicity (PBT-03)

```text
For any sequence of k6.Sample with metric.Type == Counter and positive Value:
    After all are processed via AddMetricSamples,
    OTel Counter value (read via ManualReader) is the sum of all sample.Values.
```

### TP-U6-3: Tag → Attribute Round-trip (PBT-03)

```text
For any k6.Sample with Tags map:
    After AddMetricSamples + flush + ManualReader.Collect:
    For each (key, val) in sample.Tags:
        attribute "k6.tag.<key>" == val is present in the data point.
```

---

## 11. パフォーマンスとリソース (FD 時点目安、NFR-R で確定)

| 項目 | 期待値 |
|---|---|
| `New(params)` 所要時間 | < 100 µs (args parse のみ) |
| `Start()` 所要時間 | < 100 ms (NFR-U4-6 と整合: Pipeline 構築 < 100 ms 込み) |
| `AddMetricSamples(N samples)` overhead | < 1 µs / sample (queue push のみ) |
| `Stop()` 所要時間 | < 30 sec (Pipeline.Shutdown timeout) |
| queue メモリ | < 1 MB (100 entries × 10 KB / entry) |
| flush loop goroutine | 1 個 |

---

## 12. Error 型

`*ConfigError{Kind, Field, Value, Inner}` — U6 内部の args parsing 等のエラー。
- Kind 値: `"invalid_args"`, `"invalid_protocol"`, `"type_mismatch"`, `"invalid_url"`
- exporter / journey の error は wrap せず生で返す

---

## 13. Out of Scope (再掲)

`business-logic-model.md` §14 と同じ。
