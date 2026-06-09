# U4 (exporter) — Code Generation Plan

> **This file is the Single Source of Truth (SSOT) for the U4 implementation.**
>
> **Audience**: Codex CLI (`gpt-5.5 xhigh`) for autonomous batch execution + Cursor Composer 2.5 for any single-shot follow-ups via `agent -p "<prompt>"` (Cursor Agent CLI binary is `agent`). Read `AGENTS.md` for role boundaries and the agent-selection guideline before starting.
>
> **Recommended agent per Phase** (see AGENTS.md §2):
> - **Phase 0-13 → Codex** (multi-file new construction, deep type-system reasoning around OTel SDK, integration test harness wiring). Run via `scripts/run-codex.sh u4`.
> - **Phase 12 (U7 generator addition) MAY be split off to Cursor batch** if Codex prefers focus; the additive work (2 generator funcs) follows existing U7 style.
>
> **Execution model**: Work through the checkboxes top-to-bottom. Mark each `[ ]` → `[x]` immediately upon completion of that step. Do not re-order or skip without updating this plan first.
>
> **Source artifacts to reference while implementing**:
> - FD: `aidlc-docs/construction/u4-exporter/functional-design/{business-logic-model,business-rules,domain-entities}.md`
> - NFR-R: `aidlc-docs/construction/u4-exporter/nfr-requirements/{nfr-requirements,tech-stack-decisions}.md`
> - NFR-D: `aidlc-docs/construction/u4-exporter/nfr-design/{nfr-design-patterns,logical-components}.md`
> - Application Design (types): `aidlc-docs/inception/application-design/component-methods.md`
> - U7 generator existing style: `testutil/generators/*.go`
> - U1's completed topology package as a reference for code style: `topology/`
> - PBT rules: `.aidlc-rule-details/extensions/testing/property-based/property-based-testing.md`
> - Agent contract: `AGENTS.md`
> - Shared memory: `.agent-memory/MEMORY.md`

---

## Unit Context

- **Unit ID**: U4
- **Purpose**: Implement the OTLP exporter pipeline (`exporter/`) that shares one TracerProvider / MeterProvider / LoggerProvider with one endpoint + resource across all 3 signals, including a shared singleton holder for k6 VU lifecycles.
- **Workspace root**: `/home/ymotongpoo/repos/xk6-otel-gen/`
- **Go module path**: `github.com/ymotongpoo/xk6-otel-gen`
- **Construction order position**: U7 ✓ → U1 ✓ → **U4 (this)** → U3 → U2 → U5 → U6 → U8
- **PBT requirements satisfied by this unit**:
  - **PBT-01** (Property Identification) — 4 properties documented (TP-U4-1..4)
  - **PBT-02** (Round-trip) — TP-U4-3 (OTLP protobuf round-trip)
  - **PBT-03** (Invariants) — TP-U4-1 (merge override), TP-U4-4 (Stats monotonicity)
  - **PBT-04** (Idempotency) — TP-U4-2 (`c.MergeWith(c) == c`)
- **NFR DoD** (from `nfr-requirements.md`):
  - `go build ./...` succeeds
  - `go test -race -count=1 ./exporter/...` passes
  - `go test -cover ./exporter/...` shows ≥ 80% coverage (unit-only, integration tests excluded)
  - `BenchmarkNew` runs at ≤ 100 ms / iteration
  - All U4 test functions call `t.Parallel()` (where shared state allows)
  - No package-level mutable state besides the documented `sharedOnce` / `sharedPipeline` / `sharedInitErr`
  - All public identifiers have GoDoc
  - 3 Example functions present (`ExampleNew`, `ExampleConfig_MergeWith`, `ExampleGetShared`)
  - U7's `ValidConfig` / `AnyConfig` generators added in `testutil/generators/` (Q12=A request)
  - `go vet ./exporter/...` clean
