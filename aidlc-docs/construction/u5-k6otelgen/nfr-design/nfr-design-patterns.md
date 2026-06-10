# U5 k6otelgen — NFR Design Patterns

本書は U5 (`k6otelgen/`) の **「どう実装するか」** のパターン群を確定する。FD + NFR-R を受けて、Performance / Concurrency / Error / API / Documentation / Test の各カテゴリで実装パターンを決める。

参照:
- FD: `aidlc-docs/construction/u5-k6otelgen/functional-design/`
- NFR-R: `aidlc-docs/construction/u5-k6otelgen/nfr-requirements/`
- Plan + Answers: `aidlc-docs/construction/plans/u5-k6otelgen-nfr-d-plan.md`

---

## 1. Performance パターン

### 1.1 RootModule struct layout (Q1=A)

```go
type RootModule struct {
    // Load state — guarded by schemaOnce
    schemaOnce sync.Once
    schemaErr  error
    schema     *topology.Schema
    overlay    *topology.FaultOverlay
    loadedPath string

    // Configure state — guarded by configureOnce
    configureOnce sync.Once
    configureErr  error
    config        exporter.Config
    configured    bool

    // Cached handle (constructed lazily in load())
    handle *TopologyHandle
}
```

理由:
- sync.Once guard を field 隣接配置で読みやすく
- err state を Once 結果として cache、複数 caller が同じ err を取得
- handle は load 成功後に確定、以降 read-only

### 1.2 ModuleInstance ↔ RootModule reference (Q2=A)

```go
type ModuleInstance struct {
    root *RootModule       // back-reference
    vu   modules.VU
    engine *journey.Engine
    synth  synth.Synthesizer
    handle *TopologyHandle // bound to this VU's engine
    initErr error          // captured Pipeline/synth/engine build error
}
```

- 直接 pointer、interface 抽象なし
- testability は modulestest.NewRuntime + RootModule injection で確保

### 1.3 Pipeline lazy build (Q7=A)

```go
func (i *ModuleInstance) getOrBuildPipeline() (*exporter.Pipeline, error) {
    return exporter.GetShared(func() (*exporter.Pipeline, error) {
        return exporter.New(i.root.config)
    })
}
```

`exporter.GetShared` の sync.Once が process 全体で 1 回 build を保証 (U4 既設計)。初回呼び出し timing:
- `runJourney(name)` 初回: Engine ready → pipeline 必要
- `stats()` 初回: snapshot 取得に pipeline 必要
- `journeys()`: pipeline 不要 (schema を直接参照)

### 1.4 Per-VU instance 構築 (NFR-U5-4)

```go
func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance {
    i := &ModuleInstance{root: r, vu: vu}

    // schema/overlay は load() 経由で既に確定済前提。未確定なら i.engine は nil 残し、
    // runJourney 時に "schema not loaded" を return
    if r.schema == nil {
        return i  // partial instance
    }

    pipeline, err := i.getOrBuildPipeline()
    if err != nil {
        i.initErr = err
        return i  // initErr 経由で JS exception 化
    }

    syn := synth.NewDefault(
        pipeline.TracerProvider(),
        pipeline.MeterProvider(),
        pipeline.LoggerProvider(),
    )
    eng := journey.NewEngine(r.schema, r.overlay, syn)
    i.synth = syn
    i.engine = eng
    i.handle = &TopologyHandle{
        runtime: vu.Runtime(),
        engine:  eng,
        module:  r,
        name:    r.loadedPath,
    }
    return i
}
```

NFR-U5-4 < 5 ms target:
- synth.NewDefault: NFR-U3 で eager 9 instrument 構築 (< 1 ms 期待)
- journey.NewEngine: schema を loop して全 Plan を eager build (< 1 ms 期待 NFR-U2-6)
- 合計 < 5 ms は十分達成可能

---

## 2. Concurrency パターン

### 2.1 Singleton state は sync.Once (NFR-U5-3)

`schemaOnce.Do` / `configureOnce.Do` で初回呼び出しのみ実行、err を field に cache。複数 VU が並列に load()/configure() を呼んでも 1 回しか走らない。

### 2.2 Per-VU state は VU-local

