# U3 (synth) — Code Generation Plan

> **This file is the Single Source of Truth (SSOT) for the U3 implementation.**
>
> **Audience**: Codex CLI (`gpt-5.5 xhigh`) for autonomous batch execution + Cursor Composer 2.5 for any single-shot follow-ups via `agent -p "<prompt>"` (Cursor Agent CLI binary is `agent`). Read `AGENTS.md` for role boundaries.
>
> **Recommended agent per Phase**:
> - **Phase 0-14 → Codex** (multi-file new construction, type-system reasoning around OTel SDK interfaces + semconv). Run via `scripts/run-codex.sh u3`.
> - **Phase 11 (U7 generator addition) MAY be split off to Cursor batch** if Codex prefers focus.
>
> **Execution model**: Work through the checkboxes top-to-bottom. Mark each `[ ]` → `[x]` immediately upon completion.
>
> **Source artifacts to reference while implementing**:
> - FD: `aidlc-docs/construction/u3-synth/functional-design/{business-logic-model,business-rules,domain-entities}.md`
> - NFR-R: `aidlc-docs/construction/u3-synth/nfr-requirements/{nfr-requirements,tech-stack-decisions}.md`
> - NFR-D: `aidlc-docs/construction/u3-synth/nfr-design/{nfr-design-patterns,logical-components}.md` ← **most prescriptive**
> - Application Design (types): `aidlc-docs/inception/application-design/component-methods.md` §C3
> - U7 generator existing style: `testutil/generators/*.go`
> - U4 completed exporter package: `exporter/` (Pipeline + Provider accessor)
> - PBT rules: `.aidlc-rule-details/extensions/testing/property-based/property-based-testing.md`
> - Agent contract: `AGENTS.md`

---

## Unit Context

- **Unit ID**: U3
- **Purpose**: Implement `synth/` — OTel signal synthesis (spans, metrics, logs) from topology + journey-engine inputs, using semconv v1.27.0 attribute conventions.
- **Workspace root**: `/home/ymotongpoo/repos/xk6-otel-gen/`
- **Go module path**: `github.com/ymotongpoo/xk6-otel-gen`
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → **U3 (this)** → U2 → U5 → U6 → U8
- **PBT requirements satisfied by this unit**:
  - **PBT-01** (Property Identification) — 4 properties documented (TP-U3-1..4)
  - **PBT-03** (Invariants) — TP-U3-2 (Allowed Attr Keys), TP-U3-3 (Histogram Insertion), TP-U3-4 (error.type Required)
  - **PBT-04** (Idempotency) — TP-U3-1 (BuildResource)
- **NFR DoD** (from `nfr-requirements.md` §4):
  - `go build ./synth/...` succeeds
  - `go vet ./synth/...` clean
  - `go test -race -count=1 ./synth/...` passes
  - `go test -cover ./synth/...` ≥ 80%
  - Bench: BeginSpan <10µs, RecordMetric <5µs, EmitLog <10µs, BuildResource <50µs (per iteration)
  - All exported identifiers have GoDoc
  - 3 Example functions
  - U7 generators (`ValidSpanInput` / `AnySpanInput` / `ValidMetricInput` / `AnyMetricInput` / `ValidLogInput` / `AnyLogInput` / `ValidOutcome` / `AnyOutcome` + optional `ValidErrorType`) added to `testutil/generators/`
  - `golangci-lint run ./synth/...` passes
  - Integration test (`-tags=integration ./synth/integration/...`) passes against Docker Collector with 3-signal trace_id correlation
- **Dependencies added by this unit**:
  - `go.opentelemetry.io/otel/semconv/v1.27.0` (already added by U4 refactor commit 622f6cf, verify)
  - `github.com/google/uuid` (NEW)
  - All other OTel deps already present from U4
- **FD deviation note**: NFR-D `logical-components.md` §LC-5 omits `errors.go` (FD §3 / `domain-entities.md` §3 had it). Rationale: U3 emits no error value through any external-facing channel. NFR-D supersedes FD on this point.

---

## Phase 0 — Environment Setup
**Recommended agent**: Codex CLI.

### Step 0.1 — Verify deps

- [x] `grep "semconv/v1.27.0" go.sum` confirms it's a direct dep (added by U4 refactor 622f6cf).
- [x] `go get github.com/google/uuid@latest` → `go mod tidy`.
- [x] Verify: `grep "github.com/google/uuid" go.mod` shows it as a direct dep.

