# U5 (k6otelgen) — Code Generation Plan

> **This file is the Single Source of Truth (SSOT) for the U5 implementation.**
>
> **Audience**: Codex CLI (`gpt-5.5 xhigh`) + Cursor Composer 2.5.
>
> **Recommended agent per Phase**:
> - **Phase 0-14 → Codex** via `scripts/run-codex.sh u5`. Phase 1 patches U2 (`journey.NewEngineWithSeed`) before U5 main implementation.
> - **Phase 11 (U7 generator addition) MAY be split off to Cursor batch**.
>
> **Execution model**: top-to-bottom, mark `[ ]` → `[x]` immediately upon completion.
>
> **Source artifacts**:
> - FD: `aidlc-docs/construction/u5-k6otelgen/functional-design/{business-logic-model,business-rules,domain-entities}.md`
> - NFR-R: `aidlc-docs/construction/u5-k6otelgen/nfr-requirements/{nfr-requirements,tech-stack-decisions}.md`
> - NFR-D: `aidlc-docs/construction/u5-k6otelgen/nfr-design/{nfr-design-patterns,logical-components}.md` ← **most prescriptive**
> - Application Design: `aidlc-docs/inception/application-design/component-methods.md` §C5
> - U2 completed journey package: `journey/` (Engine, BuildPlan, Execute) — adding NewEngineWithSeed in Phase 1
> - U3 completed synth package: `synth/`
> - U4 completed exporter package: `exporter/`
> - U1 completed topology package: `topology/`
> - PBT rules: `.aidlc-rule-details/extensions/testing/property-based/property-based-testing.md`
> - Agent contract: `AGENTS.md`

---

## Unit Context

- **Unit ID**: U5
- **Purpose**: Implement `k6otelgen/` — k6 JS module registered as `k6/x/otel-gen` providing load/configure/stats/journeys/runJourney APIs.
- **Workspace root**: `/home/ymotongpoo/repos/xk6-otel-gen/`
- **Go module path**: `github.com/ymotongpoo/xk6-otel-gen`
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → U2 ✓ → **U5 (this)** → U6 → U8
- **PBT requirements satisfied by this unit**:
  - **PBT-01** (Property Identification) — 3 properties (TP-U5-1..3)
  - **PBT-03** (Invariants) — TP-U5-2, TP-U5-3
  - **PBT-04** (Idempotency) — TP-U5-1
- **NFR DoD** (from `nfr-requirements.md` §4):
  - `go build ./k6otelgen/...` succeeds
  - `go vet ./k6otelgen/...` clean
  - `go test -race -count=1 ./k6otelgen/...` passes
  - `go test -cover ./k6otelgen/...` ≥ 80%
  - PBT TP-U5-1..3 all pass
  - All Go exported identifiers have GoDoc
  - 2 Example functions
  - U7 generators (`ValidConfigureOpts/AnyConfigureOpts`, `ValidLoadPath/AnyLoadPath`) added
  - `golangci-lint run ./k6otelgen/...` passes
  - Integration test (`-tags=integration ./k6otelgen/integration/...`) passes with xk6-built k6 binary + Docker Collector
  - `doc.go` includes `--out otel-gen=...` warning
- **Dependencies added by this unit**:
  - `go.k6.io/k6` (k6 SDK) — direct
  - `github.com/grafana/sobek` — direct (k6 transitive but used directly)
  - No new external from xk6-otel-gen tree
- **U2 coordination required (Phase 1)**: add `journey.NewEngineWithSeed(schema, overlay, syn, seed uint64) *Engine` (minor SemVer bump, backward-compatible new function).

---

## Phase 0 — Environment Setup
**Recommended agent**: Codex CLI.

### Step 0.1 — Add k6 SDK deps

