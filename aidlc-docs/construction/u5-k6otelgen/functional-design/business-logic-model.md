# U5 k6otelgen — Business Logic Model

本書は `k6otelgen/` パッケージの **ビジネスロジック (k6 JS module の動作)** を確定する。

参照: Application Design (`component-methods.md` §C5)、`plans/u5-k6otelgen-fd-plan.md` の Q1..Q13 回答。

---

## 1. パッケージの責務

`k6otelgen/` は **k6 JS Module SDK** を介して JS script から本拡張機能を呼べるようにする「fronted」layer:
- JS-callable API (`load`, `configure`, `stats`, `journeys`, `topology.runJourney`) の提供
- Process singleton state (Topology / FaultOverlay / Pipeline) の管理
- Per-VU state (Engine / Synthesizer) の構築
- Go error → JS exception 変換
- VU iteration ctx → Engine.Execute への propagation

### 1.1 関連 unit との関係

```text
              ┌──────────────────────────────────┐
              │   k6 JS script (user-authored)   │
              │   import otelgen from            │
              │     "k6/x/otel-gen"              │
              │   otelgen.load("./topology.yaml")│
              │   topology.runJourney(...)       │
              └─────────────┬────────────────────┘
                            │ (JS API call)
                            ▼
              ┌──────────────────────────────────┐
              │ U5 k6otelgen (this unit)         │
              │ - RootModule (singleton state)   │
              │   * Schema, FaultOverlay,         │
              │     Pipeline                      │
              │ - ModuleInstance (per-VU)         │
              │   * Engine, Synthesizer           │
              │ - TopologyHandle (JS handle)      │
              └─┬──────┬──────┬──────┬───────────┘
                │      │      │      │
                ▼      ▼      ▼      ▼
              U1 topology  U4 exporter  U3 synth  U2 journey
              (Parse,      (GetShared,  (NewDefault,  (NewEngine,
              Validate,    Pipeline,    Synthesizer)  BuildPlan,
              ApplyFaults) Stats)                     Execute)
```

### 1.2 状態の境界 (Q2=A + Q3=A)

| State | Scope | Where |
|---|---|---|
| `*topology.Schema` | Process singleton | `RootModule.schema` (sync.Once-guarded) |
| `*topology.FaultOverlay` | Process singleton | `RootModule.overlay` |
| `*exporter.Pipeline` | Process singleton | `exporter.GetShared` (U4 提供、`RootModule` から factory 経由で構築) |
| `*journey.Engine` | Per-VU | `ModuleInstance.engine` |
| `synth.Synthesizer` | Per-VU | `ModuleInstance.synth` |
| `*TopologyHandle` | Per-VU (TS-bound) | `ModuleInstance.handle` |

理由:
- **Schema / Overlay**: 読み取り専用、process 全体で 1 個。複製はメモリ無駄
- **Pipeline**: OTLP connection を 1 process で共有 (U4 設計通り)
- **Engine**: random source を持つ → per-VU instance で seed 独立、journey の試行が VU 間で独立
- **Synthesizer**: thread-safe だが per-VU で構築のオーバーヘッドは僅か (eager instrument creation のメモリ ~10KB × VU 数)

---

## 2. k6 JS Module の lifecycle

### 2.1 k6 phase ごとの動作

```text
k6 process start
    │
    │ init code (top-level of JS file が実行)
    ▼
[U5] init() in module.go が k6 SDK に register
    │ modules.Register("k6/x/otel-gen", New())
    │ ← RootModule struct instance 1 個 (process singleton)
    │
    ▼
[k6] setup() を 1 回実行 (single VU で)
    │ otelgen.configure({...}) ── RootModule に Config 設定 (sync.Once)
    │ otelgen.load("./topology.yaml") ── RootModule に Schema/Overlay 設定 + Pipeline 構築
    │   ↓
    │   exporter.GetShared(factory) で Pipeline 構築
    │   synth.NewDefault(...) は per-VU で後で行う (Pipeline は singleton から取得)
    │
    ▼
[k6] per-VU iteration 開始
    │ k6 が各 VU で NewModuleInstance(vu) を 1 回呼ぶ
    │ ← ModuleInstance 構築 (Engine + Synthesizer を per-VU で組み立てる)
    │   * tp/mp/lp は exporter.GetShared() 経由で取得
    │   * synth.NewDefault(tp, mp, lp) → per-VU Synthesizer
    │   * journey.NewEngine(schema, overlay, synth) → per-VU Engine
    │
    │ default function 内で:
    │   const t = data.topology;   // setup から渡された TopologyHandle
    │   t.runJourney("checkout")   // ModuleInstance.engine.Execute(vu.Context(), plan) を呼ぶ
    │
    ▼
[k6] teardown() (任意)
    │ JS script の teardown function 内で何もしなくて OK
    │
    ▼
[U6] Output.Stop() が k6 lifecycle 完了時に呼ばれる
    │ exporter.Pipeline.Shutdown(ctx) を呼ぶ → OTLP flush + connection close
    │
    ▼
k6 process exit
```