### Step 0.2 — Create synth/ directory skeleton

- [x] Create `synth/` directory.
- [x] Create empty `synth/doc.go` with placeholder `package synth` line + TODO comment (full doc in Phase 8).
- [x] Verify: `go build ./synth/...` succeeds (empty package).

### Phase 0 commit

- [x] `git add go.mod go.sum synth/doc.go && git commit -m "build(synth): add uuid dependency for U3"`

---

## Phase 1 — Public Interface & Types (LC-1)
**Recommended agent**: Codex CLI.

> The base API surface that all other LCs hang off of.

### Step 1.1 — Create `synth/interface.go`

- [x] Define `Synthesizer` interface (`BeginSpan`, `RecordMetric`, `EmitLog`) per NFR-D `logical-components.md` LC-1.
- [x] Define `SpanInput` struct (Service, Edge, Operation, StartTime, InstanceIdx).
- [x] Define `MetricInput` struct (Service, Edge, Operation, Latency, Outcome, InstanceIdx).
- [x] Define `LogInput` struct (Service, Severity, Body, Attributes).
- [x] Define `Outcome` struct (Success, StatusCode, ErrorType, EndTime).
- [x] Define `FinishSpanFunc` type alias.
- [x] All exported identifiers have GoDoc referencing semconv v1.27.0 + journey engine usage.

### Step 1.2 — Unit test `synth/interface_test.go`

- [x] `TestSpanInput_FieldAccess` — sanity construction with all fields populated.
- [x] `TestOutcome_FieldAccess` — sanity construction.
- [x] All tests call `t.Parallel()`.

### Phase 1 commit

- [x] `git add synth/interface.go synth/interface_test.go && git commit -m "feat(synth): add Synthesizer interface and IO types"`

---

## Phase 2 — Resource Builder (LC-2)
**Recommended agent**: Codex CLI.

### Step 2.1 — Create `synth/resource.go`

- [x] Define package-level `synthInstanceNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("xk6-otel-gen/synth"))`.
- [x] Implement `InstanceID(svcName string, idx int) string` (deterministic UUID v5).
- [x] Implement `BuildResource(svc *topology.Service, instanceIdx int) *resource.Resource` per NFR-D LC-2.
- [x] Panic format per NFR-D Q7=A.
- [x] svc.Language → `process.runtime.name` with explanatory comment.
- [x] svc.Framework → `synth.service.framework` custom attribute.
- [x] All exported identifiers have GoDoc.

### Step 2.2 — Unit test `synth/resource_test.go`

- [x] `TestBuildResource_Minimal` — only required fields populated.
- [x] `TestBuildResource_AllFields` — Version, Language, Framework all populated.
- [x] `TestBuildResource_NilPanics` — svc nil → panic.
- [x] `TestBuildResource_InvalidIdxPanics` — idx < 0 → panic.
- [x] `TestBuildResource_EmptyNamePanics` — svc.Name == "" → panic.
- [x] `TestInstanceID_Deterministic` — same (name, idx) → same UUID string.
- [x] **PBT TP-U3-1** (`TestBuildResource_Idempotent_Property`): mark `t.Skip("waits for ValidService generator from Phase 11")` for now; un-skip in Phase 11.
- [x] All tests call `t.Parallel()`.

### Phase 2 commit

- [x] `git add synth/resource.go synth/resource_test.go && git commit -m "feat(synth): add Resource builder with deterministic service.instance.id"`

---

## Phase 3 — Attribute Policy & Builders (LC-3)
**Recommended agent**: Codex CLI.

### Step 3.1 — Create `synth/attributes.go`