- [x] `go get go.k6.io/k6@latest` → records `go.k6.io/k6` in go.mod as a direct dependency.
- [x] `go get github.com/grafana/sobek@latest` if not already pulled by k6 transitively.
- [x] `go mod tidy`.
- [x] Verify: `grep -E "go.k6.io/k6|grafana/sobek" go.mod` shows both.

### Step 0.2 — Create k6otelgen/ skeleton

- [x] Create `k6otelgen/` directory.
- [x] Create empty `k6otelgen/doc.go` placeholder.
- [x] Verify: `go build ./k6otelgen/...` succeeds.

### Phase 0 commit

- [x] `git add go.mod go.sum k6otelgen/doc.go && git commit -m "build(k6otelgen): add k6 SDK and sobek dependencies"`

---

## Phase 1 — U2 Coordination: Add `journey.NewEngineWithSeed`
**Recommended agent**: Codex CLI.

> Patch U2 to support per-VU deterministic seeding required by U5. This is a minor SemVer bump (backward-compatible new function).

### Step 1.1 — Modify `journey/engine.go`

- [ ] Add `NewEngineWithSeed(schema *topology.Schema, overlay *topology.FaultOverlay, syn synth.Synthesizer, seed uint64) *Engine`:
  - same behavior as `NewEngine`, but uses `rand.NewPCG(seed, seed^0xdeadbeefcafebabe)` instead of `newDefaultRand()`
- [ ] Refactor existing `NewEngine(schema, overlay, syn) *Engine` to call `NewEngineWithSeed(..., timeBasedSeed)` where `timeBasedSeed = uint64(time.Now().UnixNano())`.
- [ ] All identifiers have GoDoc.

### Step 1.2 — Add `journey/engine_test.go::TestNewEngineWithSeed`

- [ ] Verify same seed produces deterministic Execute outcomes (with mockSynth recording RunJourney args, observe deterministic random ints / replica idx).
- [ ] Verify different seed produces different sequences (statistical, with high probability).
- [ ] Existing `TestNewEngine_*` tests still pass.

### Step 1.3 — Update U2 NFR-D / FD docs

- [ ] In `aidlc-docs/construction/u2-journey/nfr-design/nfr-design-patterns.md` §1.2 (Random source), add note that `NewEngineWithSeed` exposes seed for caller (U5).
- [ ] In `aidlc-docs/construction/u2-journey/functional-design/domain-entities.md`, add `NewEngineWithSeed` to the public API table.

### Phase 1 commit

- [ ] `git add journey/engine.go journey/engine_test.go aidlc-docs/construction/u2-journey/ && git commit -m "feat(journey): add NewEngineWithSeed for per-VU deterministic seeding"`

---

## Phase 2 — Errors & ConfigError type (LC-5)
**Recommended agent**: Codex CLI.

### Step 2.1 — Create `k6otelgen/errors.go`

- [ ] Define `ConfigError struct { Kind, Path string; Inner error }` with `Error()` + `Unwrap()` per NFR-D §3.2.
- [ ] Define `throwJSException(rt *sobek.Runtime, err error)` per NFR-D §3.1.
- [ ] Define `formatErrorMessage(err error) string` with errors.As switching over ConfigError / exporter.ConfigError / exporter.PipelineError / journey.PlanError / journey.ExecuteError / generic.
- [ ] All exported identifiers have GoDoc.

### Step 2.2 — Unit test `k6otelgen/errors_test.go`

- [ ] `TestConfigError_Error` formatting with and without Inner.
- [ ] `TestConfigError_Unwrap`.
- [ ] `TestFormatErrorMessage_*` for each error type (table-driven).
- [ ] All tests call `t.Parallel()`.

### Phase 2 commit

- [ ] `git add k6otelgen/errors.go k6otelgen/errors_test.go && git commit -m "feat(k6otelgen): add ConfigError type and JS exception helpers"`

---

## Phase 3 — Config Decoder (LC-4)
**Recommended agent**: Codex CLI.

### Step 3.1 — Create `k6otelgen/config.go`