### 2.2 init() の登録

`k6otelgen/module.go` 内で `init()` 関数:
```go
func init() {
    modules.Register("k6/x/otel-gen", New())
}
```

→ k6 process 起動時に `import otelgen from "k6/x/otel-gen"` で resolve される。

---

## 3. JS-callable API (Q1=A)

### 3.1 Top-level methods

```javascript
import otelgen from "k6/x/otel-gen";

// Configuration (typically in setup())
otelgen.configure(opts);      // → Go: RootModule.configure(opts)
const t = otelgen.load(path); // → Go: RootModule.load(path) returns TopologyHandle
const stats = otelgen.stats();   // → Go: RootModule.stats() returns Stats object
const names = otelgen.journeys(); // → Go: RootModule.journeys() returns string[]
```

### 3.2 TopologyHandle methods

```javascript
const t = otelgen.load(path);
t.runJourney(name);  // → Go: handle.runJourney(name)
t.journeys();        // → Go: handle.journeys() (alias of otelgen.journeys() for convenience)
```

### 3.3 Argument 変換

JS values は sobek 経由で Go side に渡る:
- string → Go string
- number → Go float64 / int
- object (`{ key: value }`) → Go `map[string]any` または struct fields に reflect

`configure(opts)` の opts は `{endpoint, protocol, insecure, headers, timeout, ...}` の object、Go side で `exporter.Config` に decode (詳細は `business-rules.md` §4)。

---

## 4. Load(path) の semantics (Q4=A)

### 4.1 動作フロー

```text
otelgen.load(path):
    1. RootModule.schemaOnce.Do(func() {
            yamlBytes, err := os.ReadFile(path)  // ← file system access
            if err != nil { setError(...); return }
            schema, err := topology.Parse(yamlBytes)
            if err != nil { setError(...); return }
            if err := schema.Validate(); err != nil { setError(...); return }
            overlay := schema.ApplyFaults()
            
            RootModule.schema  = schema
            RootModule.overlay = overlay
            RootModule.loadedPath = path
       })
    2. If error from sync.Once: return JS exception
    3. If path != RootModule.loadedPath: return *ConfigError "topology already loaded with different path"
    4. return TopologyHandle (cached, same handle for repeated load(path))
```

### 4.2 path の解決

- 絶対 path → そのまま
- 相対 path → k6 script の working directory を基準 (k6 SDK convention)
- file open failure → JS exception

### 4.3 cache semantics

- 同じ path で `load()` を 2 回呼ぶ → 同じ `*TopologyHandle` (cache hit)
- 異なる path で 2 回目 → `*ConfigError`
- これにより JS script が偶然複数回 `load()` を呼んでも安全

---

## 5. Configure(opts) の semantics (Q5=A)

### 5.1 優先順位 merge (U4 と整合)

```text
otelgen.configure(opts):
    1. RootModule.configureOnce.Do(func() {
            jsCfg := optsToConfig(opts)
            envCfg := exporter.ConfigFromEnv()
            builtIn := exporter.Config{} // zero value で fillDefaults が埋める
            
            // 優先順位: jsCfg > envCfg > builtIn
            merged := builtIn.MergeWith(envCfg).MergeWith(jsCfg)
            
            RootModule.config = merged
       })
    2. If 2 回目以降の呼び出し: return *ConfigError "already configured"
    3. return nil (success)
```

