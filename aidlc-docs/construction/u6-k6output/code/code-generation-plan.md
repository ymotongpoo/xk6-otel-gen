# U6 (k6output) — Code Generation Plan

> **This file is the Single Source of Truth (SSOT) for the U6 implementation.**
>
> **Audience**: Codex CLI (`gpt-5.5 xhigh`) + Cursor Composer 2.5.
>
> **Recommended agent per Phase**:
> - **Phase 0-13 → Codex** via `scripts/run-codex.sh u6`. Phase 1 patches U4 (`exporter.Pipeline.MetricExporter`) before U6 main implementation.
> - **Phase 9 (U7 generator addition) MAY be split off to Cursor batch**.
>
> **Execution model**: top-to-bottom, mark `[ ]` → `[x]` immediately upon completion.
>
> **Source artifacts**:
> - FD: `aidlc-docs/construction/u6-k6output/functional-design/{business-logic-model,business-rules,domain-entities}.md`
> - NFR-R: `aidlc-docs/construction/u6-k6output/nfr-requirements/{nfr-requirements,tech-stack-decisions}.md`
> - NFR-D: `aidlc-docs/construction/u6-k6output/nfr-design/{nfr-design-patterns,logical-components}.md` ← **most prescriptive**
> - Application Design: `aidlc-docs/inception/application-design/component-methods.md` §C6
> - U4 completed exporter package: `exporter/` (Pipeline, GetShared) — adding MetricExporter() in Phase 1
> - U5 completed k6otelgen package: `k6otelgen/` (integration test harness pattern)
> - U7 generator existing style: `testutil/generators/*.go`
> - PBT rules: `.aidlc-rule-details/extensions/testing/property-based/property-based-testing.md`
> - Agent contract: `AGENTS.md`

---

## Unit Context

- **Unit ID**: U6
- **Purpose**: Implement `k6output/` — k6 Output module registered as `--out otel-gen=...`. Dual function: (a) Pipeline shutdown trigger, (b) k6 native metric → OTLP conversion with runner Resource (`service.name="xk6-otel-gen-runner"`).
- **Workspace root**: `/home/ymotongpoo/repos/xk6-otel-gen/`
- **Go module path**: `github.com/ymotongpoo/xk6-otel-gen`
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → U2 ✓ → U5 ✓ → **U6 (this)** → U8
- **PBT requirements satisfied by this unit**:
  - **PBT-01** (Property Identification) — 3 properties (TP-U6-1..3)
  - **PBT-03** (Invariants) — TP-U6-1, TP-U6-2, TP-U6-3
- **NFR DoD** (from `nfr-requirements.md` §4):
  - `go build ./k6output/...` succeeds
  - `go vet ./k6output/...` clean
  - `go test -race -count=1 ./k6output/...` passes
  - `go test -cover ./k6output/...` ≥ 80%
  - PBT TP-U6-1..3 all pass
  - `BenchmarkAddMetricSamples` < 1 µs / sample (strict)
  - `BenchmarkFlushLoop` < 5 µs / sample (strict)
  - All Go exported identifiers have GoDoc
  - 1 Example function (`ExampleNew`)
  - `--out otel-gen=<args>` reference table in doc.go
  - U7 generators (`ValidK6Sample/AnyK6Sample`, `ValidOutputParams/AnyOutputParams`) added
  - `golangci-lint run ./k6output/...` passes
  - Integration test (`-tags=integration ./k6output/integration/...`) passes with xk6-built k6 binary + Docker Collector
- **Dependencies used by this unit**: existing OTel SDK + k6 SDK (already in go.mod via U4/U5)
- **U4 coordination required (Phase 1)**: add `exporter.Pipeline.MetricExporter() sdkmetric.Exporter` accessor (minor SemVer bump, backward-compatible new method).

---

## Phase 0 — Environment Setup
**Recommended agent**: Codex CLI.

### Step 0.1 — Verify deps