- [x] Define `direction uint8` type with `dirClient`, `dirServer`, `dirProducer`, `dirConsumer`, `dirInternal`, `dirUnset` consts.
- [x] Define `attributePolicy struct { SpanKind trace.SpanKind; AttributeNamespace string; MetricNamespace string; Direction direction }`.
- [x] Implement `policyFor(svcKind topology.ServiceKind, edgeKind topology.EdgeKind, dir direction) attributePolicy` per FD `business-rules.md` §2.1 table.
- [x] Define `cacheKey struct { svcName string; op string; edgeID string; dir direction }`.
- [x] Define `staticSetCache struct { sets sync.Map }` with `get` / `put` methods.
- [x] Implement `buildStaticSet(svc *topology.Service, op string, edge *topology.Edge, policy attributePolicy) attribute.Set` dispatching to `httpStaticAttrs` / `rpcStaticAttrs` / `dbStaticAttrs` / `messagingStaticAttrs`.
- [x] Implement per-namespace static attr builders using semconv constants (semconv.HTTPRequestMethodKey etc.).
- [x] Implement `dynamicOutcomeAttrs(policy, outcome) []attribute.KeyValue` for status_code + error.type.
- [x] Define `allowedAttrKeys map[string]struct{}` listing all semconv keys this package may emit (per NFR-D §1.2 / TP-U3-2).
- [x] Implement `cacheKeyFor(svc, op, edge, dir) cacheKey` helper.
- [x] HTTP operation parser: `parseHTTPOp(op string) (method, route string)` (e.g., `"GET /api/users"` → `("GET", "/api/users")`).
- [x] Internal helpers — brief comments.

### Step 3.2 — Unit test `synth/attributes_test.go`

- [x] `TestPolicyFor_AllCombinations` — table-driven over (Service.Kind, Edge.Kind, direction) → expected policy.
- [x] `TestBuildStaticSet_HTTP_Server` — Service.Kind=application, Edge.Kind=http, dir=server → expected http.method + http.route attrs.
- [x] Similar for HTTP_Client, RPC_Server/Client, DB, Messaging Producer/Consumer.
- [x] `TestDynamicOutcomeAttrs_*` — status_code present, error.type on failure, etc.
- [x] `TestStaticSetCache_GetPut` — basic sync.Map wrap.
- [x] **PBT TP-U3-2** (`TestSpanAttributes_AllowedKeysOnly_Property`): mark `t.Skip("waits for ValidSpanInput generator from Phase 11")` for now.
- [x] All tests call `t.Parallel()`.

### Phase 3 commit

- [x] `git add synth/attributes.go synth/attributes_test.go && git commit -m "feat(synth): add attribute policy mapping and static/dynamic builders"`

---

## Phase 4 — Synthesizer Skeleton (LC-4 part 1)
**Recommended agent**: Codex CLI.

> Set up `defaultSynthesizer` struct, `NewDefault` with eager instrument creation, plus race build tag pair.

### Step 4.1 — Create `synth/race_on.go` and `synth/race_off.go`

- [x] `race_on.go` with `//go:build race` + `const raceEnabled = true`.
- [x] `race_off.go` with `//go:build !race` + `const raceEnabled = false`.

### Step 4.2 — Create `synth/synthesizer.go` (struct + NewDefault only)

- [x] Define `defaultSynthesizer` struct with all 9 instrument fields + tracer/meter/logger/staticSetCache (per NFR-D LC-4).
- [x] Implement `NewDefault(tp, mp, lp) Synthesizer`:
  - nil-check panic (Q4=A)
  - `meter := mp.Meter("github.com/ymotongpoo/xk6-otel-gen/synth")`
  - tracer / logger similarly
  - 9 instrument creation via `mustHistogram` / `mustUDC` helpers (panic on err)
  - return `*defaultSynthesizer`.
- [x] All public identifiers have full GoDoc.

### Step 4.3 — Unit test `synth/synthesizer_test.go` (skeleton)

- [x] `TestNewDefault_NilProvider_Panics` — each of tp/mp/lp nil → panic.
- [x] `TestNewDefault_BuildsAllInstruments` — uses `tracetest`+`ManualReader` providers, asserts all 9 instruments are non-nil.
- [x] Add `helpers_test.go` with `newTestProviders(t)` helper (skeleton).
- [x] All tests call `t.Parallel()`.

### Phase 4 commit

- [x] `git add synth/synthesizer.go synth/race_on.go synth/race_off.go synth/synthesizer_test.go synth/helpers_test.go && git commit -m "feat(synth): add defaultSynthesizer skeleton with eager instrument creation"`

---

## Phase 5 — BeginSpan + FinishSpanFunc (LC-4 part 2)
**Recommended agent**: Codex CLI.

### Step 5.1 — Add `BeginSpan` to `synth/synthesizer.go`