- **Dependencies added by this unit** (OTel Go SDK, latest stable as of 2026-06):
  - `go.opentelemetry.io/otel`
  - `go.opentelemetry.io/otel/sdk`
  - `go.opentelemetry.io/otel/sdk/metric`
  - `go.opentelemetry.io/otel/sdk/log`
  - `go.opentelemetry.io/otel/trace`
  - `go.opentelemetry.io/otel/metric`
  - `go.opentelemetry.io/otel/log`
  - `go.opentelemetry.io/otel/exporters/otlp/otlptrace`
  - `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`
  - `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`
  - `go.opentelemetry.io/otel/exporters/otlp/otlpmetricgrpc`
  - `go.opentelemetry.io/otel/exporters/otlp/otlpmetrichttp`
  - `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc`
  - `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp`
  - `go.opentelemetry.io/proto/otlp` (test-only, TP-U4-3)
- **Explicitly excluded**: `go.opentelemetry.io/otel/propagation` (in-process telemetry synthesis), `go.opentelemetry.io/otel/baggage`, `stdout` exporters, `semconv` (use raw `attribute.String` keys)

---

## Phase 0 — Environment Setup
**Recommended agent**: Codex CLI.

> Add OTel SDK dependencies, ensure Go toolchain is correct.

### Step 0.1 — Verify Go version

- [x] `head -3 go.mod` shows `go 1.25` (set during U1). If not, run `go mod edit -go=1.25` and `go mod tidy`.

### Step 0.2 — Add OTel SDK dependencies

- [x] Run `go get go.opentelemetry.io/otel@latest`.
- [x] Run `go get go.opentelemetry.io/otel/sdk@latest`.
- [x] Run `go get go.opentelemetry.io/otel/sdk/metric@latest`.
- [x] Run `go get go.opentelemetry.io/otel/sdk/log@latest`.
- [x] Run `go get go.opentelemetry.io/otel/exporters/otlp/otlptrace@latest`.
- [x] Run `go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@latest`.
- [x] Run `go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@latest`.
- [x] Run `go get go.opentelemetry.io/otel/exporters/otlp/otlpmetricgrpc@latest`.
- [x] Run `go get go.opentelemetry.io/otel/exporters/otlp/otlpmetrichttp@latest`.
- [x] Run `go get go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc@latest`.
- [x] Run `go get go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp@latest`.
- [x] Run `go get go.opentelemetry.io/proto/otlp@latest` (test-only; OK if marked direct since `_test.go` imports it — do not import from any non-test file).
- [x] Run `go mod tidy`.

### Step 0.3 — Create exporter/ package skeleton

- [x] Create `exporter/` directory.
- [x] Create empty `exporter/doc.go` containing only the placeholder `package exporter` line and a TODO comment. (Real doc comment is written in Phase 8.)
- [x] Verify: `go build ./exporter/...` succeeds (empty package compiles).

### Phase 0 commit

- [x] `git add go.mod go.sum exporter/doc.go && git commit -m "build(exporter): add OTel SDK dependencies for U4 ..."`

---

## Phase 1 — Errors (LC-7)
**Recommended agent**: Codex CLI.

> The base layer everything else depends on.

### Step 1.1 — Create `exporter/errors.go`

- [x] Define `PipelineError` with `Stage string`, `Inner error`, `Error()`, `Unwrap()`.
- [x] Define `ConfigError` with `Field string`, `Value any`, `Message string`, `Error()`. (No `Unwrap` needed — leaf error.)
- [x] Define `SharedError` with `Reason string`, `Inner error`, `Error()`, `Unwrap()`.
- [x] All three have GoDoc comments referencing their use sites (`New`, `Config.Validate`, `GetShared/SetShared`).

### Step 1.2 — Unit test `exporter/errors_test.go`

- [x] `TestPipelineError_Error` checks formatting.
- [x] `TestPipelineError_Unwrap` checks `errors.Is` integration.
- [x] `TestConfigError_Error` checks formatting with various Value types.
- [x] `TestSharedError_Error` checks both nil and non-nil Inner.
- [x] All tests call `t.Parallel()`.

### Phase 1 commit

- [x] `git add exporter/errors.go exporter/errors_test.go && git commit -m "feat(exporter): add typed error hierarchy"`

---