### 5.2 Configure 省略時

- `configure()` を呼ばずに `load()` + `runJourney()` を呼ぶ → 環境変数 + built-in defaults で動く
- `OTEL_EXPORTER_OTLP_ENDPOINT` 等が未設定なら built-in default (localhost:4317)

### 5.3 opts → Config decode

`business-rules.md` §4 で詳述。代表的 mapping:

| JS key | Config field |
|---|---|
| `endpoint` | `Endpoint string` |
| `protocol` | `Protocol Protocol` (`"grpc"` / `"http"`) |
| `insecure` | `Insecure bool` |
| `headers` | `Headers map[string]string` |
| `compression` | `Compression string` |
| `timeout` | `Timeout time.Duration` (ms 単位の number / "10s" 形式の string を accept) |
| `batchSize` | `BatchSize int` |
| `batchTimeout` | `BatchTimeout time.Duration` |
| `maxQueueSize` | `MaxQueueSize int` |
| `resourceOverrides` | `ResourceOverrides map[string]string` |

---

## 6. Pipeline 構築 timing

### 6.1 Lazy construction (load + configure 後)

Pipeline は `exporter.GetShared(factory)` 経由で構築:

```go
func (r *RootModule) getOrBuildPipeline() (*exporter.Pipeline, error) {
    return exporter.GetShared(func() (*exporter.Pipeline, error) {
        return exporter.New(r.config)  // r.config は configure() / env で確定済
    })
}
```

- 初回 `runJourney()` 前に呼ばれる
- 失敗時 `*PipelineError` を JS exception 化
- `exporter.GetShared` の sync.Once により最初の builder が cache、以降 cache hit

### 6.2 Tracer / Meter / Logger Provider の取得

```go
pipeline, err := r.getOrBuildPipeline()
if err != nil { return err }
tp := pipeline.TracerProvider()
mp := pipeline.MeterProvider()
lp := pipeline.LoggerProvider()
```

これらを `synth.NewDefault(tp, mp, lp)` に渡し per-VU Synthesizer を構築。

---

## 7. Per-VU instance 構築

### 7.1 NewModuleInstance(vu) のフロー

k6 が VU ごとに 1 回呼ぶ:

```go
func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance {
    // r.schema / r.overlay / r.config はすでに setup() 経由で確定済前提
    if r.schema == nil {
        // setup を呼ばずに iteration が始まったケース (defensive)
        // → init phase で JS API が呼ばれた時に late init するか、エラー返す
    }
    pipeline, err := r.getOrBuildPipeline()
    if err != nil {
        // JS exception 化は Exports() 内の wrapper で行う
        return &ModuleInstance{vu: vu, initErr: err}
    }
    syn := synth.NewDefault(pipeline.TracerProvider(), pipeline.MeterProvider(), pipeline.LoggerProvider())
    eng := journey.NewEngine(r.schema, r.overlay, syn)
    handle := &TopologyHandle{module: r, engine: eng, name: r.loadedPath}
    return &ModuleInstance{
        vu:     vu,
        root:   r,
        engine: eng,
        synth:  syn,
        handle: handle,
    }
}
```

### 7.2 Per-VU の lifetime

- VU が k6 process 中 1 個の `*ModuleInstance` を持ち続ける
- 全 iteration で同じ `Engine` を使う (Engine は内部状態を持たないので OK)
- VU 数 = 100 なら Engine instance 100 個 (Plans cache は schema 共通だが Engine 構造体は分離)

### 7.3 Memory consideration

- Engine.plans map は `*Plan` ポインタ参照 (Plan tree 実体は schema からの slice 参照)
- VU=1000 で Engine instance × 1000 のメモリ overhead はおおよそ < 100 MB (Engine ~10KB × 1000 + plans map / VU)
- NFR Design / NFR-R で詳細閾値確定

---

## 8. RunJourney の semantics (Q6=A)

### 8.1 フロー

```text
handle.runJourney(name):
    1. plan, err := handle.engine.BuildPlan(name)
    2. If err: return JS exception (PlanError)
    3. ctx := handle.module.instance.vu.Context()   // VU iteration の ctx
    4. err := handle.engine.Execute(ctx, plan)
    5. If err: return JS exception (ExecuteError)
    6. return undefined (JS-side success)
```