- [ ] Implement `optsToConfig(opts map[string]any) (exporter.Config, error)` per NFR-D §4.2:
  - 10 known fields (endpoint, protocol, insecure, headers, compression, timeout, batchSize, batchTimeout, maxQueueSize, resourceOverrides)
  - type assertions with `*ConfigError{Kind: "type_mismatch", Path, Inner}` on failure
  - protocol values: "grpc" / "http", others → `*ConfigError{Kind: "invalid_protocol"}`
  - unknown keys: silently ignore (forward-compat) — log via k6 SDK if available, else no-op
- [ ] Implement `toDuration(v any) (time.Duration, error)`:
  - int64 / float64 → milliseconds
  - string → time.ParseDuration
  - other → error
- [ ] Implement `toStringMap(v any) (map[string]string, error)`:
  - map[string]any → map[string]string with numeric coercion (int / float → strconv)
- [ ] Internal helpers, brief comments.

### Step 3.2 — Unit test `k6otelgen/config_test.go`

- [ ] `TestOptsToConfig_AllFields_HappyPath`.
- [ ] `TestOptsToConfig_TypeMismatch_*` (table-driven for each field).
- [ ] `TestOptsToConfig_InvalidProtocol`.
- [ ] `TestOptsToConfig_UnknownKey_Ignored`.
- [ ] `TestToDuration_*` (number / string / error).
- [ ] `TestToStringMap_*` (basic / numeric coercion / error).
- [ ] All tests call `t.Parallel()`.

### Phase 3 commit

- [ ] `git add k6otelgen/config.go k6otelgen/config_test.go && git commit -m "feat(k6otelgen): add JS opts decoder with timeout/headers coercion"`

---

## Phase 4 — TopologyHandle (LC-3)
**Recommended agent**: Codex CLI.

### Step 4.1 — Create `k6otelgen/handle.go`

- [ ] Define `TopologyHandle struct` with fields per NFR-D §LC-3 (runtime, engine, module, instance, name).
- [ ] Implement `(*TopologyHandle).RunJourney(name string)`:
  - If engine == nil → throwJSException with `*ConfigError{Kind: "not_loaded"}`
  - `engine.BuildPlan(name)` → on err throwJSException
  - Get ctx from `h.instance.vu.Context()`
  - `engine.Execute(ctx, plan)` → on err throwJSException
- [ ] Implement `(*TopologyHandle).Journeys() []string`:
  - If engine == nil → return empty slice
  - return `engine.ListJourneys()`
- [ ] All exported identifiers have GoDoc.

### Step 4.2 — Note about test deferral

Tests for TopologyHandle require ModuleInstance / RootModule which land in Phases 5-6. Add minimal compile-only validation in this phase, full tests in Phase 6.

### Phase 4 commit

- [ ] `git add k6otelgen/handle.go && git commit -m "feat(k6otelgen): add TopologyHandle with RunJourney and Journeys"`

---

## Phase 5 — RootModule (LC-1)
**Recommended agent**: Codex CLI.

### Step 5.1 — Create `k6otelgen/module.go`

- [ ] Define `RootModule struct` with all fields per NFR-D §1.1 (schemaOnce / schemaErr / schema / overlay / loadedPath / configureOnce / configureErr / config / configured / handle).
- [ ] Implement `init()` calling `modules.Register("k6/x/otel-gen", New())`.
- [ ] Implement `New() *RootModule` returning fresh zero state.
- [ ] Implement `(*RootModule).NewModuleInstance(vu modules.VU) modules.Instance`:
  - Construct `*ModuleInstance` with root + vu
  - If schema not loaded, return partial instance (lateInit will fire on Load)
  - If schema loaded: getOrBuildPipeline, create synth.NewDefault, journey.NewEngineWithSeed with `time.UnixNano() XOR vu.State().VUID`, create handle
- [ ] All exported identifiers have GoDoc.