## Phase 2 — Config (LC-1)
**Recommended agent**: Codex CLI.

> Config struct + validation + merge + env loader. The biggest single file.

### Step 2.1 — Create `exporter/config.go`

- [x] Define `Protocol int` with constants `ProtocolGRPC`, `ProtocolHTTP`.
- [x] Implement `(Protocol).String() string` returning `"grpc"` / `"http"`.
- [x] Define `Config` struct exactly as in FD `domain-entities.md` §1.2.
- [x] Define unexported `defaultConfig` var with built-in defaults from `business-rules.md` §10.
- [x] Implement unexported `(Config).fillDefaults() Config`.
- [x] Implement `(Config).Validate() error` returning `errors.Join` of `*ConfigError`s per `business-rules.md` §1.1.
- [x] Implement `(Config).MergeWith(override Config) Config` per `business-rules.md` §2.
- [x] Implement `ConfigFromEnv() Config` per `business-rules.md` §2.4 (signal-specific priority, OTEL_EXPORTER_OTLP_HEADERS comma-split, OTEL_EXPORTER_OTLP_TIMEOUT in ms, OTEL_EXPORTER_OTLP_PROTOCOL parsing `"grpc"` / `"http/protobuf"`).
- [x] All identifiers have GoDoc.

### Step 2.2 — Unit test `exporter/config_test.go`

- [x] `TestProtocol_String` (table-driven).
- [x] `TestConfig_Validate_OK` (well-formed config).
- [x] `TestConfig_Validate_Errors` (table-driven: each invalid field).
- [x] `TestConfig_MergeWith_Examples` (handful of example-based cases including header replacement semantics).
- [x] `TestConfig_fillDefaults_Examples` (zero values get filled).
- [x] `TestConfigFromEnv_*` (use `t.Setenv` for each env var).
- [x] **PBT TP-U4-1**: `TestMergeWith_OverrideWins_Property` using `rapid.Check` + `generators.ValidConfig()` (will be added in Phase 12 — for now, mark this test with a `t.Skip("waits for ValidConfig generator from Phase 12")` and a TODO; un-skip in Phase 12).
- [x] **PBT TP-U4-2**: `TestMergeWith_Idempotent_Property` similar skip-until-Phase-12.
- [x] All tests call `t.Parallel()`.

### Phase 2 commit

- [x] `git add exporter/config.go exporter/config_test.go && git commit -m "feat(exporter): add Config with validate, merge, and env loader"`

---

## Phase 3 — Resource Builder (LC-2)
**Recommended agent**: Codex CLI.

### Step 3.1 — Create `exporter/resource.go`

- [x] Implement `buildResource(ctx context.Context, cfg Config) (*sdkresource.Resource, error)` per NFR-D `logical-components.md` LC-2.
- [x] Auto-detect: `WithFromEnv`, `WithHost`, `WithProcess`, `WithOS`.
- [x] If `cfg.ResourceOverrides` non-empty: build a schemaless Resource and `sdkresource.Merge(detected, override)`.
- [x] Function and parameter docs (internal helper, brief comment only).

### Step 3.2 — Unit test `exporter/resource_test.go`

- [x] `TestBuildResource_AutoDetectOnly` (empty ResourceOverrides → expect Resource non-nil, host/OS attrs present).
- [x] `TestBuildResource_OverrideWins` (override `service.name` → expect merged value).
- [x] All tests call `t.Parallel()`.

### Phase 3 commit

- [x] `git add exporter/resource.go exporter/resource_test.go && git commit -m "feat(exporter): add resource builder with override merge"`

---

## Phase 4 — Stats & Instrumentation (LC-4)
**Recommended agent**: Codex CLI.

### Step 4.1 — Create `exporter/stats.go`