- [ ] Validate input (panic on nil Service, empty Operation, out-of-range InstanceIdx) per NFR-D §2.2.
- [ ] Compute `direction` from `(svc.Kind, edge, op role)` — for now, derive simple heuristic (Edge nil → Server, Edge non-nil and svc == Edge.From → Client, etc.); document as best-effort and revisit when U2 FD lands.
- [ ] Call `policyFor` to determine SpanKind + namespace.
- [ ] Build span attrs via `buildStaticSet` + initial dynamic (typically empty at start).
- [ ] `tracer.Start(ctx, spanName, ...)`.
- [ ] `maybeIncActive(ctx, in, policy, +1)`.
- [ ] Build `FinishSpanFunc` closure with `atomic.Bool` double-call protection + `raceEnabled` panic.
- [ ] Inside finish closure: SetStatus per Q7 mapping, SetAttributes (outcome dynamics), End with EndTime, `maybeIncActive(-1)`.
- [ ] Implement `maybeIncActive`, `statusFor`, `finishAttrs` helpers.
- [ ] Implement `spanName(svc, op)` helper.

### Step 5.2 — Unit test additions to `synth/synthesizer_test.go`

- [ ] `TestBeginSpan_Server_Success` — HTTP server span, finish success, assert in-memory span has correct attrs + Unset status.
- [ ] `TestBeginSpan_Server_Failure_500` — finish with Success=false, Status=500, ErrorType="http.500" → assert Error status + error.type attr.
- [ ] `TestBeginSpan_4xx_NoErrorStatus` — Success=false (or true), Status=404 → Status=Unset per semconv.
- [ ] `TestBeginSpan_Client_HTTP` — Edge non-nil, dir=client → SpanKind=Client + server.address attr.
- [ ] `TestBeginSpan_InvalidInput_Panics` — table-driven for nil Service, empty Operation, out-of-range InstanceIdx.
- [ ] `TestFinishSpanFunc_DoubleCall_NoOp` — non-race build: second call is no-op (verify only 1 span ended).
- [ ] `TestFinishSpanFunc_DoubleCall_RacePanic` — gated by build tag, see if testable in `-race` mode.
- [ ] `TestActiveRequests_BalancedAfterFinish` — start N spans, finish all, assert UDC value back to 0.

### Phase 5 commit

- [ ] `git add synth/synthesizer.go synth/synthesizer_test.go && git commit -m "feat(synth): add BeginSpan with FinishSpanFunc double-call protection and active_requests tracking"`

---

## Phase 6 — RecordMetric (LC-4 part 3)
**Recommended agent**: Codex CLI.

### Step 6.1 — Add `RecordMetric` to `synth/synthesizer.go`

- [ ] Validate input (panic on nil Service per NFR-D §3.1).
- [ ] Determine policy + histogram via internal helpers (`histogramFor(policy)` returns `metric.Float64Histogram` or nil if namespace empty).
- [ ] Lookup static set in `staticSetCache`; build + cache if miss.
- [ ] Build dynamic outcome attrs.
- [ ] `histogram.Record(ctx, latency.Seconds(), WithAttributeSet(static), WithAttributes(dynamic...))`.
- [ ] Implement `histogramFor(policy)` switching on policy.MetricNamespace + Direction.

### Step 6.2 — Unit test additions

- [ ] `TestRecordMetric_HTTP_Server` — record a measurement, ManualReader.Collect → verify histogram data point with expected attrs.
- [ ] `TestRecordMetric_RPC_Client` — similar.
- [ ] `TestRecordMetric_DB_Client` — similar.
- [ ] `TestRecordMetric_Messaging_Producer` — similar.
- [ ] `TestRecordMetric_ZeroLatency_StillRecords` — Latency=0 should still create data point.
- [ ] `TestRecordMetric_StaticSetCached` — second call with same (svc, op, edge, dir) reuses cached set (verify via Caches lookup count if possible, or by checking single allocation pattern).
- [ ] **PBT TP-U3-3** (`TestRecordMetric_HistogramInsertion_Property`): mark `t.Skip("waits for ValidMetricInput generator from Phase 11")`.
- [ ] **PBT TP-U3-4** (`TestFinishSpan_ErrorTypeRequired_Property`): mark skip same.

### Phase 6 commit