### 8.2 Ctx propagation

- `modules.VU.Context()` は **iteration scope ctx** を返す
- k6 が iteration timeout (例: `iteration_duration: '30s'`) を持っていれば、Engine.Execute が `ctx.Done()` を尊重して停止 (U2 NFR-U2-4)
- VU が中断されたら Engine.Execute は即時 (< 10 ms) 戻る

### 8.3 Error 種別 (Q9=A)

| Go error type | JS exception form |
|---|---|
| `*topology.ConfigError` | TypeError with message |
| `*exporter.PipelineError` | TypeError with message |
| `*journey.PlanError` | TypeError with name + message |
| `*journey.ExecuteError` | TypeError with message |
| その他 generic | Error |

すべて `sobek.NewTypeError` (or 同等の API) で JS Error object を構築、`error.Error()` の文字列を message に。

---

## 9. Stats() の semantics (Q7=A)

### 9.1 動作

```text
otelgen.stats():
    pipeline, err := r.getOrBuildPipeline()
    if err: return JS exception
    s := pipeline.Stats()
    return JS object {
        tracesExported:  s.TracesExported,
        tracesFailed:    s.TracesFailed,
        metricsExported: s.MetricsExported,
        metricsFailed:   s.MetricsFailed,
        logsExported:    s.LogsExported,
        logsFailed:      s.LogsFailed,
    }
```

### 9.2 用途

主にデバッグ・観測用。JS 側で:
```javascript
console.log(JSON.stringify(otelgen.stats()));
```

Production の k6 dashboard で表示する場合は U6 (k6output) 経由で k6 metrics として emit する方が望ましい (本 unit ではこの拡張は scope 外)。

---

## 10. Journeys() の semantics

### 10.1 動作

```text
otelgen.journeys() (top-level) または handle.journeys():
    1. If schema not loaded: return [] (empty array) または exception (NFR Design で確定)
    2. return engine.ListJourneys() のコピー
```

### 10.2 用途

JS 側で動的に journey を選んで実行するパターン用。

```javascript
const all = otelgen.journeys();  // ["checkout", "browse", "search"]
const pick = all[Math.floor(Math.random() * all.length)];
topology.runJourney(pick);
```

---

## 11. Pipeline Shutdown (Q8=A)

### 11.1 責務委譲

- U5 は **Shutdown を呼ばない**
- U6 (k6output) の `Output.Stop()` が `exporter.GetShared` の Pipeline を取得して `Shutdown(ctx)` を呼ぶ責務
- k6 lifecycle で `--out otel-gen=...` が指定されていれば自動的に呼ばれる

### 11.2 `--out` 未指定時の挙動

警告: `--out otel-gen=...` を指定しない場合、Shutdown が呼ばれず:
- 未送信 batch が flush されない可能性
- OTLP connection が無 explicit close (process exit でクリーンアップされる、ただし graceful ではない)

→ FD `business-rules.md` §10 でユーザーへの guidance として明記。Integration test では `--out` を必ず使う前提。

---

## 12. PBT properties (Q11=A)

| ID | 名前 | 種別 | 概要 |
|---|---|---|---|
| TP-U5-1 | Load Idempotency | PBT-04 | 同じ path で複数回 load → 同じ TopologyHandle |
| TP-U5-2 | Configure Merge | PBT-03 Invariant | env vars + JS opts → exporter.Config が U4 MergeWith と整合 |
| TP-U5-3 | RunJourney Ctx Passed | PBT-03 Invariant | RunJourney 内で Engine.Execute に `vu.Context()` が渡る |

詳細は `business-rules.md` §11、Code Generation 時に実装スケッチ確定。

---

## 13. Out of Scope (U5 では扱わない)

- **k6 native metrics 変換**: U6 (k6output) の責務
- **Pipeline Shutdown**: U6 の責務
- **Telemetry signal 構築**: U3 (synth) の責務
- **Journey 実行ロジック**: U2 (journey) の責務
- **YAML parsing**: U1 (topology) の責務
- **OTLP 送信**: U4 (exporter) の責務
- **k6 binary build**: U8 (distribution) の責務