- [x] Define `Stats` (public, 6 fields per FD revision).
- [x] Define unexported `pipelineStats` with 6 `atomic.Int64` fields.
- [x] Implement `(*pipelineStats).snapshot() Stats`.
- [x] Define `tracingExporter` (`inner sdktrace.SpanExporter`, `stats *pipelineStats`) implementing `sdktrace.SpanExporter`.
- [x] Define `metricExporter` (`inner sdkmetric.Exporter`, `stats *pipelineStats`) implementing `sdkmetric.Exporter` (all 5 methods: `Export`, `Aggregation`, `Temporality`, `ForceFlush`, `Shutdown`).
- [x] Define `loggingExporter` (`inner sdklog.Exporter`, `stats *pipelineStats`) implementing `sdklog.Exporter`.
- [x] Implement `countMetricDataPoints(rm *metricdata.ResourceMetrics) int` (iterates ScopeMetrics → Metrics → data point count by aggregation type: Gauge, Sum, Histogram, ExponentialHistogram, Summary).
- [x] All identifiers have GoDoc (only `Stats` is exported, others are internal so brief comments suffice).

### Step 4.2 — Unit test `exporter/stats_test.go`

- [x] `TestPipelineStats_Snapshot_AtomicLoad` — increment counters from multiple goroutines, verify Snapshot returns non-decreasing values per field.
- [x] `TestTracingExporter_Success` — fake inner returns nil → `tracesExported += len(spans)`.
- [x] `TestTracingExporter_Failure` — fake inner returns error → `tracesFailed += 1`, error propagated.
- [x] Same pattern for `MetricExporter` and `LoggingExporter`.
- [x] `TestCountMetricDataPoints_*` — table-driven across Gauge / Sum / Histogram.

### Phase 4 commit

- [x] `git add exporter/stats.go exporter/stats_test.go && git commit -m "feat(exporter): add atomic Stats and per-signal instrumented wrappers"`

---

## Phase 5 — Exporter Factory (LC-3)
**Recommended agent**: Codex CLI.

### Step 5.1 — Create `exporter/exporters.go`

- [x] Implement `buildTraceExporter(ctx, cfg, stats) (sdktrace.SpanExporter, error)` — protocol switch, returns `&tracingExporter{inner, stats}`.
- [x] Implement `buildMetricExporter(ctx, cfg, stats) (sdkmetric.Exporter, error)` — analogous.
- [x] Implement `buildLogExporter(ctx, cfg, stats) (sdklog.Exporter, error)` — analogous.
- [x] For each: pass Endpoint, Headers, Timeout, optional `WithInsecure()`, optional `WithCompressor("gzip")` / `WithCompression(GzipCompression)`.
- [x] Internal helpers, brief comments only.

### Step 5.2 — Unit test (covered later in Phase 6 / Phase 9)

No standalone test in this phase — exporter factory is exercised via `New` tests.

### Phase 5 commit

- [x] `git add exporter/exporters.go && git commit -m "feat(exporter): add OTLP exporter factory for traces, metrics, logs"`

---

## Phase 6 — Pipeline (LC-5)
**Recommended agent**: Codex CLI.

> The orchestration core.

### Step 6.1 — Create `exporter/pipeline.go`

- [ ] Define `Pipeline` struct with `tp`, `mp`, `lp`, `res`, `stats`, `shutdownOnce`, `shutdownErr`.
- [ ] Implement `New(cfg Config) (*Pipeline, error)` per NFR-D `nfr-design-patterns.md` §3.1:
  - `cfg.fillDefaults()` → `cfg.Validate()` → `buildResource` → `buildTraceExporter` → `buildMetricExporter` (with cleanup of trace on fail) → `buildLogExporter` (with cleanup of both on fail) → construct 3 Providers with `WithResource` + appropriate Reader/Processor wiring.
  - Trace: `sdktrace.WithBatcher(traceExp, MaxQueueSize, MaxExportBatchSize, BatchTimeout)`.
  - Metric: `sdkmetric.WithReader(NewPeriodicReader(metricExp, WithInterval(cfg.BatchTimeout)))`.
  - Log: `sdklog.WithProcessor(NewBatchProcessor(logExp, MaxQueueSize, ExportMaxBatchSize, ExportInterval))`.
  - On any failure: return `nil, &PipelineError{Stage, Inner}` with appropriate cleanup of previously-built exporters (best-effort, error discarded — Q10=A).
