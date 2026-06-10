# U5 k6otelgen — Business Rules

本書は U5 (`k6otelgen/`) の業務規則・不変条件・Testable Properties を確定する。

---

## 1. JS Module Registration

### 1.1 Module path

- 登録 path: `"k6/x/otel-gen"` (k6 慣例 — extension module は `k6/x/<name>`)
- `init()` 関数で `modules.Register("k6/x/otel-gen", New())` を呼ぶ
- `New()` は `*RootModule` を返す (singleton)

### 1.2 RootModule の唯一性

- k6 process 内で `*RootModule` は **1 個** のみ
- `New()` は process あたり 1 回しか呼ばれない (k6 SDK が init で 1 回)
- 全 VU の `ModuleInstance` は同じ `*RootModule` を参照

### 1.3 ModuleInstance

- k6 VU ごとに `NewModuleInstance(vu)` が 1 回呼ばれる
- 各 VU は独自の `*ModuleInstance` を持つ
- `Exports()` が JS-callable object を返す (top-level + handle methods)

---

## 2. Singleton state の規則 (Q2=A + Q4=A + Q5=A)

### 2.1 sync.Once 保護

`RootModule` 内の field:
- `schemaOnce sync.Once` — load() 用
- `configureOnce sync.Once` — configure() 用
- Pipeline は `exporter.GetShared` が内部で sync.Once 持ち

### 2.2 Load(path) の不変条件

- 同じ path で 2 回呼ばれる → 同じ `*TopologyHandle` を返す (cache)
- 異なる path → `*ConfigError{Kind: "already_loaded"}`
- 未 load 状態で `runJourney()` を呼ぶ → `*ConfigError{Kind: "not_loaded"}`

### 2.3 Configure(opts) の不変条件

- 1 回目: `RootModule.config` を build (JS opts > env > defaults)
- 2 回目: `*ConfigError{Kind: "already_configured"}`
- 未 configure で load + runJourney は OK (env + defaults で動作)

### 2.4 順序の制約

```
configure (任意) → load (必須) → runJourney (任意回数)
```

- `configure` を `load` の **後で** 呼んでも OK (Pipeline 構築は最初の runJourney まで遅延されるため)
- `load` なしで `runJourney` → エラー
- `runJourney` を 2 回以上 → OK (同じ Plan を異なる iteration で再利用)

---

## 3. Per-VU state の規則 (Q3=A)

### 3.1 Engine / Synthesizer は per-VU

- `ModuleInstance.engine` — `*journey.Engine` (per-VU instance)
- `ModuleInstance.synth` — `synth.Synthesizer` (per-VU instance)
- 全 VU で Schema / Overlay / Pipeline を共有 (singleton)

### 3.2 Random seed 戦略

- `journey.Engine` 内部の `*rand.Rand` は per-Engine instance なので per-VU で独立 seed
- Seed source は `time.Now().UnixNano() ^ vu.State().VUID` (or 同様の strategy、NFR Design で確定)
- 各 VU で seed が異なれば各 VU の journey 試行が異なるパスを辿る

### 3.3 Per-VU race safety

- `*ModuleInstance` は 1 VU = 1 goroutine context、内部 race なし (k6 SDK 保証)
- `Engine.Execute` は thread-safe (U2 NFR-U2-3)
- `Synthesizer` も thread-safe (U3 NFR-U3-3)

---

## 4. Configure opts decode rules (Q5=A)

### 4.1 JS opts → exporter.Config

| JS key | Type | Config field | 変換 |
|---|---|---|---|
| `endpoint` | string | `Endpoint` | as-is |
| `protocol` | string | `Protocol` | `"grpc"` → `ProtocolGRPC`, `"http"` → `ProtocolHTTP`, else error |
| `insecure` | bool | `Insecure` | as-is |
| `headers` | object | `Headers` | JS object → `map[string]string`、各 value は string 強制 (number → string 変換) |
| `compression` | string | `Compression` | `"gzip"` or `""` のみ受理、その他 error |
| `timeout` | number or string | `Timeout` | number: ms 単位の int → `time.Duration`、string: Go duration format ("10s" 等) |
| `batchSize` | number | `BatchSize` | int 変換 |
| `batchTimeout` | number or string | `BatchTimeout` | timeout と同じ |
| `maxQueueSize` | number | `MaxQueueSize` | int 変換 |
| `resourceOverrides` | object | `ResourceOverrides` | key/value とも string |