- `*ModuleInstance` は k6 が VU ごとに 1 個構築
- 1 VU = 1 goroutine (k6 保証)
- VU iteration の連続呼び出しは sequential
- VU 間の干渉なし

### 2.3 Per-VU random seed (Q6=A)

`journey.NewEngine` 内部の `*rand.Rand` 構築時に:
```go
seed := uint64(time.Now().UnixNano()) ^ uint64(vu.State().VUID)
rng := rand.New(rand.NewPCG(seed, seed^0xdeadbeefcafebabe))
```

→ U2 の `journey.NewEngine` には `seed` 引数を渡せないが (NewEngine signature 既設計)、内部で `newDefaultRand()` を呼ぶ。NFR-D-U2 では seed 戦略を NFR Design で確定としていた → U5 から seed を渡す仕組みが必要。

**実装オプション (NFR Design 確定事項)**:
- **Option A**: U2 に `NewEngineWithSeed(schema, overlay, syn, seed uint64) *Engine` を追加 — U2 minor bump
- **Option B**: U5 内で Engine 構築後 `Engine.SetRandSeed(seed)` を呼ぶ — U2 に SetRandSeed method 追加
- **推奨 Option A**: U5 が `journey.NewEngineWithSeed(schema, overlay, syn, computedSeed)` を呼ぶ。Code Generation Phase で U2 patch (NFR-D-U2 §1.2 SeedDefault 既述の余地、breaking なし)

### 2.4 Concurrency safety verification

- `go test -race -count=1 ./k6otelgen/...` が pass
- 並列 VU から `Exports()` 経由で `Load` / `Configure` / `RunJourney` 呼び出すスモークテスト

---

## 3. Error パターン

### 3.1 JS exception 化ヘルパー (Q8=A)

```go
// throwJSException converts a Go error to a JS exception and panics
// (sobek convention). The caller is responsible for being inside a
// sobek FunctionCall wrapper, where the panic is captured by sobek
// and turned into a JS exception for the caller.
func throwJSException(rt *sobek.Runtime, err error) {
    msg := formatErrorMessage(err)
    panic(rt.NewTypeError(msg))
}

func formatErrorMessage(err error) string {
    var (
        ce  *ConfigError
        ec  *exporter.ConfigError
        pe  *exporter.PipelineError
        ple *journey.PlanError
        ee  *journey.ExecuteError
    )
    switch {
    case errors.As(err, &ce):
        return fmt.Sprintf("k6otelgen: [%s] %s", ce.Kind, ce.Error())
    case errors.As(err, &ec):
        return fmt.Sprintf("k6otelgen: exporter config: %s", ec.Error())
    case errors.As(err, &pe):
        return fmt.Sprintf("k6otelgen: exporter pipeline: %s", pe.Error())
    case errors.As(err, &ple):
        return fmt.Sprintf("k6otelgen: plan: %s", ple.Error())
    case errors.As(err, &ee):
        return fmt.Sprintf("k6otelgen: execute: %s", ee.Error())
    default:
        return fmt.Sprintf("k6otelgen: %s", err.Error())
    }
}
```

### 3.2 ConfigError type (FD §1.5)

```go
type ConfigError struct {
    Kind  string  // "already_loaded" | "already_configured" | "not_loaded" | "path_mismatch" | "file_not_found" | "parse_error" | "validate_error"
    Path  string
    Inner error
}

func (e *ConfigError) Error() string {
    if e.Inner != nil {
        return fmt.Sprintf("k6otelgen: %s (%s): %v", e.Kind, e.Path, e.Inner)
    }
    return fmt.Sprintf("k6otelgen: %s (%s)", e.Kind, e.Path)
}

func (e *ConfigError) Unwrap() error { return e.Inner }
```

### 3.3 jsXxx wrapper の defer recover

```go
func (i *ModuleInstance) jsLoad(call sobek.FunctionCall) sobek.Value {
    // Recover any panic that's NOT from throwJSException, convert to JS error
    defer func() {
        if r := recover(); r != nil {
            // sobek's NewTypeError-induced panic re-panics for JS to catch.
            // Other panics (programmer error) are wrapped as JS error.
            if jsExc, ok := r.(*sobek.Exception); ok {
                panic(jsExc)  // re-panic for sobek to handle
            }
            panic(i.vu.Runtime().NewTypeError(fmt.Sprintf("k6otelgen: internal panic: %v", r)))
        }
    }()
    path := call.Argument(0).String()
    handle, err := i.Load(path)
    if err != nil { throwJSException(i.vu.Runtime(), err) }
    return i.vu.Runtime().ToValue(handle)
}
```