- [ ] Implement `(*Pipeline).TracerProvider() trace.TracerProvider`.
- [ ] Implement `(*Pipeline).MeterProvider() metric.MeterProvider`.
- [ ] Implement `(*Pipeline).LoggerProvider() log.LoggerProvider`.
- [ ] Implement `(*Pipeline).Shutdown(ctx) error` using `shutdownOnce` + `errors.Join` over 3 Provider Shutdowns.
- [ ] Implement `(*Pipeline).Stats() Stats` delegating to `pipelineStats.snapshot()`.
- [ ] All public identifiers have full GoDoc with usage hints.

### Step 6.2 — Unit test `exporter/pipeline_test.go` (with mockExporter scaffolding)

- [ ] Define `mockSpanExporter`, `mockMetricExporter`, `mockLogExporter` in this file (or in a `mocks_test.go` helper file in the `exporter_test` external test package).
- [ ] **Decision**: place tests in **internal `package exporter`** so we can directly call `buildTraceExporter` etc. with mocks via test-helper-only constructors. Alternatively, expose a small internal `newWithExporters(cfg, traceExp, metricExp, logExp) (*Pipeline, error)` helper that is **only callable from package-internal tests**.
- [ ] `TestNew_Success` — uses real (in-memory) defaults with `Insecure=true, Endpoint="localhost:4317"`. Since we cannot guarantee a listener, expect that `New` itself succeeds (OTLP exporters lazy-connect on first export). Shutdown immediately.
- [ ] `TestNew_ValidationError` — invalid Config → `*PipelineError{Stage:"validate"}`.
- [ ] `TestNew_ExporterFailure_CleansUp` — inject a mock that fails on second exporter build, verify first exporter's Shutdown was called.
- [ ] `TestPipeline_Shutdown_Idempotent` — call Shutdown twice, expect same error / nil and only one inner call (via mocks).
- [ ] `TestPipeline_Stats_DelegatesToSnapshot`.
- [ ] All tests call `t.Parallel()` where shared state allows.

### Phase 6 commit

- [ ] `git add exporter/pipeline.go exporter/pipeline_test.go && git commit -m "feat(exporter): add Pipeline orchestrator with all-or-nothing New and idempotent Shutdown"`

---

## Phase 7 — Shared Holder (LC-6)
**Recommended agent**: Codex CLI.

### Step 7.1 — Create `exporter/shared.go`

- [ ] Define unexported package-level vars `sharedMu sync.Mutex`, `sharedOnce sync.Once`, `sharedPipeline *Pipeline`, `sharedInitErr error`.
- [ ] Implement `GetShared(factory func() (*Pipeline, error)) (*Pipeline, error)` using `sharedOnce.Do`.
- [ ] Implement `SetShared(p *Pipeline) error` per NFR-D `logical-components.md` LC-6.
- [ ] Implement `ResetShared()` — replaces `sharedOnce` with a new instance and clears the cached values under `sharedMu`. Doc comment: "Intended for tests only."
- [ ] All exported identifiers have GoDoc; `ResetShared` doc explicitly says "tests only" and warns about goroutines holding the previous Pipeline.

### Step 7.2 — Unit test `exporter/shared_test.go`

- [ ] All tests call `exporter.ResetShared()` at the start (and `t.Cleanup(exporter.ResetShared)` for safety).
- [ ] `TestGetShared_CachesSuccess` — factory called once, second `GetShared` returns same pointer.
- [ ] `TestGetShared_CachesError` — factory returns error, second `GetShared` returns same error (factory NOT re-called).
- [ ] `TestSetShared_BeforeAnyGet` — `SetShared(p)` succeeds, then `GetShared(factory)` returns p without calling factory.
- [ ] `TestSetShared_AfterGet_Fails` — `*SharedError{Reason:"already_initialized"}`.
- [ ] `TestSetShared_Nil_Fails` — `*SharedError{Reason:"not_set"}`.
- [ ] **NOTE**: these tests **must NOT run in parallel** with other tests touching shared state. Use `t.Parallel()` only within subtests that don't touch the holder, or run shared_test.go single-threaded.

