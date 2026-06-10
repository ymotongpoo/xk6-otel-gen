# U5 k6otelgen — Logical Components

本書は `k6otelgen/` 内の **論理コンポーネント** (LC) を確定する。各 LC について 責務 / 公開 API / 実装スケッチ / 依存関係 を定義。

参照:
- FD: `aidlc-docs/construction/u5-k6otelgen/functional-design/`
- NFR Design Patterns: `nfr-design-patterns.md` (本ディレクトリ内)

---

## コンポーネント一覧

| LC | 名前 | ファイル | 責務 |
|---|---|---|---|
| LC-0 | Package Documentation | `doc.go` | パッケージレベル GoDoc + JS usage example + `--out` warning |
| LC-1 | Root Module | `module.go` | `RootModule` struct + `New` + `NewModuleInstance` + `init()` 登録 |
| LC-2 | Module Instance | `instance.go` | `ModuleInstance` + `Exports()` + jsXxx wrappers + Load/Configure/Stats/Journeys |
| LC-3 | Topology Handle | `handle.go` | `TopologyHandle` + `RunJourney` + `Journeys` (per-VU bound) |
| LC-4 | Config Decoder | `config.go` | `optsToConfig` + `toDuration` + `toStringMap` (JS opts → exporter.Config) |
| LC-5 | Errors | `errors.go` | `ConfigError` type + `throwJSException` helper + `formatErrorMessage` |

---

## LC-0: Package Documentation (`doc.go`)

### 責務
- パッケージ overview
- JS-side usage example (setup → iteration → teardown)
- `--out otel-gen=...` 推奨 warning
- State model (singleton vs per-VU)

### 実装スケッチ
NFR Design Patterns §5.1 参照。

### 依存
- なし

---

## LC-1: Root Module (`module.go`)

### 責務
- `RootModule` 構造体 (process singleton)
- `New() *RootModule`
- `(*RootModule).NewModuleInstance(vu) modules.Instance`
- `init()` で `modules.Register("k6/x/otel-gen", New())`

### 公開 API
```go
type RootModule struct { /* unexported */ }

func New() *RootModule
func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance
```

### 実装スケッチ
```go
func init() {
    modules.Register("k6/x/otel-gen", New())
}

func New() *RootModule {
    return &RootModule{}
}

func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance {
    i := &ModuleInstance{root: r, vu: vu}
    if r.schema == nil {
        return i  // schema not loaded yet; runJourney will surface this via *ConfigError
    }
    pipeline, err := i.getOrBuildPipeline()
    if err != nil {
        i.initErr = err
        return i
    }
    syn := synth.NewDefault(pipeline.TracerProvider(), pipeline.MeterProvider(), pipeline.LoggerProvider())
    seed := uint64(time.Now().UnixNano()) ^ uint64(vu.State().VUID)
    eng := journey.NewEngineWithSeed(r.schema, r.overlay, syn, seed)
    i.synth = syn
    i.engine = eng
    i.handle = &TopologyHandle{
        runtime: vu.Runtime(),
        engine:  eng,
        module:  r,
        instance: i,
        name:    r.loadedPath,
    }
    return i
}

// helper to expose ModuleInstance's vu.Context() to TopologyHandle
func (r *RootModule) currentVUContext() context.Context {
    // not directly resolvable from RootModule alone; handle holds its own
    // reference (see instance field in TopologyHandle)
    panic("unreachable: use TopologyHandle.instance.vu.Context()")
}
```

### 依存
- LC-2 (`ModuleInstance`)
- LC-3 (`TopologyHandle`)
- LC-5 (errors via downstream)
- `go.k6.io/k6/js/modules`, `time`
- `topology`, `synth`, `journey`, `exporter`

---

## LC-2: Module Instance (`instance.go`)

### 責務
- `ModuleInstance` 構造体 (per-VU)
- `Exports()` で JS-callable function 登録
- `jsConfigure` / `jsLoad` / `jsStats` / `jsJourneys` wrapper
- `Load(path)` / `Configure(opts)` / `Stats()` / `Journeys()` の Go-side 実装
- `getOrBuildPipeline()` lazy build