---

## 4. API パターン

### 4.1 Exports() dispatch (Q3=A)

```go
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
```

各 `jsXxx` は `func(call sobek.FunctionCall) sobek.Value` 形式の wrapper。

### 4.2 JS opts decode (Q4=A + Q5=A)

```go
// optsToConfig converts decoded JS opts map to an exporter.Config.
// Unknown keys are warn-logged and ignored (forward compat).
// Type mismatches return *ConfigError{Kind: "type_mismatch", ...}.
func optsToConfig(opts map[string]any) (exporter.Config, error) {
    var c exporter.Config
    for k, v := range opts {
        switch k {
        case "endpoint":
            s, ok := v.(string)
            if !ok { return c, &ConfigError{Kind: "type_mismatch", Path: "endpoint"} }
            c.Endpoint = s
        case "protocol":
            s, ok := v.(string)
            if !ok { return c, &ConfigError{Kind: "type_mismatch", Path: "protocol"} }
            switch s {
            case "grpc":
                c.Protocol = exporter.ProtocolGRPC
            case "http":
                c.Protocol = exporter.ProtocolHTTP
            default:
                return c, &ConfigError{Kind: "invalid_protocol", Path: s}
            }
        case "insecure":
            b, ok := v.(bool)
            if !ok { return c, &ConfigError{Kind: "type_mismatch", Path: "insecure"} }
            c.Insecure = b
        case "headers":
            m, err := toStringMap(v)
            if err != nil { return c, &ConfigError{Kind: "type_mismatch", Path: "headers", Inner: err} }
            c.Headers = m
        case "compression":
            // ...
        case "timeout":
            d, err := toDuration(v)
            if err != nil { return c, &ConfigError{Kind: "type_mismatch", Path: "timeout", Inner: err} }
            c.Timeout = d
        case "batchSize", "batchTimeout", "maxQueueSize":
            // ...
        case "resourceOverrides":
            // ...
        default:
            // unknown key — warn + ignore
            // (in production: log via k6 SDK if available; otherwise no-op)
        }
    }
    return c, nil
}

// toDuration accepts number (ms) or string (Go duration format)
func toDuration(v any) (time.Duration, error) {
    switch x := v.(type) {
    case int64:    return time.Duration(x) * time.Millisecond, nil
    case float64:  return time.Duration(x) * time.Millisecond, nil
    case string:   return time.ParseDuration(x)
    default:       return 0, fmt.Errorf("expected number or string, got %T", v)
    }
}

func toStringMap(v any) (map[string]string, error) {
    raw, ok := v.(map[string]any)
    if !ok { return nil, fmt.Errorf("expected object, got %T", v) }
    out := make(map[string]string, len(raw))
    for k, val := range raw {
        s, ok := val.(string)
        if !ok {
            // attempt number → string conversion for convenience
            switch x := val.(type) {
            case int64:   s = strconv.FormatInt(x, 10)
            case float64: s = strconv.FormatFloat(x, 'f', -1, 64)
            default:      return nil, fmt.Errorf("header %s value %T not coercible to string", k, val)
            }
        }
        out[k] = s
    }
    return out, nil
}
```

### 4.3 TopologyHandle method binding (Q9=A)

```go
type TopologyHandle struct {
    runtime *sobek.Runtime
    engine  *journey.Engine
    module  *RootModule
    name    string
}

// JS-callable: handle.runJourney(name)
func (h *TopologyHandle) RunJourney(name string) {
    plan, err := h.engine.BuildPlan(name)
    if err != nil { throwJSException(h.runtime, err) }
    ctx := h.contextForExecute()
    if err := h.engine.Execute(ctx, plan); err != nil {
        throwJSException(h.runtime, err)
    }
}

func (h *TopologyHandle) Journeys() []string {
    return h.engine.ListJourneys()
}

func (h *TopologyHandle) contextForExecute() context.Context {
    // ModuleInstance への back-reference 経由で vu.Context() を取得
    // (実装は instance.go で h.instance を持つ形にする)
    return h.module.currentVUContext()
}
```