### Phase 7 commit

- [ ] `git add exporter/shared.go exporter/shared_test.go && git commit -m "feat(exporter): add shared Pipeline holder with sync.Once and ResetShared"`

---

## Phase 8 — Documentation (LC-0 + doc_test.go)
**Recommended agent**: Codex CLI.

### Step 8.1 — Replace `exporter/doc.go` placeholder

- [ ] Write the full package documentation per NFR-D `nfr-design-patterns.md` §5.2.
- [ ] Cover: overview, typical usage, lifecycle, configuration priority (4 layers), shared holder usage.

### Step 8.2 — Create `exporter/doc_test.go`

- [ ] `ExampleNew` — construct Pipeline, use TracerProvider, Shutdown. Must include `// Output:` line (empty if no output expected).
- [ ] `ExampleConfig_MergeWith` — show merge priority with `fmt.Println(merged.Endpoint, merged.Timeout)` and matching `// Output: override:4317 10s`.
- [ ] `ExampleGetShared` — show `GetShared(factory)` once.
- [ ] Examples MUST compile and pass `go test ./exporter/...`.

### Phase 8 commit

- [ ] `git add exporter/doc.go exporter/doc_test.go && git commit -m "docs(exporter): add package documentation and Example functions"`

---

## Phase 9 — Round-trip PBT (TP-U4-3)
**Recommended agent**: Codex CLI.

### Step 9.1 — Create `exporter/otlp_roundtrip_test.go`

- [ ] `TestOTLPProtobufRoundTrip` using `rapid.Check`:
  - Generate `*coltracepb.ExportTraceServiceRequest` with random ResourceSpans / ScopeSpans / Spans (use realistic field types: trace_id 16 bytes, span_id 8 bytes, etc.).
  - Marshal → Unmarshal → assert `proto.Equal(orig, parsed)`.
- [ ] Optionally also TestOTLPMetricsRoundTrip, TestOTLPLogsRoundTrip (analogous). Decision: implement at least Traces; Metrics and Logs are nice-to-have but acceptable to defer.
- [ ] Uses `go.opentelemetry.io/proto/otlp/collector/trace/v1` (test-only import).
- [ ] All tests call `t.Parallel()`.

### Phase 9 commit

- [ ] `git add exporter/otlp_roundtrip_test.go && git commit -m "test(exporter): add PBT for OTLP protobuf round-trip (TP-U4-3)"`

---

## Phase 10 — Stats Monotonicity PBT (TP-U4-4)
**Recommended agent**: Codex CLI.

### Step 10.1 — Create `exporter/stats_monotonic_test.go`

- [ ] `TestStats_Monotonic_Property` using `rapid.Check`:
  - For each iteration, build a Pipeline backed by a mock that may succeed or fail per call.
  - Run `nOps := rapid.IntRange(1, 20)` `simulateExport` calls (call `tracingExporter.ExportSpans` etc. with random batch sizes).
  - After each, capture `p.Stats()` and assert each `*Exported` and `*Failed` field is `>=` the previous observation.
- [ ] All tests call `t.Parallel()`.

### Phase 10 commit

- [ ] `git add exporter/stats_monotonic_test.go && git commit -m "test(exporter): add stateful PBT for Stats monotonicity (TP-U4-4)"`

---

## Phase 11 — Benchmark
**Recommended agent**: Codex CLI.

### Step 11.1 — Create `exporter/bench_test.go`

- [ ] `BenchmarkNew` — fixed `benchConfig` (gRPC, localhost:4317, Insecure, Timeout 5s, BatchSize 512, MaxQueueSize 2048, BatchTimeout 1s).
- [ ] Each iteration: `New(benchConfig)` → `p.Shutdown(context.Background())`.
- [ ] `b.ReportAllocs()`.
- [ ] Verify locally: `go test -bench=BenchmarkNew -benchmem ./exporter/...` runs in well under 100 ms per iteration.

### Phase 11 commit