- [ ] `head -3 go.mod` shows `go 1.25`.
- [ ] `grep -E "go.k6.io/k6|otel/sdk/metric" go.mod` confirms transitive deps from U4/U5 are present.
- [ ] No new external deps required (k6 SDK + OTel SDK already pulled).

### Step 0.2 — Create k6output/ skeleton

- [ ] Create `k6output/` directory.
- [ ] Create empty `k6output/doc.go` placeholder.
- [ ] Verify: `go build ./k6output/...` succeeds.

### Phase 0 commit

- [ ] `git add k6output/doc.go && git commit -m "build(k6output): scaffold package for U6"`

---

## Phase 1 — U4 Coordination: Add `Pipeline.MetricExporter`
**Recommended agent**: Codex CLI.

> Patch U4 to expose internal OTLP metric exporter for U6 (which builds a separate MeterProvider with runner Resource). Minor SemVer bump (backward-compatible new method).

### Step 1.1 — Modify `exporter/pipeline.go`

- [ ] Add method:
  ```go
  // MetricExporter returns the underlying OTLP metric exporter used by this
  // Pipeline. Intended for k6output to construct an additional MeterProvider
  // with a different Resource (e.g., xk6-otel-gen-runner) while sharing the
  // same OTLP connection.
  //
  // The returned exporter is owned by the Pipeline; callers must NOT call
  // Shutdown on it directly — use Pipeline.Shutdown for unified lifecycle.
  func (p *Pipeline) MetricExporter() sdkmetric.Exporter { return p.metricExp }
  ```
- [ ] Verify the internal field is named appropriately (likely `metricExp` based on U4 NFR-D, or adjust based on actual U4 code).
- [ ] Full GoDoc on the new method.

### Step 1.2 — Add `exporter/pipeline_test.go::TestPipeline_MetricExporter`

- [ ] `TestPipeline_MetricExporter_NotNil` — construct Pipeline with valid config, MetricExporter() returns non-nil.
- [ ] `TestPipeline_MetricExporter_SameAsInternal` — verify identity with the exporter used by Pipeline's own MeterProvider.

### Step 1.3 — Update U4 docs

- [ ] In `aidlc-docs/construction/u4-exporter/functional-design/domain-entities.md` §4 / §2, add `MetricExporter` to the public API table.
- [ ] In `aidlc-docs/construction/u4-exporter/nfr-design/logical-components.md` §LC-5 (Pipeline), document the new method and its purpose for k6output integration.

### Phase 1 commit

- [ ] `git add exporter/pipeline.go exporter/pipeline_test.go aidlc-docs/construction/u4-exporter/ && git commit -m "feat(exporter): add Pipeline.MetricExporter accessor for k6output integration"`

---

## Phase 2 — Errors (LC-4)
**Recommended agent**: Codex CLI.

### Step 2.1 — Create `k6output/errors.go`

- [ ] Define `ConfigError struct { Kind, Field, Value string; Inner error }` with `Error()` + `Unwrap()` per NFR-D §3.1.
- [ ] Kind enum: `"invalid_args"`, `"invalid_protocol"`, `"type_mismatch"`, `"invalid_url"`.
- [ ] All exported identifiers have GoDoc.

### Step 2.2 — Unit test `k6output/errors_test.go`

- [ ] `TestConfigError_Error` formatting with and without Inner / Field / Value.
- [ ] `TestConfigError_Unwrap`.
- [ ] All tests call `t.Parallel()`.

### Phase 2 commit

- [ ] `git add k6output/errors.go k6output/errors_test.go && git commit -m "feat(k6output): add ConfigError type"`

---

## Phase 3 — Args Parser (LC-2)
**Recommended agent**: Codex CLI.

### Step 3.1 — Create `k6output/params.go`