### Step 5.2 — Unit test `k6otelgen/module_test.go`

- [ ] `TestNew_ReturnsZeroState`.
- [ ] `TestNewModuleInstance_BeforeLoad_PartialInstance` (schema nil OK).
- [ ] `TestNewModuleInstance_AfterLoad_BuildsEngine` (need to load first via Phase 6, may need to inline simple yaml).
- [ ] Helper `newTestRootModule(t) *RootModule` available (placeholder; full helpers in Phase 6).
- [ ] All tests call `t.Parallel()`.

### Phase 5 commit

- [ ] `git add k6otelgen/module.go k6otelgen/module_test.go && git commit -m "feat(k6otelgen): add RootModule with init registration and NewModuleInstance"`

---

## Phase 6 — ModuleInstance (LC-2) + Test Helpers
**Recommended agent**: Codex CLI.

### Step 6.1 — Create `k6otelgen/instance.go`

- [ ] Define `ModuleInstance struct` with all fields per NFR-D §1.2.
- [ ] Implement `(*ModuleInstance).Exports() modules.Exports` with 4 jsXxx wrappers per NFR-D §4.1.
- [ ] Implement `(*ModuleInstance).jsConfigure / jsLoad / jsStats / jsJourneys` wrappers per NFR-D §4.2:
  - sobek FunctionCall extraction
  - call Go-side method
  - throwJSException on error
  - sobek.Undefined() or runtime.ToValue() return
- [ ] Implement `(*ModuleInstance).Load(path string) (*TopologyHandle, error)`:
  - sync.Once Do: ReadFile + Parse + Validate + ApplyFaults
  - on err: set root.schemaErr, return wrapped *ConfigError
  - on subsequent call with different path: `*ConfigError{Kind: "path_mismatch"}`
  - lateInit if handle nil (Engine + Synthesizer + Handle construction)
  - return cached handle
- [ ] Implement `(*ModuleInstance).Configure(opts map[string]any) error`:
  - if root.configured → `*ConfigError{Kind: "already_configured"}`
  - sync.Once Do: optsToConfig + ConfigFromEnv + MergeWith chain
  - set root.config + root.configured = true
- [ ] Implement `(*ModuleInstance).Stats() (Stats, error)`:
  - getOrBuildPipeline → on err return *ConfigError
  - pipeline.Stats() → map to local `Stats` struct with JS-friendly field names
- [ ] Implement `(*ModuleInstance).Journeys() []string`:
  - if root.schema == nil → return []string{}
  - if i.engine == nil → return []string{} (or root cache)
  - return engine.ListJourneys()
- [ ] Implement `(*ModuleInstance).getOrBuildPipeline() (*exporter.Pipeline, error)`:
  - exporter.GetShared with factory using root.config
- [ ] Implement `(*ModuleInstance).lateInit() error`:
  - getOrBuildPipeline
  - synth.NewDefault
  - journey.NewEngineWithSeed(..., per-VU seed)
  - construct handle
- [ ] Define `Stats struct` with JS-friendly field tags (sobek auto-naming).
- [ ] All exported identifiers have GoDoc.

### Step 6.2 — Create `k6otelgen/helpers_test.go`

- [ ] `newTestRuntime(t) *modulestest.Runtime` per NFR-D §6.1.
- [ ] `newTestRootModule(t) *RootModule`.
- [ ] `loadTestSchema(t, runtime, yaml string)` — write yaml to temp file, run JS to load.
- [ ] `mockSynth` struct + constructor (used in handle_test, pbt_test).
- [ ] All helpers use `t.Helper()`.

### Step 6.3 — Create `k6otelgen/instance_test.go`