### 公開 API
```go
type ModuleInstance struct { /* unexported */ }

func (i *ModuleInstance) Exports() modules.Exports

// Go-side public (for testability)
func (i *ModuleInstance) Load(path string) (*TopologyHandle, error)
func (i *ModuleInstance) Configure(opts map[string]any) error
func (i *ModuleInstance) Stats() (Stats, error)
func (i *ModuleInstance) Journeys() []string
```

### 実装スケッチ

```go
type ModuleInstance struct {
    root    *RootModule
    vu      modules.VU
    engine  *journey.Engine
    synth   synth.Synthesizer
    handle  *TopologyHandle
    initErr error
}

func (i *ModuleInstance) Exports() modules.Exports {
    return modules.Exports{
        Named: map[string]any{
            "configure": i.jsConfigure,
            "load":      i.jsLoad,
            "stats":     i.jsStats,
            "journeys":  i.jsJourneys,
        },
    }
}

// --- jsXxx wrappers (NFR-D §3.3, §4.1) ---

func (i *ModuleInstance) jsConfigure(call sobek.FunctionCall) sobek.Value {
    var opts map[string]any
    if err := i.vu.Runtime().ExportTo(call.Argument(0), &opts); err != nil {
        throwJSException(i.vu.Runtime(), &ConfigError{Kind: "invalid_opts", Inner: err})
    }
    if err := i.Configure(opts); err != nil {
        throwJSException(i.vu.Runtime(), err)
    }
    return sobek.Undefined()
}

func (i *ModuleInstance) jsLoad(call sobek.FunctionCall) sobek.Value {
    path := call.Argument(0).String()
    handle, err := i.Load(path)
    if err != nil { throwJSException(i.vu.Runtime(), err) }
    return i.vu.Runtime().ToValue(handle)
}

func (i *ModuleInstance) jsStats(call sobek.FunctionCall) sobek.Value {
    s, err := i.Stats()
    if err != nil { throwJSException(i.vu.Runtime(), err) }
    return i.vu.Runtime().ToValue(s)
}

func (i *ModuleInstance) jsJourneys(call sobek.FunctionCall) sobek.Value {
    return i.vu.Runtime().ToValue(i.Journeys())
}

// --- Go-side implementation ---

func (i *ModuleInstance) Load(path string) (*TopologyHandle, error) {
    var err error
    i.root.schemaOnce.Do(func() {
        yaml, ioErr := os.ReadFile(path)
        if ioErr != nil { err = &ConfigError{Kind: "file_not_found", Path: path, Inner: ioErr}; i.root.schemaErr = err; return }
        schema, parseErr := topology.Parse(yaml)
        if parseErr != nil { err = &ConfigError{Kind: "parse_error", Path: path, Inner: parseErr}; i.root.schemaErr = err; return }
        if vErr := schema.Validate(); vErr != nil { err = &ConfigError{Kind: "validate_error", Path: path, Inner: vErr}; i.root.schemaErr = err; return }
        i.root.schema = schema
        i.root.overlay = schema.ApplyFaults()
        i.root.loadedPath = path
    })
    if i.root.schemaErr != nil { return nil, i.root.schemaErr }
    if i.root.loadedPath != path {
        return nil, &ConfigError{Kind: "path_mismatch", Path: path}
    }
    // After load, the handle for this VU instance might not be wired yet (NewModuleInstance was called before load)
    if i.handle == nil {
        // late-init: build per-VU Engine + Synthesizer + Handle now
        if err := i.lateInit(); err != nil { return nil, err }
    }
    return i.handle, nil
}

func (i *ModuleInstance) Configure(opts map[string]any) error {
    var configureErr error
    i.root.configureOnce.Do(func() {
        jsCfg, err := optsToConfig(opts)
        if err != nil { configureErr = err; i.root.configureErr = err; return }
        envCfg := exporter.ConfigFromEnv()
        builtIn := exporter.Config{}
        merged := builtIn.MergeWith(envCfg).MergeWith(jsCfg)
        i.root.config = merged
        i.root.configured = true
    })
    if configureErr != nil { return configureErr }
    if i.root.configureErr != nil { return i.root.configureErr }
    // Second call check
    if i.root.configured && /* this call is not the first */ {
        // sync.Once.Do skipped, signal "already configured"
        // We detect by comparing pre/post state — implementation detail
        return &ConfigError{Kind: "already_configured"}
    }
    return nil
}

// (Stats, Journeys, lateInit, getOrBuildPipeline 同様)
```