- [ ] `git add synth/synthesizer.go synth/synthesizer_test.go && git commit -m "feat(synth): add RecordMetric with hybrid static+dynamic attribute strategy"`

---

## Phase 7 — EmitLog (LC-4 part 4)
**Recommended agent**: Codex CLI.

### Step 7.1 — Add `EmitLog` to `synth/synthesizer.go`

- [ ] Validate input (panic on nil Service).
- [ ] Default Severity (Info if Undefined).
- [ ] Default Body fallback if empty.
- [ ] Build `log.Record` with Timestamp/ObservedTimestamp=Now, Severity, Body, attrs.
- [ ] Add semconv `service.name` to record attrs automatically.
- [ ] Implement `toLogValue(any) log.Value` helper for `LogInput.Attributes` map values.
- [ ] `logger.Emit(ctx, record)`.

### Step 7.2 — Unit test additions

- [ ] `TestEmitLog_Success` — Severity=Info, custom Body → record captured by helpers_test logRecorder.
- [ ] `TestEmitLog_NilService_Panics`.
- [ ] `TestEmitLog_EmptyBody_DefaultFallback` — empty Body → "<svc> event" or similar.
- [ ] `TestEmitLog_AttributesPropagated` — custom Attributes map keys all show up.
- [ ] `TestEmitLog_ServiceNameAuto` — service.name attribute always present.

### Phase 7 commit

- [ ] `git add synth/synthesizer.go synth/synthesizer_test.go synth/helpers_test.go && git commit -m "feat(synth): add EmitLog with default Body fallback and service.name auto-attribute"`

---

## Phase 8 — Documentation (LC-0 + doc_test.go)
**Recommended agent**: Codex CLI.

### Step 8.1 — Replace `synth/doc.go` placeholder

- [ ] Full package doc per NFR-D §5.2 (overview, usage, lifecycle, semconv version note).

### Step 8.2 — Create `synth/doc_test.go`

- [ ] `ExampleNewDefault` — construct + verify.
- [ ] `ExampleBuildResource` — show Resource construction with svc Replicas + Language + Framework, print attrs.
- [ ] `ExampleSynthesizer_BeginSpan` — full span lifecycle (BeginSpan → finish).
- [ ] All Examples must compile + pass `go test ./synth/...`.

### Phase 8 commit

- [ ] `git add synth/doc.go synth/doc_test.go && git commit -m "docs(synth): add package documentation and Example functions"`

---

## Phase 9 — PBT TP-U3-1 / TP-U3-2 (`pbt_test.go`)
**Recommended agent**: Codex CLI.

> Phase 9 implements PBT tests that **do not** require Phase 11 generators (TP-U3-1 needs ValidService from testutil/generators which already exists; TP-U3-2 can use locally-constructed inputs).

### Step 9.1 — Create `synth/pbt_test.go`

- [ ] `TestBuildResource_Idempotent_Property` (TP-U3-1) using existing `generators.ValidService`:
  - draw svc, draw idx in [0, svc.Replicas), call BuildResource twice, assert resource.Equal.