- [ ] `TestExports_Names` — Exports().Named has 4 keys.
- [ ] `TestLoad_HappyPath` via runtime — yaml load → handle returned.
- [ ] `TestLoad_PathMismatch_ReturnsError` — load same RootModule twice with different paths.
- [ ] `TestLoad_InvalidYAML_ReturnsError`.
- [ ] `TestConfigure_HappyPath`.
- [ ] `TestConfigure_AlreadyConfigured_Error`.
- [ ] `TestConfigure_Merge_JSOverridesEnv` — use t.Setenv to set env, then configure with JS opts, assert root.config matches expected merge.
- [ ] `TestStats_BeforePipeline_Error` (or HappyPath if pipeline lazy build works without RunJourney).
- [ ] `TestJourneys_BeforeLoad_Empty`.
- [ ] `TestJourneys_AfterLoad_Sorted`.

### Step 6.4 — Create `k6otelgen/handle_test.go`

- [ ] `TestHandle_RunJourney_HappyPath` — mockSynth captures BeginSpan, assert it was called.
- [ ] `TestHandle_RunJourney_UnknownJourney_ThrowsError`.
- [ ] `TestHandle_RunJourney_CtxPassed` — verify ctx received by mockSynth == vu.Context() pointer.
- [ ] `TestHandle_Journeys_BeforeLoad_Empty`.
- [ ] `TestHandle_Journeys_AfterLoad_Returns`.

### Phase 6 commit

- [ ] `git add k6otelgen/instance.go k6otelgen/instance_test.go k6otelgen/handle_test.go k6otelgen/helpers_test.go && git commit -m "feat(k6otelgen): add ModuleInstance with JS API wrappers and Go implementations"`

---

## Phase 7 — Documentation (LC-0) + doc_test.go
**Recommended agent**: Codex CLI.

### Step 7.1 — Replace `k6otelgen/doc.go` placeholder

- [ ] Full package documentation per NFR-D §5.1:
  - JS usage example (setup → iteration → teardown)
  - `--out otel-gen=...` IMPORTANT warning
  - State model (singleton vs per-VU)

### Step 7.2 — Create `k6otelgen/doc_test.go`

- [ ] `ExampleNew` — `rm := k6otelgen.New(); _ = rm`. Output: empty.
- [ ] `ExampleRootModule_NewModuleInstance` — comment-based usage hint, no executable code needed (mock vu construction is complex).
- [ ] Examples must compile.

### Phase 7 commit

- [ ] `git add k6otelgen/doc.go k6otelgen/doc_test.go && git commit -m "docs(k6otelgen): add package documentation with --out warning and Example functions"`

---

## Phase 8 — U7 Generator Additions
**Recommended agent**: Codex CLI (or Cursor batch).

### Step 8.1 — Add `testutil/generators/k6otelgen_inputs.go`

- [ ] Implement `ValidConfigureOpts(opts ...ConfigureOptsOption) *rapid.Generator[map[string]any]` per FD §7.2:
  - generates JS-friendly map with valid endpoint / protocol / insecure / headers / timeout etc.
- [ ] Implement `AnyConfigureOpts(opts ...ConfigureOptsOption) *rapid.Generator[map[string]any]`:
  - allows invalid values (negative timeout, unknown protocol, malformed types)
- [ ] Implement `ValidLoadPath(opts ...LoadPathOption) *rapid.Generator[string]`:
  - generates temp dir based relative paths
- [ ] Implement `AnyLoadPath(opts ...LoadPathOption) *rapid.Generator[string]`:
  - allows traversal patterns / unicode / very long paths

### Step 8.2 — Add `testutil/generators/k6otelgen_inputs_test.go`

- [ ] Property tests that valid generators produce only invariant-respecting values.

### Phase 8 commit

- [ ] `git add testutil/generators/k6otelgen_inputs.go testutil/generators/k6otelgen_inputs_test.go && git commit -m "feat(testutil): add k6otelgen input generators for U5 PBT"`

---

## Phase 9 — PBT (TP-U5-1..3)
**Recommended agent**: Codex CLI.

### Step 9.1 — Create `k6otelgen/pbt_test.go`