> **NOTE**: `currentVUContext()` の実装は ModuleInstance への参照経路が必要。設計案として:
> - `*TopologyHandle` に `*ModuleInstance` の back-reference を持たせる (per-VU で安全)
> - or `vu modules.VU` を直接 handle に embed
>
> 後者の方が直接的 → handle struct 内に `vu modules.VU` を field として置く。NFR-D で確定。

### 4.4 sobek の auto-method-expose

sobek は Go struct method を自動的に JS object property として expose する (first letter を lowercase 化)。`TopologyHandle.RunJourney` → JS の `handle.runJourney`。

---

## 5. Documentation パターン

### 5.1 `doc.go` の構造 (Q10=A)

```go
// Package k6otelgen registers the "k6/x/otel-gen" k6 extension module.
//
// JS usage:
//
//   import otelgen from "k6/x/otel-gen";
//
//   export function setup() {
//       otelgen.configure({
//           endpoint: "https://otel.example.com:4317",
//           protocol: "grpc",
//           insecure: false,
//       });
//       return { topology: otelgen.load("./topology.yaml") };
//   }
//
//   export default function (data) {
//       data.topology.runJourney("checkout-flow");
//   }
//
// IMPORTANT: Always run with --out otel-gen=... to ensure the exporter
// Pipeline is shut down cleanly. Without --out, unsent OTLP batches may
// be lost at k6 process exit because the Pipeline.Shutdown is invoked
// only by the U6 k6output module's Output.Stop() lifecycle hook:
//
//   k6 run --out otel-gen=... script.js
//
// State model:
//   - Process singleton: Topology, FaultOverlay, exporter Pipeline.
//     Loaded once via otelgen.load() in setup().
//   - Per-VU: journey.Engine + synth.Synthesizer. Constructed in
//     NewModuleInstance(vu) per VU.
//
package k6otelgen
```

`--out` warning は package comment 内に明示。

### 5.2 Example function (Q10 Q10=A NFR-R)

`doc_test.go`:

```go
func ExampleNew() {
    rm := k6otelgen.New()
    _ = rm
    // Output:
}

func ExampleRootModule_NewModuleInstance() {
    // Typically called by k6 SDK, not user code:
    //   rm := k6otelgen.New()
    //   inst := rm.NewModuleInstance(vu)
}
```

最小限。JS user は package comment の JS example を読む。

### 5.3 GoDoc 網羅性

全 Go exported identifier に doc comment、`revive` enforce。

---

## 6. Test パターン

### 6.1 Helper layout (Q11=A)

`helpers_test.go`:

```go
package k6otelgen

import (
    "testing"

    "go.k6.io/k6/js/modulestest"
)

// newTestRuntime constructs a modulestest.Runtime with k6/x/otel-gen
// registered. Use for tests that exercise the JS API surface end-to-end.
func newTestRuntime(t *testing.T) *modulestest.Runtime {
    t.Helper()
    rt := modulestest.NewRuntime(t)
    // Register the module so `import otelgen from "k6/x/otel-gen"` works
    require.NoError(t, rt.SetupModuleSystem(map[string]any{
        "k6/x/otel-gen": New(),
    }))
    return rt
}

// newTestRootModule returns a fresh RootModule for unit tests that
// don't need the full JS runtime (e.g., optsToConfig tests).
func newTestRootModule(t *testing.T) *RootModule {
    t.Helper()
    return New()
}

// loadTestSchema writes the given YAML to a temp file and calls
// rootModule.load via the runtime.
func loadTestSchema(t *testing.T, rt *modulestest.Runtime, yaml string) {
    t.Helper()
    path := writeTempYAML(t, yaml)
    _, err := rt.RunOnEventLoop(fmt.Sprintf(`
        const otelgen = require("k6/x/otel-gen");
        otelgen.load(%q);
    `, path))
    require.NoError(t, err)
}

// mockSynth is a synth.Synthesizer test double.
type mockSynth struct {
    mu       sync.Mutex
    spans    []synth.SpanInput
    metrics  []synth.MetricInput
    logs     []synth.LogInput
    contexts []context.Context
}
func newMockSynth() *mockSynth { return &mockSynth{} }
func (m *mockSynth) BeginSpan(ctx context.Context, in synth.SpanInput) (context.Context, synth.FinishSpanFunc) {
    m.mu.Lock()
    m.spans = append(m.spans, in)
    m.contexts = append(m.contexts, ctx)
    m.mu.Unlock()
    return ctx, func(synth.Outcome) {}
}
func (m *mockSynth) RecordMetric(ctx context.Context, in synth.MetricInput) {
    m.mu.Lock(); m.metrics = append(m.metrics, in); m.mu.Unlock()
}
func (m *mockSynth) EmitLog(ctx context.Context, in synth.LogInput) {
    m.mu.Lock(); m.logs = append(m.logs, in); m.mu.Unlock()
}
```