### 4.2 不明な key

JS opts に未知の key がある場合 → **warn level log + ignore** (defensive)。エラーにはしない (将来 config 追加時の forward-compatibility)。

### 4.3 不正値

- 型不一致 (例: `endpoint: 123`) → `*ConfigError{Field, Value, Message: "type mismatch"}`
- 範囲不正 (例: `timeout: -1`) → exporter.Config.Validate でエラー化

---

## 5. Load の規則 (Q4=A)

### 5.1 path resolution

```text
load(path):
    if filepath.IsAbs(path):
        absPath = path
    else:
        cwd, _ = os.Getwd()
        absPath = filepath.Join(cwd, path)
    
    yamlBytes, err = os.ReadFile(absPath)
    ...
```

cwd は k6 process の working directory (通常 JS script があるディレクトリではないが、k6 invocation directory)。

### 5.2 YAML パース失敗時

- `*topology.ConfigError` を JS exception 化
- Field / Value / Message を message に含める
- JS 側で catch 可能

### 5.3 Validate 失敗時

- `topology.Schema.Validate()` がエラーを返すと JS exception
- 複数の不変条件違反は errors.Join → JS message に全部含める

### 5.4 ApplyFaults

- `schema.ApplyFaults()` は failure しない API 想定 (U1 確認)
- 失敗時 panic → JS exception 化

---

## 6. RunJourney の規則 (Q6=A + Q9=A)

### 6.1 ctx propagation

- `vu.Context()` を `Engine.Execute(ctx, plan)` に渡す
- ctx は k6 iteration scope (iteration timeout 尊重)
- VU 中断 (k6 Ctrl-C 等) → ctx.Done() → Engine 即時停止 (< 10 ms)

### 6.2 plan の取得

```text
runJourney(name):
    1. plan, err := engine.BuildPlan(name)
       (BuildPlan は cache hit、O(1))
    2. if err: JS exception
    3. err = engine.Execute(ctx, plan)
    4. if err: JS exception
    5. return undefined
```

### 6.3 戻り値

- JS-side 戻り値は **undefined** (success)
- 失敗時は exception
- `runJourney(name).then(...)` のような Promise-like 化は **しない** (k6 JS は sync-only)

---

## 7. Stats / Journeys の規則 (Q7=A)

### 7.1 Stats

```text
stats():
    pipeline, err = r.getOrBuildPipeline()
    if err: exception
    s = pipeline.Stats()
    return {
        tracesExported: s.TracesExported,
        tracesFailed: s.TracesFailed,
        metricsExported: s.MetricsExported,
        metricsFailed: s.MetricsFailed,
        logsExported: s.LogsExported,
        logsFailed: s.LogsFailed,
    }
```

### 7.2 Journeys

```text
journeys() (top-level または handle method):
    if schema == nil:
        return []  (empty)
    return engine.ListJourneys()  // sorted slice
```

未 load 時に空配列を返すか exception 化するかは NFR Design で確定 (現状 empty array を default、defensive)。

---

## 8. Error の JS 表現 (Q9=A)

### 8.1 Mapping table

| Go error | JS exception type | message format |
|---|---|---|
| `*topology.ConfigError` | `TypeError` | `"k6otelgen: topology: <Field>: <Message>"` |
| `*exporter.ConfigError` | `TypeError` | `"k6otelgen: exporter config: <Field>: <Message>"` |
| `*exporter.PipelineError` | `Error` | `"k6otelgen: exporter: <Stage>: <Inner.Error()>"` |
| `*journey.PlanError` | `Error` | `"k6otelgen: plan: <Kind>: <Path joined>"` |
| `*journey.ExecuteError` | `Error` | `"k6otelgen: execute: <Kind>: <Inner.Error()>"` |
| Other | `Error` | `"k6otelgen: <error.Error()>"` |

### 8.2 sobek API

`runtime.NewTypeError(message)` (k6 SDK の sobek runtime) 経由で JS Error object 生成。

### 8.3 ConfigError (U5 自身) の発火条件