- [ ] `git add exporter/bench_test.go && git commit -m "test(exporter): add BenchmarkNew with fixed Config"`

---

## Phase 12 — U7 Generator Additions (`ValidConfig` / `AnyConfig`)
**Recommended agent**: Codex CLI (or Cursor batch if Codex prefers focus).

> Per Q12=A in FD, add 2 new generators in `testutil/generators/` for U4's PBT.

### Step 12.1 — Add `testutil/generators/exporter_config.go`

- [ ] Implement `ValidConfig(opts ...ConfigOption) *rapid.Generator[exporter.Config]` per FD `domain-entities.md` §6.
- [ ] Implement `AnyConfig(opts ...ConfigOption) *rapid.Generator[exporter.Config]` (allows invalid values: negative timeouts, empty endpoints, MaxQueueSize < BatchSize, unknown compression, etc.).
- [ ] Define `ConfigOption` and at least these helpers: `WithFixedEndpoint(string)`, `WithProtocol(exporter.Protocol)`, `WithMinTimeout(time.Duration)`.
- [ ] Follow existing U7 generator style (constants, helpers, doc comments).

### Step 12.2 — Add `testutil/generators/exporter_config_test.go`

- [ ] `TestValidConfig_PassesValidate_Property` — `ValidConfig().Draw(t, "cfg")` → `cfg.Validate()` returns nil.
- [ ] `TestAnyConfig_SometimesInvalid_Property` — `AnyConfig().Draw(...)` over many iterations occasionally fails Validate (use a counter to check distribution within rapid).
- [ ] All tests call `t.Parallel()`.

### Step 12.3 — Un-skip Phase 2 PBT tests

- [ ] In `exporter/config_test.go`, remove `t.Skip("waits for ValidConfig generator from Phase 12")` from `TestMergeWith_OverrideWins_Property` and `TestMergeWith_Idempotent_Property`.
- [ ] Import `github.com/ymotongpoo/xk6-otel-gen/testutil/generators` in `config_test.go` if not already.
- [ ] Run `go test -race ./exporter/...` — all PBT tests pass.

### Phase 12 commit

- [ ] `git add testutil/generators/exporter_config.go testutil/generators/exporter_config_test.go exporter/config_test.go && git commit -m "feat(testutil): add ValidConfig and AnyConfig generators for U4 PBT"`

---

## Phase 13 — Integration Test Harness
**Recommended agent**: Codex CLI.

> `-tags=integration` only — does NOT run in default `go test`.

### Step 13.1 — Create `exporter/testdata/collector-config.yaml`

- [ ] Collector config with OTLP/gRPC receiver on `:4317` and file_exporter writing to `/var/log/otel/traces.json`, `metrics.json`, `logs.json`.
- [ ] 3 pipelines (traces, metrics, logs) all routing OTLP → file.

### Step 13.2 — Create `exporter/testdata/docker-compose.yaml`

- [ ] Single service `collector` using `otel/opentelemetry-collector-contrib:latest`, mounts `./collector-config.yaml` and `./otel-logs/` (volume for output JSON), exposes `4317` and `4318`.

### Step 13.3 — Create `exporter/integration/helpers.go`

- [ ] `StartCollector(t *testing.T) (endpoint string, cleanup func())` — uses `os/exec` to run `docker compose -f testdata/docker-compose.yaml up -d`, waits for port readiness, returns cleanup that runs `docker compose down`.
- [ ] `ReadCollectorTraces(t *testing.T) []byte` — reads `./otel-logs/traces.json`.
- [ ] Analogous for Metrics, Logs.
- [ ] All helpers use `t.Helper()`.

### Step 13.4 — Create `exporter/integration/integration_test.go`

- [ ] `//go:build integration` build tag at top.
- [ ] `TestIntegration_ThreeSignals_Correlated` — construct Pipeline, emit one span / one metric data point / one log record sharing the same `trace_id` and `span_id`, force flush, Shutdown, then read all three JSON files and assert the same trace_id / span_id appears across all three.
- [ ] All tests call `t.Parallel()` only within the same test (since they share the Collector singleton).