### 6.2 PBT (TP-U5-1〜3)

`pbt_test.go` に集約。`testutil/generators/k6otelgen_inputs.go` の `ValidConfigureOpts` / `ValidLoadPath` を使用。

```go
// TP-U5-1: Load Idempotency
func TestLoad_Idempotent_Property(t *testing.T) {
    t.Parallel()
    rapid.Check(t, func(t *rapid.T) {
        // generate a valid YAML, write to temp file, call load() twice,
        // assert same handle pointer
    })
}

// TP-U5-2: Configure Merge
func TestConfigure_Merge_Property(t *testing.T) {
    t.Parallel()
    rapid.Check(t, func(t *rapid.T) {
        // generate jsOpts + envVars, set envs, call configure(jsOpts),
        // compute expected = builtIn.MergeWith(envCfg).MergeWith(jsCfg),
        // assert RootModule.config == expected
    })
}

// TP-U5-3: RunJourney Ctx Passed
func TestRunJourney_CtxPassed_Property(t *testing.T) {
    t.Parallel()
    rapid.Check(t, func(t *rapid.T) {
        // construct test instance with mockSynth that records ctx received,
        // call handle.runJourney(name),
        // assert recorded ctx is the vu.Context() pointer
    })
}
```

### 6.3 Integration test の xk6 build flow (Q12=A)

`k6otelgen/integration/integration_test.go`:

```go
//go:build integration

func TestIntegration_EndToEnd(t *testing.T) {
    // 1. Skip if Docker / xk6 unavailable
    requireDocker(t)
    xk6 := requireXK6(t)

    // 2. Build k6 binary with this extension (via xk6 build)
    binPath := buildK6Binary(t, xk6, "github.com/ymotongpoo/xk6-otel-gen")

    // 3. Start Collector via docker compose
    collectorAddr, cleanup := startCollector(t, "./testdata")
    defer cleanup()

    // 4. Run k6 script with --out otel-gen=<collectorAddr>
    output := runK6Script(t, binPath, "./testdata/script.js",
        "--out", fmt.Sprintf("otel-gen=endpoint=%s", collectorAddr),
    )
    require.Zero(t, output.ExitCode)

    // 5. Read Collector JSON files, assert spans/metrics/logs received
    traces := readCollectorFile(t, "/var/log/otel/traces.json")
    assert.GreaterOrEqual(t, traces.SpanCount, 1)
}

func buildK6Binary(t *testing.T, xk6Path, modulePath string) string {
    binPath := filepath.Join(t.TempDir(), "k6")
    cmd := exec.Command(xk6Path, "build", "--with", modulePath+"=.")
    cmd.Env = append(os.Environ(), "K6_VERSION=latest")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    require.NoError(t, cmd.Run())
    return binPath
}
```

### 6.4 unit test の sync.Once 隔離

`RootModule` の `sync.Once` が cross-test 干渉しないように、各 test で **新規 `RootModule` を作る**:

```go
func TestLoad_HappyPath(t *testing.T) {
    t.Parallel()
    rm := newTestRootModule(t)  // fresh per test
    // ...
}
```

→ `New()` が毎回新規 RootModule を返すなら問題なし (k6 SDK は process あたり 1 個しか使わないが、test では複数構築 OK)。

---

## 7. NFR-R Open Questions の解消

| Open Question | 確定 |
|---|---|
| Multiple topology 同時 load | 不採用、Q4 のまま single topology |
| `otelgen.shutdown()` JS API | 不採用、U6 経由 |
| Configure partial merge | 不採用、Q5=A single-shot |
| JS event hook | 不採用、scope 外 |