> **NOTE on Configure 2nd-call detection**: sync.Once.Do は idempotent で 2 回目以降は no-op。「2 回目」を検出するには:
> - method 先頭で `if i.root.configured { return &ConfigError{Kind: "already_configured"} }` を check して early return
> - sync.Once は **初回 build** のみ guard
> Code Generation Plan で具体的 impl を確定。

### 依存
- LC-1 (RootModule)
- LC-3 (TopologyHandle)
- LC-4 (optsToConfig)
- LC-5 (errors)
- `go.k6.io/k6/js/modules`, `github.com/grafana/sobek`
- `topology`, `exporter`, `synth`, `journey`, `os`

---

## LC-3: Topology Handle (`handle.go`)

### 責務
- `TopologyHandle` 構造体 (per-VU 由来、JS handle 経由で見える)
- `RunJourney(name)` の Go-side 実装 + JS expose
- `Journeys()` (handle method)

### 公開 API
```go
type TopologyHandle struct { /* unexported */ }

// JS-exposed (sobek auto-maps first letter to lowercase)
func (h *TopologyHandle) RunJourney(name string)
func (h *TopologyHandle) Journeys() []string
```

### 実装スケッチ
```go
type TopologyHandle struct {
    runtime  *sobek.Runtime
    engine   *journey.Engine
    module   *RootModule
    instance *ModuleInstance  // for vu.Context() access
    name     string            // loaded YAML path
}

func (h *TopologyHandle) RunJourney(name string) {
    if h.engine == nil {
        throwJSException(h.runtime, &ConfigError{Kind: "not_loaded"})
    }
    plan, err := h.engine.BuildPlan(name)
    if err != nil { throwJSException(h.runtime, err) }
    ctx := h.instance.vu.Context()
    if err := h.engine.Execute(ctx, plan); err != nil {
        throwJSException(h.runtime, err)
    }
}

func (h *TopologyHandle) Journeys() []string {
    if h.engine == nil { return []string{} }
    return h.engine.ListJourneys()
}
```

### 依存
- LC-1 (RootModule reference for state)
- LC-2 (ModuleInstance for vu.Context())
- LC-5 (errors)
- `github.com/grafana/sobek`
- `journey`

---

## LC-4: Config Decoder (`config.go`)

### 責務
- `optsToConfig(opts map[string]any) (exporter.Config, error)`
- `toDuration(v any) (time.Duration, error)` (number ms / string Go duration)
- `toStringMap(v any) (map[string]string, error)` (JS object → header map)
- 10-field JS key 認識 (FD §4)
- 未知 key の warn + ignore (forward-compat)

### 公開 API
- なし (LC-2 のみが呼ぶ internal)

### 実装スケッチ
NFR Design Patterns §4.2 参照。

### 依存
- LC-5 (ConfigError)
- `exporter`, `time`, `strconv`, `fmt`

---

## LC-5: Errors (`errors.go`)

### 責務
- `ConfigError` 型 (FD §1.5, 7-value Kind enum)
- `throwJSException(rt, err)` helper
- `formatErrorMessage(err) string` で Go error 種別を識別し JS-friendly message を構築

### 公開 API
```go
type ConfigError struct {
    Kind  string
    Path  string
    Inner error
}
func (e *ConfigError) Error() string
func (e *ConfigError) Unwrap() error
```

### 実装スケッチ
NFR Design Patterns §3.1, §3.2 参照。

### 依存
- `errors`, `fmt`
- `topology`, `exporter`, `journey` (for type assertion in formatErrorMessage)
- `github.com/grafana/sobek` (NewTypeError)

---

## コンポーネント間依存図

```text
              ┌──────────────────┐
              │ LC-0 doc.go      │
              └──────────────────┘

              ┌──────────────────┐
              │ LC-5 errors.go   │ ◄─── (LC-2, LC-3, LC-4 で使用)
              │ - ConfigError    │
              │ - throwJSException│
              │ - formatError-   │
              │   Message        │
              └──────────────────┘

              ┌──────────────────┐
              │ LC-4 config.go   │ ◄─── (LC-2 が呼ぶ)
              │ - optsToConfig   │
              │ - toDuration     │
              │ - toStringMap    │
              └────────┬─────────┘
                       │
              ┌────────▼─────────┐
              │ LC-2 instance.go │
              │ - ModuleInstance │
              │ - Exports()      │
              │ - jsXxx wrappers │
              │ - Load/Configure │
              │ - Stats/Journeys │
              └────────┬─────────┘
                       │
              ┌────────▼─────────┐
              │ LC-3 handle.go   │
              │ - TopologyHandle │
              │ - RunJourney     │
              │ - Journeys       │
              └────────┬─────────┘
                       │
              ┌────────▼─────────┐
              │ LC-1 module.go   │
              │ - RootModule     │
              │ - New            │
              │ - NewModule-     │
              │   Instance       │
              │ - init() register│
              └──────────────────┘
```