### Step 13.5 — Update CI hint (no actual workflow change, just documentation)

- [ ] In `exporter/integration/README.md`, document: "`go test -tags=integration ./exporter/integration/...` requires Docker. Skipped by default."

### Phase 13 commit

- [ ] `git add exporter/integration/ exporter/testdata/ && git commit -m "test(exporter): add integration test harness with Collector and 3-signal correlation"`

---

## Phase 14 — Final Wrap & DoD Verification
**Recommended agent**: Codex CLI.

### Step 14.1 — Run full suite

- [ ] `go build ./...` — succeeds.
- [ ] `go vet ./exporter/...` — clean.
- [ ] `go test -race -count=1 ./exporter/...` — passes.
- [ ] `go test -cover ./exporter/...` — verify coverage ≥ 80%. If below, add targeted tests.
- [ ] `go test -bench=BenchmarkNew -benchmem ./exporter/...` — verify per-iteration < 100 ms.

### Step 14.2 — Create `aidlc-docs/construction/u4-exporter/code/code-generation-summary.md`

- [ ] Document: files created (with line counts), test coverage final %, BenchmarkNew result, any deviations from plan.
- [ ] Include `git log --oneline | head -20` of the U4-related commits.

### Step 14.3 — Mark all checkboxes [x]

- [ ] Walk back through this plan and verify every `[ ]` is `[x]`. If any remain, address the gap (or document why intentionally skipped).

### Step 14.4 — Update `aidlc-docs/aidlc-state.md`

- [ ] Set `Current Unit` to U3.
- [ ] Add U4 to completed list.

### Phase 14 commit

- [ ] `git add aidlc-docs/ && git commit -m "chore(u4-exporter): finalize code-generation-summary and checkbox state"`

---

## Anti-patterns to AVOID during implementation

(Per NFR-D `nfr-design-patterns.md` §10)

- ❌ Generics over the per-signal wrapper (SDK interface types differ; concrete is clearer)
- ❌ `NewWithFactory(cfg, factory)` or other production-code test hooks (API pollution)
- ❌ Functional options for Config (struct literal is the chosen idiom)
- ❌ Adding `*QueueLen` fields to Stats "for future compatibility" (verified absent in OTel SDK upstream)
- ❌ Wrapping internal queue counters in our wrapper to fake QueueLen (creates misleading values)
- ❌ Importing `propagation` package (in-process telemetry synthesis)
- ❌ Importing `semconv` (use raw attribute keys to avoid version-coupling)
- ❌ Using real Collector in unit tests (use mock; integration tests are separate `-tags=integration`)
- ❌ `errors.Join` to combine cleanup errors with primary error (Q10=A: discard cleanup errors)
- ❌ Splitting `pipeline.go` into multiple files (Q11=A: keep together)
- ❌ Splitting `exporters.go` by signal (Q11=A: 3 functions in 1 file is the chosen split)

---

## Notes for the implementing agent

1. **OTel SDK module coordination**: All `go.opentelemetry.io/otel/*` modules SHOULD be at the same version. If `go mod tidy` resolves them to mismatched minor versions, run `go get` to align them.
2. **Mock placement**: Prefer mocks in `_test.go` files within `package exporter` (internal test package) so internal helpers can be exercised. If a mock needs to be reused across test files, place it in `mocks_test.go`.
3. **`t.Parallel()` and `shared.go`**: Tests touching the package-level `sharedPipeline` MUST NOT run in parallel with other tests touching it. Use a `var sharedTestMu sync.Mutex` if needed, or document that `shared_test.go` runs serially.
4. **`Stats` field count**: Public `Stats` has exactly **6 fields** (no `*QueueLen`). Do not add fields "for future compatibility" — the FD explicitly forbids this.
5. **Conventional Commits**: After each Phase, propose a commit per the per-Phase template, including the `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` trailer (Codex may add its own author-bot trailer in addition).
6. **Sandbox mode**: This run uses `--sandbox danger-full-access` per `scripts/run-codex.sh` because workspace-write blocks `.git` writes. Documented trade-off accepted.