- [ ] `TestLoad_Idempotent_Property` (TP-U5-1):
  - generate ValidLoadPath, write yaml to it, load twice via runtime, assert same handle
- [ ] `TestConfigure_Merge_Property` (TP-U5-2):
  - generate ValidConfigureOpts + env vars (t.Setenv), call configure, compute expected merge, assert root.config matches
- [ ] `TestRunJourney_CtxPassed_Property` (TP-U5-3):
  - construct test instance with mockSynth, call handle.runJourney, assert mockSynth.contexts contains vu.Context() pointer
- [ ] All tests call `t.Parallel()`.

### Phase 9 commit

- [ ] `git add k6otelgen/pbt_test.go && git commit -m "test(k6otelgen): add PBT for TP-U5-1..3"`

---

## Phase 10 — Benchmark
**Recommended agent**: Codex CLI.

### Step 10.1 — Create `k6otelgen/bench_test.go`

- [ ] `BenchmarkNewModuleInstance` — fresh RootModule + loaded schema, repeated NewModuleInstance calls (simulating VU creation).
- [ ] `BenchmarkLoad` — fresh RootModule + load() (single call due to sync.Once).
- [ ] `BenchmarkConfigure` — fresh RootModule + Configure(opts).
- [ ] Soft target verification: NewModuleInstance < 5ms.
- [ ] `b.ReportAllocs()`.

### Phase 10 commit

- [ ] `git add k6otelgen/bench_test.go && git commit -m "test(k6otelgen): add benchmarks for NewModuleInstance/Load/Configure"`

---

## Phase 11 — Integration Test Harness
**Recommended agent**: Codex CLI.

### Step 11.1 — Create `k6otelgen/integration/testdata/topology.yaml`

- [ ] Minimal valid topology with 2-3 services, 1-2 journeys.

### Step 11.2 — Create `k6otelgen/integration/testdata/script.js`

- [ ] k6 JS script using `otelgen.configure`, `otelgen.load`, default function with `topology.runJourney`.

### Step 11.3 — Create `k6otelgen/integration/testdata/collector-config.yaml` + `docker-compose.yaml`

- [ ] OTel Collector config with file_exporter, docker-compose pinning collector-contrib image.

### Step 11.4 — Create `k6otelgen/integration/helpers.go`

- [ ] `requireDocker(t)`, `requireXK6(t)` — skip integration tests if tools missing.
- [ ] `buildK6Binary(t, xk6Path, modulePath, outputDir) string` — exec.Command for `xk6 build --with <modulePath>=.`.
- [ ] `startCollector(t, configDir) (endpoint string, cleanup func())`.
- [ ] `runK6Script(t, k6Bin, scriptPath, ...args) output` — exec k6 binary.
- [ ] `readCollectorTraces(t, path) tracesContent`.

### Step 11.5 — Create `k6otelgen/integration/integration_test.go`

- [ ] `//go:build integration`.
- [ ] `TestIntegration_EndToEnd`:
  - requireDocker + requireXK6
  - buildK6Binary
  - startCollector with file_exporter
  - runK6Script with `--out otel-gen=<endpoint>`
  - assertExitCode == 0
  - readCollectorTraces + assert spans received

### Step 11.6 — Create `k6otelgen/integration/README.md`

- [ ] Document Docker + xk6 requirement, invocation.

### Phase 11 commit

- [ ] `git add k6otelgen/integration/ && git commit -m "test(k6otelgen): add integration test harness with xk6 build and Docker Collector"`

---

## Phase 12 — Final Wrap & DoD Verification
**Recommended agent**: Codex CLI.

### Step 12.1 — Run full suite