---

## ビルド時の依存外部パッケージ

| 用途 | パッケージ |
|---|---|
| k6 module SDK | `go.k6.io/k6/js/modules` |
| JS runtime (sobek) | `github.com/grafana/sobek` |
| Topology | `github.com/ymotongpoo/xk6-otel-gen/topology` |
| Exporter | `github.com/ymotongpoo/xk6-otel-gen/exporter` |
| Synth | `github.com/ymotongpoo/xk6-otel-gen/synth` |
| Journey | `github.com/ymotongpoo/xk6-otel-gen/journey` |
| stdlib | `sync`, `os`, `path/filepath`, `time`, `strconv`, `fmt`, `errors`, `context` |

---

## テストコンポーネント (Code Generation 時に詳細化)

| テストファイル | LC 対象 | テスト形式 |
|---|---|---|
| `module_test.go` | LC-1 | example-based (init register / New / NewModuleInstance) |
| `instance_test.go` | LC-2 | example-based + Exports surface verify (via modulestest) |
| `handle_test.go` | LC-3 | example-based (RunJourney → mockSynth で record / Engine の ctx 渡し) |
| `config_test.go` | LC-4 | table-driven (各 opts field の decode / type mismatch / unknown key warn) |
| `pbt_test.go` | LC-2, LC-3 | TP-U5-1..3 (rapid + testutil/generators) |
| `helpers_test.go` | (全 LC 共通) | newTestRuntime / newTestRootModule / loadTestSchema / mockSynth |
| `doc_test.go` | LC-0..LC-5 | 2 Example functions (Q10=A) |
| `bench_test.go` | LC-1, LC-2 | BenchmarkNewModuleInstance / BenchmarkRunJourneyOverhead |
| `integration/integration_test.go` | LC 全体 (E2E) | `//go:build integration`、xk6 build + Docker Collector |
| `integration/helpers.go` | (integration 共通) | requireDocker / requireXK6 / buildK6Binary / startCollector / readCollectorFile |
| `integration/script.js` | (test fixture) | k6 JS script |
| `integration/topology.yaml` | (test fixture) | サンプル topology |
| `integration/testdata/collector-config.yaml` | (test fixture) | OTel Collector config |
| `integration/testdata/docker-compose.yaml` | (test fixture) | Docker compose |

---

## U2 coordination 要件 (per-VU seed)

NFR Design Patterns §8 で言及した通り、U2 に `NewEngineWithSeed(schema, overlay, syn, seed uint64) *Engine` を追加する必要あり。

- **U2 への影響**: minor bump (新規 function、backward-compatible)
- **U2 内部実装**: 既存 `NewEngine` は内部で `NewEngineWithSeed(..., time-based seed)` を呼ぶ refactor
- **タイミング**: U5 Code Generation Plan の **Phase 1 (or 早期 Phase)** で U2 patch を独立 phase 化、U5 main implementation の前提とする

NFR Design 段階で **U2 のサブ patch を計画的に含む** ことを明示。

---

## まとめ

- **6 production files** (FD §3 と一致)
- **9 test files** (helpers / doc / bench + integration 子 dir 含む)
- 各 LC は Single Responsibility
- 依存関係は単方向 (LC-0/5 → LC-4 → LC-2 → LC-3 → LC-1)
- 公開 API は FD §4 で確定済 + Go-side testability で `*ModuleInstance.Load/Configure/Stats/Journeys` も public
- U2 への coordination (`NewEngineWithSeed`) は Code Generation Plan で明示 phase 化
- sobek runtime を経由した JS expose は `Exports.Named` map + auto-method-binding (`TopologyHandle.RunJourney` → JS `handle.runJourney`)