すべて future-revisit。

---

## 8. U2 への coordination 要件 (random seed 戦略)

§2.3 で言及した通り、U5 から journey.Engine に per-VU seed を渡す必要あり。

### 8.1 提案: U2 `NewEngineWithSeed` 追加

```go
// In U2 journey/engine.go:
// (existing) func NewEngine(schema, overlay, syn) *Engine
//   - internally uses newDefaultRand() with time-based seed

// NEW: func NewEngineWithSeed(schema, overlay, syn, seed uint64) *Engine
//   - uses rand.New(rand.NewPCG(seed, seed^0xdeadbeefcafebabe))
```

### 8.2 SemVer

新規 function 追加 → U2 minor bump、backward-compatible。

### 8.3 タイミング

Code Generation Plan で **「Phase: Add NewEngineWithSeed to journey」** を独立 phase として登録、U5 phase の前提に。

---

## 9. NFR-R 各項目との対応

| NFR-R | 対応する Design パターン |
|---|---|
| NFR-U5-1 (API stability) | §4 全般、§4.2 opts decode contract |
| NFR-U5-2 (ConfigError.Kind SemVer) | §3.2 ConfigError type + enum |
| NFR-U5-3 (Singleton lifecycle) | §1.1 sync.Once layout |
| NFR-U5-4 (Per-VU lifecycle) | §1.4 NewModuleInstance、< 5 ms target を全 step 軽量化で達成 |
| NFR-U5-5 (Concurrency) | §2 全般 |
| NFR-U5-6 (Performance soft target) | §1, §4.2 のシンプル実装 |
| NFR-U5-7 (Memory) | §1.4 で per-VU 構築コストを必要最小限に |
| NFR-U5-8 (No self-metric) | RootModule / ModuleInstance に counter なし |
| NFR-U5-9 (Documentation) | §5 全般、`--out` warning §5.1 |
| NFR-U5-10 (Testability) | §6 全般 |
| NFR-U5-11 (Shutdown delegation) | §5.1 doc.go 明示、Go code に Shutdown 呼び出しなし |
| NFR-U5-12 (Filesystem) | os.ReadFile を使うのみ、追加 sanitize なし |

---

## 10. Anti-patterns (採用しない)

| アンチパターン | 不採用理由 |
|---|---|
| `*RootModule` を nested struct で grouping (Q1 案 B) | flat layout がシンプル |
| ModuleInstance ↔ RootModule を interface (Q2 案 B) | over-engineering |
| sobek `Define` で property 1 つずつ登録 (Q3 案 B) | `Exports.Named` で十分 |
| reflection-based auto dispatch (Q3 案 C) | magic 多すぎ |
| sobek.Object methods で 1 field ずつ decode (Q4 案 B) | 冗長 |
| JSON.stringify round-trip (Q4 案 C) | overhead |
| Pipeline を NewModuleInstance で eager build (Q7 案 B) | VU 構築コスト増、利点小 |
| Pipeline を Load 中で build (Q7 案 C) | setup() で失敗が iteration まで遅延しない代わりに timing 矛盾 |
| Error 型ごとに別 helper (Q8 案 B) | DRY 違反 |
| Go error を JS object として return (Q8 案 C) | sobek 慣例違反 |
| JS-side class-based API (Q9 案 C) | sobek 慣例違反 |
| Runtime warning log で `--out` 不検出 (Q10 案 B/C) | spammy / invasive |
| Test 用に `internal/k6otelgentest` package を作る (Q11 案 C) | 過剰 |
| CI 事前 build k6 binary (Q12 案 B) | local test 困難 |
| Docker container 内で xk6 build (Q12 案 C) | overhead 大 |
| `errors.go` を `config.go` に同居 (Q13 案 B) | 種別の独立性失う、grep しにくい |

---

## 11. 設計確定 — Open Items

12 / 13 質問確定。残り 1 つの open:

- **§2.3 の per-VU seed strategy** で U2 `NewEngineWithSeed` の追加が必要 → Code Generation Plan で別 Phase として登録 (U2 patch + U5 が新 API を呼ぶ)