- [ ] `go build ./...` succeeds.
- [ ] `go vet ./k6otelgen/...` clean.
- [ ] `go test -race -count=1 ./...` passes.
- [ ] `go test -cover ./k6otelgen/...` ≥ 80%.
- [ ] `go test -bench=. -benchmem ./k6otelgen/...` shows NewModuleInstance < 5ms.
- [ ] `golangci-lint run ./k6otelgen/...` passes.
- [ ] `go test -tags=integration ./k6otelgen/integration/...` passes (with Docker + xk6).

### Step 12.2 — Create `aidlc-docs/construction/u5-k6otelgen/code/code-generation-summary.md`

- [ ] File list with line counts (production + test + integration).
- [ ] Verification results.
- [ ] Deviations from plan.
- [ ] Recent commits.

### Step 12.3 — Mark all plan checkboxes [x]

- [ ] Walk back; verify all `[ ]` are `[x]`.

### Step 12.4 — Update `aidlc-docs/aidlc-state.md`

- [ ] Mark U5 complete. Set Current Unit to U6 (k6 Output Module).

### Phase 12 commit

- [ ] `git add aidlc-docs/ && git commit -m "chore(u5-k6otelgen): finalize code-generation-summary and checkbox state"`

---

## Anti-patterns to AVOID during implementation

(Per NFR-D `nfr-design-patterns.md` §10)

- ❌ Nested struct grouping in RootModule (Q1 案 B)
- ❌ ModuleInstance ↔ RootModule via interface (Q2 案 B)
- ❌ sobek.Define per-property registration (Q3 案 B)
- ❌ Reflection-based auto dispatch (Q3 案 C)
- ❌ Manual sobek.Object methods for opts decode (Q4 案 B)
- ❌ JSON.stringify round-trip for opts (Q4 案 C)
- ❌ Eager Pipeline build in NewModuleInstance (Q7 案 B)
- ❌ Build Pipeline inside Load (Q7 案 C)
- ❌ Per-error-type helper functions (Q8 案 B)
- ❌ Go error as JS object return (Q8 案 C)
- ❌ JS class-based API (Q9 案 C)
- ❌ Runtime warning log for --out detection (Q10 案 B/C)
- ❌ internal/k6otelgentest package (Q11 案 C)
- ❌ Pre-built k6 binary for integration (Q12 案 B)
- ❌ Docker container for xk6 build (Q12 案 C)
- ❌ errors.go merged into config.go (Q13 案 B)

---

## Notes for the implementing agent

1. **U2 patch is Phase 1**: `journey.NewEngineWithSeed` MUST land before U5 main implementation. Otherwise NewModuleInstance can't be properly seeded per-VU.
2. **sobek API specifics**: k6 uses `github.com/grafana/sobek` (the modern fork of dop251/goja). Use `sobek.FunctionCall`, `sobek.Runtime.ExportTo`, `sobek.Runtime.ToValue`, `sobek.Runtime.NewTypeError`. Avoid goja-specific APIs.
3. **modulestest.NewRuntime**: the k6 SDK provides `go.k6.io/k6/js/modulestest` for test fixtures. Use `rt.SetupModuleSystem(map[string]any{"k6/x/otel-gen": New()})` and `rt.RunOnEventLoop(jsCode)`.
4. **Configure 2nd-call detection**: sync.Once is idempotent. Check `root.configured` BEFORE the sync.Once.Do call, return ConfigError if already true.
5. **Pipeline shutdown delegation**: NEVER call `pipeline.Shutdown()` from U5. Document the dependency on U6 Output.Stop().
6. **Stats field naming**: Go struct fields are PascalCase; sobek auto-lowercases the first letter. So `TracesExported` → `tracesExported` in JS. Verify with a quick test if needed.
7. **Conventional Commits**: After each Phase, propose a commit. Include `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
8. **Sandbox mode**: `--sandbox danger-full-access` per `scripts/run-codex.sh u5`.
9. **Capacity caveat**: U5 is medium complexity (~6 production files + ~9 test files + integration). Should complete in 1 Codex session. Natural split point if needed: after Phase 7 (Documentation).