- [ ] Define `Params struct` per NFR-D §LC-2 (OTLP fields + QueueSize + FlushInterval + ScriptPath).
- [ ] Implement `defaultParams() Params`:
  - Protocol=ProtocolGRPC, Endpoint=localhost:4317, Timeout=10s, BatchSize=512, BatchTimeout=1s, MaxQueueSize=2048
  - QueueSize=100, FlushInterval=1*time.Second
- [ ] Implement `parseOutArgs(s string) (Params, error)` per NFR-D §4.1:
  - strings.Split by `,` → each token by `=` → applyKV switch
  - validate queueSize range [10, 10000]
  - unknown keys ignored (forward-compat)
- [ ] Implement `applyKV(p *Params, key, val string) error` for 10 keys.
- [ ] Implement `parseHeaders(s string) (map[string]string, error)` for `key1:val1;key2:val2` format.
- [ ] Internal helpers, brief comments.

### Step 3.2 — Unit test `k6output/params_test.go`

- [ ] `TestDefaultParams_Values`.
- [ ] `TestParseOutArgs_Empty` — empty string → defaults.
- [ ] `TestParseOutArgs_AllKeys_HappyPath` — table-driven for each of 10 keys.
- [ ] `TestParseOutArgs_InvalidProtocol`.
- [ ] `TestParseOutArgs_TypeMismatch_*` (insecure / timeout / batchSize numeric parse failures).
- [ ] `TestParseOutArgs_QueueSizeOutOfRange` (< 10 and > 10000).
- [ ] `TestParseOutArgs_UnknownKey_Ignored`.
- [ ] `TestParseOutArgs_MalformedToken` — no `=` in token.
- [ ] `TestParseHeaders_*` (basic / multiple / malformed).
- [ ] All tests call `t.Parallel()`.

### Phase 3 commit

- [ ] `git add k6output/params.go k6output/params_test.go && git commit -m "feat(k6output): add --out args parser with queueSize range validation"`

---

## Phase 4 — Sample Converter (LC-3)
**Recommended agent**: Codex CLI.

### Step 4.1 — Create `k6output/convert.go`

- [ ] Define `k6MetricSpec struct { k6Name, otelName, unit string; instType instrumentType }`.
- [ ] Define `instrumentType int` with constants `tInstCounter`, `tInstHistogram`, `tInstGauge`.
- [ ] Define `knownK6Metrics []k6MetricSpec` for the 11 standard k6 metrics per NFR-D §1.2.
- [ ] Define `instrumentMap struct { counters, histograms, gauges sync.Map }`.
- [ ] Define `tagSetCache struct { sets sync.Map }` per NFR-D §1.4.
- [ ] Implement `hashTags(tags map[string]string) string` (sorted keys joined per NFR-D §1.4).
- [ ] Implement `(*tagSetCache).get(tags) attribute.Set` with cache + `attribute.NewSet`.
- [ ] Implement `k6UnitHint(name string) string` for known unit hints.
- [ ] Implement `dotted(s string) string` for `_` → `.` substitution.
- [ ] Internal helpers, brief comments.

### Step 4.2 — Unit test `k6output/convert_test.go`