- [ ] `TestSpanAttributes_AllowedKeysOnly_Property` (TP-U3-2) using locally-built SpanInputs (don't wait for ValidSpanInput):
  - generate (ServiceKind, EdgeKind, Operation, direction) combinations, call BeginSpan + finish, capture span attributes from in-memory exporter, assert all keys ∈ `allowedAttrKeys`.
- [ ] All tests call `t.Parallel()`.

### Phase 9 commit

- [ ] `git add synth/pbt_test.go && git commit -m "test(synth): add PBT for BuildResource idempotency and allowed attribute keys"`

---

## Phase 10 — Benchmark
**Recommended agent**: Codex CLI.

### Step 10.1 — Create `synth/bench_test.go`

- [ ] `BenchmarkBuildResource` — fixed svc, idx=0.
- [ ] `BenchmarkBeginSpan_HTTP_Server` — fixed SpanInput (svc kind=application, edge nil), include finish call.
- [ ] `BenchmarkRecordMetric_HTTP_Server` — fixed MetricInput.
- [ ] `BenchmarkEmitLog` — fixed LogInput.
- [ ] All use ManualReader / tracetest providers (no exporter network call).
- [ ] `b.ReportAllocs()`.
- [ ] Verify locally: ns/op meets NFR-U3-6 (<10µs / <5µs / <10µs / <50µs).
- [ ] If a benchmark fails the budget, document the gap in code-generation-summary.md but do not block — note as TODO for sync.Pool fallback per NFR-D §1.2.4.

### Phase 10 commit

- [ ] `git add synth/bench_test.go && git commit -m "test(synth): add benchmarks for per-call latency targets"`

---

## Phase 11 — U7 Generator Additions
**Recommended agent**: Codex CLI (or Cursor batch).

> Per FD §6, add generators for synth IO types to `testutil/generators/`.

### Step 11.1 — Add `testutil/generators/synth_inputs.go`

- [ ] Implement `ValidSpanInput(opts ...SpanInputOption) *rapid.Generator[synth.SpanInput]` per FD §6.3.
- [ ] Implement `AnySpanInput(opts ...SpanInputOption) *rapid.Generator[synth.SpanInput]`.
- [ ] Same for MetricInput, LogInput, Outcome (4 pairs = 8 funcs).
- [ ] Optionally: `ValidErrorType() *rapid.Generator[string]` sampling from `SemconvErrorTypes` slice (use when implementing TP-U3-4 in Phase 12).
- [ ] Each generator follows existing U7 generator style (constants, doc comments).

### Step 11.2 — Add `testutil/generators/synth_inputs_test.go`

- [ ] `TestValidSpanInput_PassesValidation_Property` — `ValidSpanInput().Draw(t, "in")` produces inputs that don't panic `synth.BeginSpan`.
- [ ] `TestAnySpanInput_SometimesInvalid_Property` — distribution check.
- [ ] Same for other generators.

### Step 11.3 — Un-skip earlier PBT tests

- [ ] In `synth/pbt_test.go`, add tests using newly available generators:
  - `TestRecordMetric_HistogramInsertion_Property` (TP-U3-3) using `ValidMetricInput`.
  - `TestFinishSpan_ErrorTypeRequired_Property` (TP-U3-4) using `ValidSpanInput` + Outcome with Success=false.
- [ ] Run `go test -race ./synth/... ./testutil/generators/...` — all pass.

### Phase 11 commit

- [ ] `git add testutil/generators/synth_inputs.go testutil/generators/synth_inputs_test.go synth/pbt_test.go && git commit -m "feat(testutil): add synth IO generators for U3 PBT"`

---

## Phase 12 — Integration Test Harness
**Recommended agent**: Codex CLI.

> `-tags=integration` only — does NOT run in default `go test`. Aligned with U4's pattern.

### Step 12.1 — Create `synth/testdata/collector-config.yaml`

- [ ] OTLP/gRPC receiver + file_exporter to `/var/log/otel/{traces,metrics,logs}.json` (mimic U4).

### Step 12.2 — Create `synth/testdata/docker-compose.yaml`

- [ ] Single `collector` service, mount config + output dir, expose 4317/4318.

### Step 12.3 — Create `synth/integration/helpers.go`

- [ ] `StartCollector(t)` / `ReadCollectorTraces` / `ReadCollectorMetrics` / `ReadCollectorLogs` (reuse U4 pattern, but in a new copy or factor out shared module).
- [ ] `BuildPipeline(t, cfg)` — uses U4's `exporter.New(cfg)` to construct real Pipeline.

### Step 12.4 — Create `synth/integration/integration_test.go`

- [ ] `//go:build integration`.
- [ ] `TestIntegration_SynthToCollector_ThreeSignals_Correlated` — construct Pipeline, build Synthesizer, run one BeginSpan→finish, RecordMetric, EmitLog with same context, ForceFlush all, Shutdown, read all 3 JSON files, assert same trace_id across all signals.

### Step 12.5 — Add `synth/integration/README.md`

- [ ] Document Docker requirement + invocation.

### Phase 12 commit

- [ ] `git add synth/integration/ synth/testdata/ && git commit -m "test(synth): add integration test harness with 3-signal correlation"`

---

## Phase 13 — FD Revision for errors.go Removal
**Recommended agent**: Codex CLI (lightweight doc edit).

> NFR-D supersedes FD on `errors.go`. Update FD docs to reflect.

### Step 13.1 — Update `aidlc-docs/construction/u3-synth/functional-design/domain-entities.md`

- [ ] §3 file layout: remove `errors.go` from the list. Add a NOTE pointing to NFR-D §LC-5 for the rationale.
- [ ] §4 public API list: no change (errors were never exported).

### Step 13.2 — Update `business-logic-model.md` (if mentions errors.go)

- [ ] `grep "errors.go" business-logic-model.md business-rules.md` — if found, update.

### Phase 13 commit

- [ ] `git add aidlc-docs/construction/u3-synth/functional-design/ && git commit -m "docs(synth): reconcile FD with NFR-D — drop errors.go from file layout"`

---

## Phase 14 — Final Wrap & DoD Verification
**Recommended agent**: Codex CLI.

### Step 14.1 — Run full suite

- [ ] `go build ./...` succeeds.
- [ ] `go vet ./synth/...` clean.
- [ ] `go test -race -count=1 ./...` passes.
- [ ] `go test -cover ./synth/...` ≥ 80%.
- [ ] `go test -bench=. -benchmem ./synth/...` shows all bench within NFR-U3-6 budgets (or document failures).
- [ ] `golangci-lint run ./synth/...` passes.
- [ ] `go test -tags=integration ./synth/integration/...` passes (with Docker available).

### Step 14.2 — Create `aidlc-docs/construction/u3-synth/code/code-generation-summary.md`

- [ ] File list with line counts (production + test).
- [ ] Verification results (coverage %, bench numbers).
- [ ] Deviations from plan (if any).
- [ ] Recent commits (`git log --oneline | head -15`).

### Step 14.3 — Mark all plan checkboxes [x]

- [ ] Walk back through this plan; verify every `[ ]` is `[x]`. Document any intentionally skipped items.

### Step 14.4 — Update `aidlc-docs/aidlc-state.md`

- [ ] Mark U3 complete. Set Current Unit to U2 (Journey Engine).

### Phase 14 commit

- [ ] `git add aidlc-docs/ && git commit -m "chore(u3-synth): finalize code-generation-summary and checkbox state"`

---

## Anti-patterns to AVOID during implementation

(Per NFR-D `nfr-design-patterns.md` §9)

- ❌ Map-based instrument dispatch (`map[string]Histogram`)
- ❌ Production-code test hooks (`NewWithMockProviders(...)` etc.)
- ❌ `sync.Pool` for attribute slice (use only if bench fails NFR-U3-6 budget)
- ❌ Full (svc, op, edge, outcome) attribute set cache — Outcome combinatorics explode
- ❌ Resource caching (Q3=A no cache)
- ❌ Random UUID for service.instance.id (must be deterministic)
- ❌ `sync.Once` for finishFunc protection (use atomic.Bool)
- ❌ Separate `MarkActive`/`MarkInactive` API (active_requests is internal)
- ❌ Typed error payloads in panic (use formatted string)
- ❌ Per-test mock provider duplication (centralize in helpers_test.go)
- ❌ Explicit histogram bucket boundaries (SDK default for now)
- ❌ `synth.simulated.runtime` custom namespace (use `process.runtime.name` with reinterpretation comment)
- ❌ Integration tests under `exporter/integration/` (separate `synth/integration/`)
- ❌ Splitting `synthesizer.go` by signal — keep BeginSpan/RecordMetric/EmitLog together
- ❌ Importing `propagation` / `baggage` / SDK concrete types

---

## Notes for the implementing agent

1. **semconv import path**: Use exactly `go.opentelemetry.io/otel/semconv/v1.27.0` (already added by U4 refactor 622f6cf, do NOT bump).
2. **direction inference in BeginSpan**: Application Design §C3 + U2 are still TBD; for now use the heuristic in NFR-D §5.1 BeginSpan sketch and add a TODO comment that this will be refined by U2 FD.
3. **active_requests UpDownCounter**: Per NFR-D §1.1 there's no DB / Messaging active gauge; only HTTP + RPC server have them. Document this in `histogramFor` / `activeUDC` helper.
4. **Conventional Commits**: After each Phase, propose a commit per the per-Phase template. Include `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` (Codex may add its own author trailer).
5. **Sandbox mode**: This run uses `--sandbox danger-full-access` per `scripts/run-codex.sh u3`.
6. **Test parallelism**: All unit tests call `t.Parallel()`. Integration tests in `synth/integration/` may share the single Collector and run serially within the test function.