- Load: file not found, parse error, validate error, already loaded with different path
- Configure: already configured
- RunJourney: schema not loaded yet
- Stats / Journeys: pipeline build failure (rare)

---

## 9. Lifecycle 契約

### 9.1 k6 init phase

- `init()` で `modules.Register("k6/x/otel-gen", New())` が 1 回呼ばれる
- `New()` は `*RootModule{}` のみ返す (heavy init はしない)
- JS script の **top-level** で `otelgen.load(...)` を呼ぶのは可能だが、setup() で呼ぶ方が一般的

### 9.2 setup phase

- 単一 VU で実行
- `configure()` + `load()` を呼ぶ典型的 pattern
- 返り値は default function に `data` として渡る

### 9.3 default function (per-VU iteration)

- 各 VU で k6 が独立 `*ModuleInstance` を構築
- `runJourney()` がここで呼ばれる
- iteration 中の RunJourney 失敗は iteration を fail させる (JS exception → k6 metric `iterations{outcome=fail}`)

### 9.4 teardown phase

- 単一 VU で実行
- 通常何もしなくて良い (Pipeline shutdown は U6 が担当)

### 9.5 Output.Stop (U6) phase

- k6 が `--out otel-gen=...` を有効化していた場合、`U6 Output.Stop()` が呼ばれる
- そこで `exporter.GetShared().Shutdown(ctx)` を呼ぶ責務

---

## 10. `--out` 未指定時の警告

### 10.1 リスク

`k6 run --out otel-gen=...` を指定せずに run すると:
- U6 が初期化されない → Pipeline.Shutdown が呼ばれない
- 未送信 batch が lost
- OTLP connection が graceful close されない (process exit で強制 close)

### 10.2 ガイダンス

- `doc.go` で明示的に warn: "Run with `--out otel-gen=...` for proper pipeline shutdown"
- Integration test は必ず `--out` を使う
- 将来 `RootModule` に `IsOutputAttached()` 検査 + 警告 log を追加検討 (本 unit では scope 外)

---

## 11. Testable Properties (PBT-01, Q11=A)

### TP-U5-1: Load Idempotency (PBT-04)

```text
For all (yamlContent) drawn from ValidTopologyYAML():
    write to temp file at path
    h1, _ := otelgen.load(path)
    h2, _ := otelgen.load(path)
    h1 == h2  (same TopologyHandle pointer)
```

### TP-U5-2: Configure Merge (PBT-03 Invariant)

```text
For all (jsOpts, envVars) drawn from ValidConfigureOpts() × env:
    set envVars in test
    err := otelgen.configure(jsOpts)
    builtIn := exporter.Config{}
    envCfg := exporter.ConfigFromEnv()
    jsCfg := optsToConfig(jsOpts)
    expected := builtIn.MergeWith(envCfg).MergeWith(jsCfg)
    Assert RootModule.config == expected
```

### TP-U5-3: RunJourney Ctx Passed (PBT-03 Invariant)

```text
For all (yamlContent, journeyName):
    Create test instance with mock Engine that records ctx received
    handle.runJourney(journeyName)
    Assert recorded ctx == vu.Context() (pointer equality)
```

実装は U7 generators (Q12=A) を使う。

---

## 12. パフォーマンスとリソース (FD 時点目安、NFR-R で確定)

| 項目 | 期待値 |
|---|---|
| `init()` 所要時間 | < 100 µs (RootModule 構築のみ) |
| `configure()` 所要時間 | < 500 µs (opts decode + merge) |
| `load()` 所要時間 | < 50 ms (typical YAML、Parse + Validate + ApplyFaults) |
| `getOrBuildPipeline()` 初回 | < 100 ms (NFR-U4-6 と整合) |
| `NewModuleInstance(vu)` 所要時間 | < 1 ms (Engine + Synthesizer 構築) |
| `runJourney(name)` 所要時間 | journey 実行時間 + 数 µs overhead (BuildPlan cache hit + Execute) |
| Process メモリ overhead (RootModule + N VU instance) | < (10MB + 100KB × N) (Pipeline + per-VU Engine) |

---

## 13. Out of Scope (再掲)

`business-logic-model.md` §13 と同じ。