- [ ] `TestKnownK6Metrics_TableComplete` — assert 11 entries with correct OTel names.
- [ ] `TestHashTags_*` — sorted invariance, empty map, single tag.
- [ ] `TestTagSetCache_Get_CacheHit` — second call with same tags returns same Set (deep equality + pointer? per Set's value semantics).
- [ ] `TestK6UnitHint_*` (table-driven).
- [ ] `TestDotted_*` — `http_req_duration` → `http.req.duration`.
- [ ] All tests call `t.Parallel()`.

### Phase 4 commit

- [ ] `git add k6output/convert.go k6output/convert_test.go && git commit -m "feat(k6output): add k6 sample converter with instrument cache and tag set hashing"`

---

## Phase 5 — Output Lifecycle (LC-1)
**Recommended agent**: Codex CLI.

### Step 5.1 — Create `k6output/output.go`

- [ ] Define `Output struct` with all fields per NFR-D §1.1.
- [ ] Implement `init()` calling `output.RegisterExtension("otel-gen", New)`.
- [ ] Implement `New(params output.Params) (output.Output, error)`:
  - parseOutArgs from `params.ConfigArgument`
  - build runnerResource via `buildRunnerResource(params)`
  - return `*Output` (no Pipeline build here)
- [ ] Implement `buildRunnerResource(params output.Params) *resource.Resource` per NFR-D §4.2.
- [ ] Implement `(*Output).Description() string` returning a human-readable description with endpoint.
- [ ] Implement `(*Output).Start() error` per NFR-D §2.1:
  - sync.Once Do:
    - `exporter.GetShared(factory)` to acquire pipeline
    - build runner MeterProvider with `pipeline.MetricExporter()` wrapped in `NewPeriodicReader`
    - build known instruments
    - start flush goroutine
- [ ] Implement `(*Output).AddMetricSamples(samples []metrics.SampleContainer)` per NFR-D §2.3 (non-blocking + drop-oldest).
- [ ] Implement `(*Output).tryPush(s) bool` per NFR-D §2.3.
- [ ] Implement `(*Output).Stop() error` per NFR-D §2.1 (always nil).
- [ ] Implement `(*Output).flushLoop()` per NFR-D §2.2.
- [ ] Implement `(*Output).buildKnownInstruments() error` per NFR-D §1.2.
- [ ] Implement `(*Output).lookupOrBuildInstrument(name, k6Type, unit) any` per NFR-D §1.3.
- [ ] Implement `(*Output).emitContainer(container) ` and `(*Output).emitSample(sample)` per NFR-D §LC-3 sketch.
- [ ] Pluggable logger via `o.logger` field for warn messages (defaults to `log.Printf`).
- [ ] All exported identifiers have GoDoc.

### Step 5.2 — Create `k6output/helpers_test.go`

- [ ] `newTestParams(t *testing.T, args string) output.Params`.
- [ ] `newTestOutput(t *testing.T, args string) *Output` — uses `exporter.ResetShared()` before each call to ensure fresh Pipeline.
- [ ] `recordingLogger() (func(string, ...any), *[]string)` — captures warn log messages for assertions.
- [ ] All helpers use `t.Helper()`.

### Step 5.3 — Unit test `k6output/output_test.go`

- [ ] `TestNew_HappyPath` — endpoint specified, returns *Output without error.
- [ ] `TestNew_InvalidArgs_ReturnsError` — invalid protocol → *ConfigError.
- [ ] `TestDescription_ContainsEndpoint`.
- [ ] `TestStart_Idempotent` — call twice, second call no-op.
- [ ] `TestStart_PipelineFailure_ReturnsError` — wrong endpoint that triggers GetShared factory failure.
- [ ] `TestAddMetricSamples_BeforeStart_NoOp` — no panic.
- [ ] `TestAddMetricSamples_AfterStop_NoOp` — no panic.
- [ ] `TestStop_Idempotent` — call twice, both return nil.
- [ ] `TestStop_AlwaysReturnsNil` — even if Pipeline.Shutdown errors, return nil.
- [ ] All tests call `t.Parallel()` carefully (some require exporter.ResetShared serialization).

### Phase 5 commit

- [ ] `git add k6output/output.go k6output/output_test.go k6output/helpers_test.go && git commit -m "feat(k6output): add Output lifecycle with sync.Once-guarded Start/Stop and flush goroutine"`

---

## Phase 6 — Documentation (LC-0) + doc_test.go
**Recommended agent**: Codex CLI.

### Step 6.1 — Replace `k6output/doc.go` placeholder

- [ ] Full package documentation per NFR-D §5.1:
  - JS shell example with `k6 run --out otel-gen=...`
  - Dual-function explanation
  - Supported --out args reference table (10 keys including queueSize)
  - Configuration priority
  - High-cardinality risk note

### Step 6.2 — Create `k6output/doc_test.go`

- [ ] `ExampleNew` — minimal example showing `New(params)` call.

### Phase 6 commit

- [ ] `git add k6output/doc.go k6output/doc_test.go && git commit -m "docs(k6output): add package documentation and Example function"`

---

## Phase 7 — PBT (TP-U6-1..3)
**Recommended agent**: Codex CLI.

### Step 7.1 — Create `k6output/pbt_test.go`

- [ ] `TestOutput_Robustness_AllStates` (TP-U6-1, example-based table per NFR-D §6.3):
  - covers: Start_AddSamples_Stop / AddSamples_BeforeStart / Stop_BeforeStart / Stop_AfterStop_NoOp / AddSamples_AfterStop
  - assert no panic
- [ ] `TestCounter_Monotonic_Property` (TP-U6-2, rapid):
  - draw N samples with positive Counter values
  - emit via Output, read ManualReader → assert sum matches
- [ ] `TestTag_Attribute_RoundTrip_Property` (TP-U6-3, rapid):
  - draw sample with random Tags
  - emit + collect → assert `k6.tag.<key>` attributes present with correct values
- [ ] All tests call `t.Parallel()` carefully.

### Phase 7 commit

- [ ] `git add k6output/pbt_test.go && git commit -m "test(k6output): add PBT for TP-U6-1..3"`

---

## Phase 8 — Benchmark
**Recommended agent**: Codex CLI.

### Step 8.1 — Create `k6output/bench_test.go`

- [ ] `BenchmarkAddMetricSamples` — measure queue push per sample (NFR-U6-3 strict <1µs).
- [ ] `BenchmarkFlushLoop` — measure flush emit per sample (NFR-U6-3 strict <5µs).
- [ ] `BenchmarkTagSetCache_Hit` — assert near-zero allocations on cache hit.
- [ ] `BenchmarkTagSetCache_Miss` — first lookup allocations.
- [ ] `BenchmarkInstrumentLookup` — sync.Map.Load performance.
- [ ] All use `b.ReportAllocs()`.

### Phase 8 commit

- [ ] `git add k6output/bench_test.go && git commit -m "test(k6output): add benchmarks for per-sample push/flush"`

---

## Phase 9 — U7 Generator Additions
**Recommended agent**: Codex CLI (or Cursor batch).

### Step 9.1 — Add `testutil/generators/k6output_inputs.go`

- [ ] Implement `ValidK6Sample(opts ...SampleOption) *rapid.Generator[metrics.Sample]`:
  - Sample with valid Metric (Counter / Trend / Gauge / Rate), positive Value, Tags map (0-5 entries with semconv-friendly keys)
- [ ] Implement `AnyK6Sample(opts ...SampleOption) *rapid.Generator[metrics.Sample]`:
  - allows negative values, nil Metric, large Tags
- [ ] Implement `ValidOutputParams(opts ...ParamsOption) *rapid.Generator[k6output.Params]`:
  - valid endpoint, protocol, all U6 Params fields
- [ ] Implement `AnyOutputParams(opts ...ParamsOption) *rapid.Generator[k6output.Params]`:
  - invalid combinations
- [ ] Generators follow existing U7 style.

### Step 9.2 — Add `testutil/generators/k6output_inputs_test.go`

- [ ] Property tests that valid generators produce only invariant-respecting values.

### Phase 9 commit

- [ ] `git add testutil/generators/k6output_inputs.go testutil/generators/k6output_inputs_test.go && git commit -m "feat(testutil): add k6 sample + output params generators for U6 PBT"`

---

## Phase 10 — Integration Test Harness
**Recommended agent**: Codex CLI.

### Step 10.1 — Create `k6output/integration/testdata/topology.yaml`

- [ ] Minimal valid topology (reuse U5's pattern).

### Step 10.2 — Create `k6output/integration/testdata/script.js`

- [ ] k6 script that uses `otelgen.load()` + `runJourney()` plus a few HTTP-like operations to generate k6 native samples (http_req_duration etc.).

### Step 10.3 — Create `k6output/integration/testdata/collector-config.yaml` + `docker-compose.yaml`

- [ ] OTel Collector with file_exporter, docker-compose pinning collector-contrib image (reuse U5 tag).

### Step 10.4 — Create `k6output/integration/helpers.go`

- [ ] `requireDocker(t)`, `requireXK6(t)`.
- [ ] `buildK6Binary(t, modulePath, outputDir) string`.
- [ ] `startCollector(t, configDir) (endpoint string, cleanup func())`.
- [ ] `runK6Script(t, k6Bin, scriptPath, ...args) output`.
- [ ] `readCollectorMetrics(t, path) metricsContent` — file_exporter JSON parser.

### Step 10.5 — Create `k6output/integration/integration_test.go`

- [ ] `//go:build integration`.
- [ ] `TestIntegration_EndToEnd`:
  - requireDocker + requireXK6
  - buildK6Binary with this extension
  - startCollector
  - runK6Script with `--out otel-gen=endpoint=<collector>`
  - assert exit code 0
  - readCollectorMetrics + assert `k6.*` metrics received (e.g., `k6.iterations.total`, `k6.vus`)
  - assert `service.name=xk6-otel-gen-runner` in Resource

### Step 10.6 — Create `k6output/integration/README.md`

- [ ] Document Docker + xk6 requirement.

### Phase 10 commit

- [ ] `git add k6output/integration/ && git commit -m "test(k6output): add integration test harness with xk6 build and Docker Collector"`

---

## Phase 11 — U5 Integration Test Un-guard
**Recommended agent**: Codex CLI.

> U5 integration test currently guards for U6's absence (per U5 code-generation-summary.md). Now that U6 has landed, un-guard to enable U5+U6 end-to-end testing.

### Step 11.1 — Modify `k6otelgen/integration/integration_test.go`

- [ ] Remove the U6 absence guard.
- [ ] Add `--out otel-gen=...` to the k6 script invocation.
- [ ] Ensure U5 integration test now flows through U6 for shutdown trigger.

### Step 11.2 — Verify

- [ ] `go test -tags=integration ./k6otelgen/integration/...` passes (with Docker + xk6).
- [ ] `go test -tags=integration ./k6output/integration/...` passes.

### Phase 11 commit

- [ ] `git add k6otelgen/integration/ && git commit -m "test(k6otelgen): un-guard U6 dependency in integration tests"`

---

## Phase 12 — Final Wrap & DoD Verification
**Recommended agent**: Codex CLI.

### Step 12.1 — Run full suite

- [ ] `go build ./...` succeeds.
- [ ] `go vet ./k6output/...` clean.
- [ ] `go test -race -count=1 ./...` passes.
- [ ] `go test -cover ./k6output/...` ≥ 80%.
- [ ] `go test -bench=. -benchmem ./k6output/...` shows BenchmarkAddMetricSamples < 1µs, BenchmarkFlushLoop < 5µs.
- [ ] `golangci-lint run ./k6output/...` passes.
- [ ] `go test -tags=integration ./k6output/integration/...` passes (with Docker + xk6).
- [ ] `go test -tags=integration ./k6otelgen/integration/...` passes (un-guarded U6 dependency).

### Step 12.2 — Create `aidlc-docs/construction/u6-k6output/code/code-generation-summary.md`

- [ ] File list with line counts.
- [ ] Verification results.
- [ ] Deviations from plan.
- [ ] Recent commits.

### Step 12.3 — Mark all plan checkboxes [x]

- [ ] Walk back; verify all `[ ]` are `[x]`.

### Step 12.4 — Update `aidlc-docs/aidlc-state.md`

- [ ] Mark U6 complete. Set Current Unit to U8 (Samples & Distribution).

### Phase 12 commit

- [ ] `git add aidlc-docs/ && git commit -m "chore(u6-k6output): finalize code-generation-summary and checkbox state"`

---

## Anti-patterns to AVOID during implementation

(Per NFR-D `nfr-design-patterns.md` §9, 22 items abbreviated)

- ❌ Flat Output struct (use 3-group layout)
- ❌ URL query-style or regexp args parser (use strings.Split + applyKV)
- ❌ Full eager or full lazy instrument construction (use eager-known + lazy-unknown hybrid)
- ❌ `map[string]any` + RWMutex for instruments (use per-type sync.Map)
- ❌ `time.Timer + atomic.Bool` for flush cancel (use context.WithCancel + done channel)
- ❌ Simple drop-newest queue full (use drop-oldest via 2-stage select)
- ❌ `MetricReader()` or `BuildMeterProvider(res)` accessor on U4 (use `MetricExporter()` returning Exporter)
- ❌ runner Resource in shared exporter package (keep in k6output/output.go private)
- ❌ Per-call tag set build without cache (use tagSetCache with hash key)
- ❌ FNV/xxHash tag hash (use sorted keys joined string)
- ❌ rapid.Run stateful PBT for TP-U6-1 (use example-based table)
- ❌ k6 SDK testutils dependency (use ad-hoc helpers in helpers_test.go)
- ❌ Split convert.go (keep as single file with both metric and attr conversion)
- ❌ errors.go merged into output.go (keep separate for grep-ability)
- ❌ Direct OTLP exporters/* import (use exporter package abstraction)
- ❌ Stop returning error (always return nil to protect k6 lifecycle)
- ❌ Block on queue full (use non-blocking with drop-oldest)
- ❌ Self-metric expose (drops counter is internal-only)
- ❌ Cardinality limit inside U6 (delegate to Collector processors)
- ❌ semconv namespace rewrite for k6 metrics (use `k6.*` to preserve runner identity)
- ❌ synth Resource reuse (build dedicated runner Resource)
- ❌ synth MeterProvider reuse (build separate MeterProvider with runner Resource)

---

## Notes for the implementing agent

1. **U4 patch is Phase 1**: `exporter.Pipeline.MetricExporter()` MUST land before U6 main implementation. The internal field name might be `metricExp` or similar — verify against actual U4 implementation.
2. **k6 SDK API**: `go.k6.io/k6/output` provides `output.Output` interface, `output.Params`, `output.RegisterExtension`. `go.k6.io/k6/metrics` provides `metrics.Sample`, `metrics.SampleContainer`, `metrics.MetricType`. Inspect actual signatures during implementation.
3. **k6 Sample Tags**: actual type may be `*metrics.SampleTags` (immutable) instead of `map[string]string`. The conversion logic should handle whichever the SDK exposes (use `.CloneTags()` or `.Get(key)` accordingly).
4. **OTel Gauge availability**: `metric.Float64Gauge` is stable in OTel Go SDK v1.31+. If using an older SDK, fall back to `Int64UpDownCounter` for `vus`-style metrics.
5. **k6 metric naming reality**: actual k6 metric names may include variations (e.g., `http_req_duration` vs `http_reqs`). Verify against `go.k6.io/k6/metrics/metrics.go` standard metrics list during Phase 4 implementation.
6. **exporter.ResetShared**: U4 provides this test helper. Use it in helpers_test.go to ensure each test starts with a fresh GetShared state.
7. **Build version injection**: `buildVersion = ""` at package level, set via `-ldflags="-X github.com/ymotongpoo/xk6-otel-gen/k6output.buildVersion=v0.1.0"` during xk6 build. If empty at runtime, omit `service.version` attribute.
8. **Conventional Commits**: After each Phase, propose a commit. Include `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
9. **Sandbox mode**: `--sandbox danger-full-access` per `scripts/run-codex.sh u6`.
10. **Capacity caveat**: U6 is medium complexity (~5 production files + ~8 test files + integration + U4 patch). Should complete in 1 Codex session. Natural break point if needed: after Phase 6 (Documentation).
