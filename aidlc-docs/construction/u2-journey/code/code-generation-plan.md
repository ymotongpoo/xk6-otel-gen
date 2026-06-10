# U2 (journey) — Code Generation Plan

> **This file is the Single Source of Truth (SSOT) for the U2 implementation.**
>
> **Audience**: Codex CLI (`gpt-5.5 xhigh`) for autonomous batch execution + Cursor Composer 2.5 for any single-shot follow-ups via `agent -p "<prompt>"`.
>
> **Recommended agent per Phase**:
> - **Phase 0-14 → Codex** (multi-file new construction, deep type-system reasoning for cascade / recovery / fault precedence, plus a U3 coordination touch). Run via `scripts/run-codex.sh u2`.
> - **Phase 7 (U3 coordination) MAY be split off to Cursor batch** — it's a minor 2-file edit to synth (interface.go + synthesizer.go).
> - **Phase 10 (U7 generator addition) MAY be split off to Cursor batch**.
>
> **Execution model**: Work through the checkboxes top-to-bottom. Mark each `[ ]` → `[x]` immediately upon completion.
>
> **Source artifacts to reference while implementing**:
> - FD: `aidlc-docs/construction/u2-journey/functional-design/{business-logic-model,business-rules,domain-entities}.md`
> - NFR-R: `aidlc-docs/construction/u2-journey/nfr-requirements/{nfr-requirements,tech-stack-decisions}.md`
> - NFR-D: `aidlc-docs/construction/u2-journey/nfr-design/{nfr-design-patterns,logical-components}.md` ← **most prescriptive**
> - Application Design (types): `aidlc-docs/inception/application-design/component-methods.md` §C2
> - U1 completed topology package: `topology/` (FaultOverlay 3-method API, Edge.LatencyDist, Journey, Operation, RecoveryPolicy, ExhaustedAction)
> - U3 completed synth package: `synth/` (Synthesizer interface, SpanInput/MetricInput/LogInput/Outcome/FinishSpanFunc)
> - U7 generator existing style: `testutil/generators/*.go`
> - PBT rules: `.aidlc-rule-details/extensions/testing/property-based/property-based-testing.md`
> - Agent contract: `AGENTS.md`

---

## Unit Context

- **Unit ID**: U2
- **Purpose**: Implement `journey/` — Journey Engine orchestrating Plan construction, Execute with cascade / recovery / fault, replica selection, and real time.Sleep latency, delegating signal emission to synth.Synthesizer.
- **Workspace root**: `/home/ymotongpoo/repos/xk6-otel-gen/`
- **Go module path**: `github.com/ymotongpoo/xk6-otel-gen`
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → **U2 (this)** → U5 → U6 → U8
- **PBT requirements satisfied by this unit**:
  - **PBT-01** (Property Identification) — 5 properties (TP-U2-1..5)
  - **PBT-03** (Invariants) — TP-U2-2, TP-U2-3, TP-U2-4, TP-U2-5
  - **PBT-04** (Idempotency) — TP-U2-1 (BuildPlan)
- **NFR DoD** (from `nfr-requirements.md` §4):
  - `go build ./journey/...` succeeds
  - `go vet ./journey/...` clean
  - `go test -race -count=1 ./journey/...` passes
  - `go test -cover ./journey/...` ≥ 80%
  - `BenchmarkBuildPlan < 1ms / op`, `BenchmarkExecute_PureOverhead < 50µs / step`
  - PBT TP-U2-1..5 all pass
  - All exported identifiers have GoDoc
  - 3 Example functions
  - U7 generators (`ValidPlan/AnyPlan`, `ValidNode/AnyNode`, `ValidEngineOutcome/AnyEngineOutcome`) added
  - `golangci-lint run ./journey/...` passes
  - Integration test (`-tags=integration ./journey/integration/...`) passes against Docker Collector with cascade pattern verification
- **Dependencies added by this unit**:
  - `math/rand/v2` (stdlib, Go 1.22+ — already part of Go 1.25)
  - No new external modules
- **U3 coordination required**: extend `synth.Outcome` with `Cascaded bool` field (Phase 7) — synth-side patch needed before Phase 8 can use the attribute.
- **NFR-D adapter notes**:
  - U1 FaultOverlay actual API: `NodeFaults` / `OperationFaults` / `EdgeFaults` returning `[]FaultSpec`
  - U1 Edge has `Latency LatencyDist {Distribution, P50, P95}` (not Operation.Latency)
  - U2 internal helpers (`foldFaults`, `sampleEdgeLatency`) bridge to these actual shapes

---

## Phase 0 — Environment Setup
**Recommended agent**: Codex CLI.

### Step 0.1 — Verify deps

- [x] `head -3 go.mod` shows `go 1.25`.
- [x] No new external deps required (math/rand/v2 is stdlib).
- [x] Optional: scan `go.sum` to confirm `topology`, `synth` are accessible via local module (they are part of the same module).

### Step 0.2 — Create journey/ directory skeleton

- [x] Create `journey/` directory.
- [x] Create empty `journey/doc.go` with placeholder `package journey` + TODO comment (full doc in Phase 9).
- [x] Verify: `go build ./journey/...` succeeds (empty package).

### Phase 0 commit

- [x] `git add journey/doc.go && git commit -m "build(journey): scaffold package for U2"`

---

## Phase 1 — Errors & AllowedErrorTypes (LC-7)
**Recommended agent**: Codex CLI.

### Step 1.1 — Create `journey/errors.go`

- [x] Define `PlanError struct { Kind string; Path []string; Inner error }` with `Error()` + `Unwrap()` per NFR-D §3.3.
- [x] Define `ExecuteError struct { Kind string; Inner error }` with `Error()` + `Unwrap()`.
- [x] Define `AllowedErrorTypes []string` const with the 16 values from FD §11 (timeout, connection_refused, dns_failure, http.{500,502,503,504}, grpc.{unavailable,deadline_exceeded,unauthenticated}, db.{connection_lost,constraint_violation}, crashed, circuit_open, rate_limited, context_canceled).
- [x] All exported identifiers have GoDoc.

### Step 1.2 — Unit test `journey/errors_test.go`

- [x] `TestPlanError_Error` — formatting with and without Path.
- [x] `TestExecuteError_Error` — formatting with and without Inner.
- [x] `TestErrors_Unwrap` — errors.Is integration for both types.
- [x] `TestAllowedErrorTypes_NonEmpty_And_Unique`.
- [x] All tests call `t.Parallel()`.

### Phase 1 commit

- [x] `git add journey/errors.go journey/errors_test.go && git commit -m "feat(journey): add typed error hierarchy and AllowedErrorTypes constants"`

---

## Phase 2 — Engine + NewEngine + ListJourneys (LC-1)
**Recommended agent**: Codex CLI.

### Step 2.1 — Create `journey/engine.go`

- [x] Define `Engine struct { impl *engineImpl }` (opaque public, internal pointer).
- [x] Define `engineImpl struct { schema, overlay, synth, plans, journeyKeys, rand, rmu }` per NFR-D §1.1.
- [x] Implement `NewEngine(schema, overlay, syn) *Engine`:
  - nil-check panic with formatted message ("journey: NewEngine: ... must not be nil")
  - Initialize plans map, rand source via `newDefaultRand()`
  - Loop `schema.Journeys`, call `e.impl.buildPlan(name)` (stubbed for now, fully implemented in Phase 3), populate plans + journeyKeys
  - Sort journeyKeys
  - Panic if buildPlan returns error (NewEngine fail-fast)
- [x] Implement `(*Engine).ListJourneys() []string` returning copy of `journeyKeys`.
- [x] All public identifiers have GoDoc.

> **NOTE**: `buildPlan` body is added in Phase 3. For Phase 2, stub buildPlan to return `&Plan{JourneyName: name, Root: &Node{}}` so NewEngine compiles and passes minimal tests.

### Step 2.2 — Unit test `journey/engine_test.go`

- [x] `TestNewEngine_NilArgs_Panic` — each of schema/overlay/syn nil → panic.
- [x] `TestNewEngine_EmptySchema` — schema with no Journeys → no panic, ListJourneys returns empty slice.
- [x] `TestListJourneys_SortedKeys` — schema with 3 journeys → ListJourneys returns sorted slice.
- [x] `TestListJourneys_ReturnsCopy` — caller modifying returned slice doesn't affect Engine internal state.

### Phase 2 commit

- [x] `git add journey/engine.go journey/engine_test.go && git commit -m "feat(journey): add Engine struct with NewEngine and ListJourneys"`

---

## Phase 3 — Plan + Node + BuildPlan (LC-2)
**Recommended agent**: Codex CLI.

### Step 3.1 — Create `journey/plan.go`

- [x] Define `Plan struct { JourneyName string; Root *Node }`.
- [x] Define `Node struct { Service, Operation, Edge, Parallel, Children }`.
- [x] Implement `(*Engine).BuildPlan(name) (*Plan, error)` returning cached plan or `*PlanError{Kind: "unknown_journey"}`.
- [x] Implement `(*engineImpl).buildPlan(name) (*Plan, error)` with DFS expansion per NFR-D LC-2:
  - Look up `j := schema.Journeys[name]`, return `unknown_journey` if missing or `empty_journey` if Steps==0
  - Build root Node by walking j.Steps with `buildStepNode`
  - `buildStepNode` handles Step.Parallel (virtual fan-out) vs Step.Op.Calls (sequential children)
- [x] Replace the buildPlan stub from Phase 2 with the real implementation.
- [x] All exported identifiers have GoDoc.

> **NOTE**: U1's actual Step / Operation / Call types must be inspected (`topology/types.go`) to wire up correctly. The plan-builder code may need light adaptation if U1's Operation.Calls shape differs from FD's sketch.

### Step 3.2 — Unit test `journey/plan_test.go`

- [x] `TestBuildPlan_UnknownJourney_ReturnsError`.
- [x] `TestBuildPlan_EmptyJourney_ReturnsError`.
- [x] `TestBuildPlan_SingleStep_HappyPath` — verify Root.Service / Operation are populated.
- [x] `TestBuildPlan_ParallelSteps` — Step.Parallel creates virtual fan-out Node.
- [x] `TestBuildPlan_NestedCalls` — Op.Calls expanded as Children.
- [x] **PBT TP-U2-1** (`TestBuildPlan_Idempotent_Property`): mark `t.Skip("waits for ValidSchema generator from testutil/generators (already exists) — implement in Phase 11")` for now.
- [x] **PBT TP-U2-2** (`TestBuildPlan_AllOpsVisited_Property`): mark skip same.
- [x] All tests call `t.Parallel()`.

### Phase 3 commit

- [x] `git add journey/plan.go journey/plan_test.go && git commit -m "feat(journey): add Plan/Node types and BuildPlan DFS algorithm"`

---

## Phase 4 — Replica Selector (LC-6)
**Recommended agent**: Codex CLI.

### Step 4.1 — Create `journey/replica.go`

- [x] Implement `newDefaultRand() *rand.Rand` using `rand.NewPCG(seed1, seed2)` (math/rand/v2).
- [x] Implement `(*engineImpl).randIntN(n int) int` with mutex protection.
- [x] Implement `(*engineImpl).randFloat64() float64` with mutex protection.
- [x] Internal helpers, brief comments.

### Step 4.2 — Unit test `journey/replica_test.go`

- [x] `TestRandIntN_ZeroOrOne_ReturnsZero` — n=0 or n=1 → 0.
- [x] `TestRandIntN_Range` — n=10, draw 1000 times, verify all in [0, 10).
- [x] `TestRandFloat64_Range` — draw 1000 times, verify all in [0.0, 1.0).
- [x] `TestRand_Concurrent_NoRace` — concurrent randIntN calls from multiple goroutines (race detector verifies safety).
- [x] All tests call `t.Parallel()`.

### Phase 4 commit

- [x] `git add journey/replica.go journey/replica_test.go && git commit -m "feat(journey): add per-Engine random source with mutex protection"`

---

## Phase 5 — Fault Adapter (LC-5)
**Recommended agent**: Codex CLI.

### Step 5.1 — Create `journey/fault.go`

- [x] Define `foldedFault struct { crashed, disconnected, errorRate, errorType, latencyInflate }` per NFR-D §4.
- [x] Implement `(*engineImpl).foldFaults(node *Node) foldedFault`:
  - Scan `e.overlay.NodeFaults(node.Service)` for FaultCrash + FaultLatencyInflation
  - Scan `e.overlay.EdgeFaults(node.Edge)` (if Edge != nil) for FaultDisconnect + FaultLatencyInflation
  - Look up node.Service.Operations[node.Operation], scan `e.overlay.OperationFaults(op)` for FaultErrorRateOverride + FaultLatencyInflation
- [x] Implement `(*engineImpl).sampleInflation(spec topology.FaultSpec) time.Duration`. Code should be defensive: read `spec.Severity` based on U1's actual FaultSpec.Severity shape; if Severity has Delay / Multiplier fields, use them; otherwise return zero with TODO.
- [x] Implement `(*engineImpl).sampleEdgeLatency(edge *topology.Edge) time.Duration`:
  - edge nil → `defaultEntryLatency` (= 10 * time.Millisecond)
  - switch on `edge.Latency.Distribution`: "fixed" / "" → P50, "lognormal" → `sampleLognormal(P50, P95)`, "uniform" → `sampleUniform(P50, P95)`, default → P50
- [x] Implement `sampleLognormal` / `sampleUniform` helpers (use `e.randFloat64()`).
- [x] All internal, brief comments.

### Step 5.2 — Unit test `journey/fault_test.go`

- [x] `TestFoldFaults_Crash` — overlay with FaultCrash on service → ff.crashed=true.
- [x] `TestFoldFaults_Disconnect` — overlay with FaultDisconnect on edge → ff.disconnected=true.
- [x] `TestFoldFaults_ErrorRate` — overlay with FaultErrorRateOverride on operation → ff.errorRate populated.
- [x] `TestFoldFaults_LatencyInflation_Accumulates` — multiple inflation faults → latencies sum.
- [x] `TestFoldFaults_Precedence` — crash + disconnect + error_rate + latency_inflation all set → all reflected in foldedFault (Outcome derivation tested in Phase 8).
- [x] `TestSampleEdgeLatency_NilEdge_Default` — nil edge → 10 ms.
- [x] `TestSampleEdgeLatency_Fixed` — Distribution="" or "fixed" → P50 exactly.
- [x] `TestSampleEdgeLatency_Lognormal_InRange` — Distribution="lognormal" → values clustered around P50.
- [x] All tests call `t.Parallel()`.

### Phase 5 commit

- [x] `git add journey/fault.go journey/fault_test.go && git commit -m "feat(journey): add FaultOverlay adapter with precedence folding and latency sampling"`

---

## Phase 6 — Executor Skeleton (LC-3 part 1)
**Recommended agent**: Codex CLI.

> Implement `Execute` and `executeNode` happy path **without** recovery and cascade emission. Recovery (LC-4) + cascade attribute requires Phase 7 (U3 coordination) and Phase 8 (full executor).

### Step 6.1 — Create `journey/executor.go`

- [x] Implement `(*Engine).Execute(ctx context.Context, plan *Plan) error`:
  - nil-check (return `*ExecuteError{Kind: "nil_plan"}` or `"nil_ctx"`)
  - `defer recover()` for top-level panic → `*ExecuteError{Kind: "internal"}`
  - Call `e.impl.executeNode(ctx, plan.Root, nil)`
- [x] Implement `(*engineImpl).executeNode(ctx, node, parent) Outcome`:
  - virtual fan-out (Parallel != nil) → dispatch to executeParallelGroup
  - cascade check (parent != nil && !parent.Success) → return basic cascade Outcome without span emission (cascade emission added in Phase 8)
  - fault evaluation via foldFaults (handle crash + disconnect + error_rate)
  - replica selection via randIntN
  - sample base latency via sampleEdgeLatency
  - effectiveLatency = base + inflation
  - BeginSpan via synth, get spanCtx + finishFn
  - select on time.After with ctx.Done() handling (ctx cancel → finish + return)
  - Iterate Children (sequential), recursive executeNode
  - finishFn(toSynthOutcome(...))
  - RecordMetric + EmitLog
- [x] Implement `(*engineImpl).executeParallelGroup(ctx, group, parent) Outcome`:
  - sync.WaitGroup + goroutine-per-child with `defer recover()` per child
  - aggregateParallelOutcomes (max latency, any-failure-aggregates)
- [x] Helper functions: `pickStatusCode`, `toSynthOutcome`, `waitWithCancel`, `aggregateParallelOutcomes`.

> **NOTE**: applyRecovery is **not** called in this phase (will be added in Phase 8). Primary failures just produce failure Outcome without retry.

### Step 6.2 — Unit test `journey/executor_test.go` (happy path)

- [x] `TestExecute_NilArgs_ReturnsError` — Execute(nil, plan) and Execute(ctx, nil).
- [x] `TestExecute_Sequential_HappyPath` — 3-step journey, mockSynth captures BeginSpan calls in order.
- [x] `TestExecute_Parallel_HappyPath` — 2-branch parallel, both branches execute.
- [x] `TestExecute_CtxCancel_StopsWithin10ms` — cancel ctx during long sleep, verify return < 10ms + ErrorType "context_canceled".
- [x] `TestExecute_PanicInSynth_RecoversAndReturns` — mockSynth that panics in BeginSpan → Execute returns *ExecuteError{Kind: "internal"}.
- [x] `TestExecute_FaultCrash_FailureOutcome` — FaultCrash on a service → Outcome.Success=false, ErrorType="crashed".
- [x] `TestExecute_FaultDisconnect_FailureOutcome` — FaultDisconnect on an edge → ErrorType="connection_refused".
- [x] `TestExecute_FaultErrorRate_PrimaryFailureForced` — error_rate=1.0 → primary fails.
- [x] Create `journey/helpers_test.go` with `mockSynth`, `newTestSchema`, `newTestOverlay`, `assertOutcomeMatches` helpers per NFR-D §9.1.

### Phase 6 commit

- [x] `git add journey/executor.go journey/executor_test.go journey/helpers_test.go && git commit -m "feat(journey): add Executor with sequential/parallel dispatch, ctx cancel, and panic recovery"`

---

## Phase 7 — U3 Coordination: Extend synth.Outcome with Cascaded Marker
**Recommended agent**: Codex CLI (or Cursor batch — small, targeted edit).

> This phase patches the synth/ package to add a Cascaded bool field on Outcome and have the Synthesizer emit synth.cascaded=true attribute when set. Required by Phase 8 cascade span emission.

### Step 7.1 — Patch `synth/interface.go`

- [x] Add `Cascaded bool` field to `synth.Outcome` struct. Position it after EndTime for logical grouping (or at the end if EndTime is the convention).
- [x] Update GoDoc on Outcome.Cascaded to explain semantics: "Cascaded is set by the caller (typically the Journey Engine) to indicate that this Outcome represents a child step forced to skip execution by an upstream failure. The Synthesizer emits the `synth.cascaded=true` attribute when this is true."

### Step 7.2 — Patch `synth/synthesizer.go`

- [x] In the finishFn closure (returned by BeginSpan), after the existing SetAttributes call, check `outcome.Cascaded` and if true call `span.SetAttributes(attribute.Bool("synth.cascaded", true))`.
- [x] Add `"synth.cascaded"` to `synth/attributes.go` allowedAttrKeys map.

### Step 7.3 — Patch tests

- [x] Add `TestFinishFn_CascadedAttribute` to `synth/synthesizer_test.go` — finish with Outcome.Cascaded=true → span attribute synth.cascaded=true present.
- [x] Verify existing tests still pass.

### Step 7.4 — Update U3 FD/NFR-D docs

- [x] Update `aidlc-docs/construction/u3-synth/functional-design/domain-entities.md` §1.2 to add Cascaded field. Add a note referencing U2 coordination.
- [x] Update `aidlc-docs/construction/u3-synth/nfr-design/nfr-design-patterns.md` (or logical-components.md) to mention synth.cascaded handling in finishFn.

### Phase 7 commit

- [x] `git add synth/ aidlc-docs/construction/u3-synth/ && git commit -m "feat(synth): add Outcome.Cascaded marker for journey engine integration"`

---

## Phase 8 — Recovery + Executor Complete (LC-4 + LC-3 part 2)
**Recommended agent**: Codex CLI.

### Step 8.1 — Create `journey/recovery.go`

- [x] Implement `(*engineImpl).applyRecovery(ctx, node, primary Outcome) Outcome` per NFR-D §4.4:
  - For each `policy.Fallback`, call `executeFallback`, append to FallbackAttempts, return on success with FallbackUsed
  - On exhaustion, switch on OnExhausted (Propagate / ReturnDefault / SucceedSilently)
- [x] Implement `(*engineImpl).executeFallback(ctx, fromNode, fbEdge) Outcome`:
  - Construct synthetic fallback Node from fbEdge
  - Call `executeNode(ctx, fbNode, nil)` (fallback execution is independent, no parent cascade)

### Step 8.2 — Update `journey/executor.go`

- [x] In `executeNode`, after children traversal and primary-failure determination, if primary failed and node.Edge != nil and node.Edge.OnFailure != nil → call `applyRecovery` and use returned Outcome.
- [x] Implement `(*engineImpl).executeCascade(ctx, node, parent) Outcome`:
  - Build SpanInput, call BeginSpan + finishFn with `outcome.Cascaded=true` (requires Phase 7)
  - Skip Sleep
  - Skip children
  - Return Outcome with Success=false, Cascaded=true, Latency near zero, ErrorType=parent.ErrorType
- [x] Replace placeholder cascade handling in Phase 6 with call to `executeCascade`.

### Step 8.3 — Add `journey/recovery_test.go`

- [x] `TestApplyRecovery_FirstFallbackSucceeds` — primary fails, fallback[0] succeeds → Outcome.Success=true, FallbackUsed=fallback[0], FallbackAttempts=[].
- [x] `TestApplyRecovery_AllFallbacksFail_Propagate` — all fail + OnExhausted=Propagate → Outcome.Success=false, FallbackAttempts=all.
- [x] `TestApplyRecovery_AllFallbacksFail_ReturnDefault` — Outcome.Success=true, DefaultUsed=true.
- [x] `TestApplyRecovery_AllFallbacksFail_SucceedSilently` — Outcome.Success=true, SilentlySucceeded=true.
- [x] `TestExecute_CascadeChildSpan_EmittedWithAttribute` — parent crashes, child cascade span emitted with synth.cascaded=true attr (verify via mockSynth that records BeginSpan + finishFn args, including outcome.Cascaded).
- [x] `TestExecute_CascadeChildSpan_NearZeroDuration` — cascade child span Latency ≈ 0.
- [x] All tests call `t.Parallel()`.

### Phase 8 commit

- [x] `git add journey/recovery.go journey/executor.go journey/recovery_test.go journey/executor_test.go && git commit -m "feat(journey): add recovery flow with OnExhausted modes and cascade span emission"`

---

## Phase 9 — Documentation (LC-0 + doc_test.go)
**Recommended agent**: Codex CLI.

### Step 9.1 — Replace `journey/doc.go` placeholder

- [x] Full package doc per NFR-D LC-0.

### Step 9.2 — Create `journey/doc_test.go`

- [x] `ExampleNewEngine` — construct engine + ListJourneys.
- [x] `ExampleEngine_BuildPlan` — get a Plan from a journey name.
- [x] `ExampleEngine_Execute` — full Execute flow with a real (or stub) Synthesizer.
- [x] All Examples must compile and pass.

### Phase 9 commit

- [x] `git add journey/doc.go journey/doc_test.go && git commit -m "docs(journey): add package documentation and Example functions"`

---

## Phase 10 — U7 Generator Additions
**Recommended agent**: Codex CLI (or Cursor batch).

### Step 10.1 — Add `testutil/generators/journey_inputs.go`

- [x] Implement `ValidPlan(opts ...PlanOption) *rapid.Generator[*journey.Plan]` per FD §6.3:
  - Build Plans via topology generators + Engine.BuildPlan (or directly construct Node trees with valid Service/Operation pointers)
  - depth ≤ 5, breadth ≤ 4
- [x] Implement `AnyPlan(opts ...PlanOption) *rapid.Generator[*journey.Plan]` with relaxed invariants.
- [x] Implement `ValidNode(opts ...NodeOption) *rapid.Generator[*journey.Node]`.
- [x] Implement `AnyNode(opts ...NodeOption) *rapid.Generator[*journey.Node]`.
- [x] Implement `ValidEngineOutcome(opts ...OutcomeOption) *rapid.Generator[journey.Outcome]`:
  - enforce FD §1.5 invariants (Success ↔ ErrorType=="", Cascaded → Latency ≈ 0, DefaultUsed/SilentlySucceeded → Success=true, ErrorType ∈ AllowedErrorTypes ∪ {""})
- [x] Implement `AnyEngineOutcome(opts ...OutcomeOption) *rapid.Generator[journey.Outcome]` with relaxed invariants for shrinking.
- [x] Each generator follows existing U7 generator style.

### Step 10.2 — Add `testutil/generators/journey_inputs_test.go`

- [x] Property tests verifying valid generators produce only invariant-respecting values.

### Phase 10 commit

- [x] `git add testutil/generators/journey_inputs.go testutil/generators/journey_inputs_test.go && git commit -m "feat(testutil): add journey IO generators for U2 PBT"`

---

## Phase 11 — PBT (TP-U2-1 through TP-U2-5)
**Recommended agent**: Codex CLI.

### Step 11.1 — Create `journey/pbt_test.go`

- [x] Un-skip Phase 3 PBT tests (`TestBuildPlan_Idempotent_Property`, `TestBuildPlan_AllOpsVisited_Property`) and import `testutil/generators`.
- [x] `TestExecute_OutcomeCascadeConditional_Property` (TP-U2-3) — draw Schema with cascade-triggering faults, Execute, walk Outcomes via mockSynth call log, assert: `outcome.Cascaded ⇒ outcome.Success == false ∧ outcome.Latency ≈ 0`.
- [x] `TestExecute_OutcomeErrorTypeAllowed_Property` (TP-U2-4) — draw any Schema + faults, Execute, every Outcome.ErrorType is empty OR ∈ AllowedErrorTypes.
- [x] `TestExecute_TimeMonotonic_Property` (TP-U2-5) — draw Schema, Execute, walk span chain via mockSynth, assert child.StartTime ≥ parent.StartTime; for each node, finishFn EndTime ≥ BeginSpan StartTime.
- [x] All tests call `t.Parallel()`.

> **NOTE**: `testutil/generators` must export the underlying `ValidSchema` already (from U1). If TP-U2-1/TP-U2-2 cannot use `ValidPlan` directly (e.g. ValidPlan generates standalone Plans not tied to a Schema), they can use ValidSchema + BuildPlan(name) for direct verification.

### Phase 11 commit

- [x] `git add journey/pbt_test.go && git commit -m "test(journey): add PBT for TP-U2-1..5"`

---

## Phase 12 — Benchmark
**Recommended agent**: Codex CLI.

### Step 12.1 — Create `journey/bench_test.go`

- [x] `BenchmarkBuildPlan_Typical` — fixed Schema with 1 journey of depth 5 / 15 operations.
- [x] `BenchmarkExecute_PureOverhead` — execute typical journey with mockSynth and effectiveLatency=0 (sleep skip). Measure per-step overhead.
- [x] `BenchmarkListJourneys` — Engine with 5 journeys.
- [x] All use `b.ReportAllocs()`.
- [x] Verify locally: BuildPlan < 1ms / op, per-step pure overhead < 50µs.
- [x] If a benchmark misses budget, document in code-generation-summary.md and consider Q1 fallback (per-VU rand or rand/v2 global).

### Phase 12 commit

- [x] `git add journey/bench_test.go && git commit -m "test(journey): add benchmarks for BuildPlan and Execute pure overhead"`

---

## Phase 13 — Integration Test Harness
**Recommended agent**: Codex CLI.

> Aligned with U3/U4 pattern. `-tags=integration` only.

### Step 13.1 — Create `journey/testdata/collector-config.yaml`

- [x] OTLP/gRPC receiver + file_exporter to `/var/log/otel/{traces,metrics,logs}.json`.

### Step 13.2 — Create `journey/testdata/docker-compose.yaml`

- [x] Single collector service, mount config + output dir, expose 4317/4318. Reuse the pinned image tag from U3/U4 for consistency.

### Step 13.3 — Create `journey/integration/helpers.go`

- [x] `StartCollector(t)` / `ReadCollectorTraces` / `BuildEngine(t, schema, overlay)` — wires up real exporter Pipeline + real synth.Synthesizer + Engine.

### Step 13.4 — Create `journey/integration/integration_test.go`

- [x] `//go:build integration` at top.
- [x] `TestIntegration_Sequential_Correlated` — simple A→B→C journey, verify all 3 spans share trace_id.
- [x] `TestIntegration_CascadePropagation` — A→B→C journey with FaultCrash on B (Probability=1.0), no OnFailure on edge A→B → verify:
  - 3 spans emitted with same trace_id
  - Span B has Status=Error, ErrorType="crashed"
  - Span C has Status=Error, attribute `synth.cascaded=true`
- [x] `TestIntegration_Recovery_FallbackUsed` — primary fails (FaultErrorRateOverride=1.0), fallback succeeds → primary span Status=Error, fallback span Status=Ok, parent Outcome reflects FallbackUsed.

### Step 13.5 — Create `journey/integration/README.md`

- [x] Document Docker requirement + invocation.

### Phase 13 commit

- [x] `git add journey/integration/ journey/testdata/ && git commit -m "test(journey): add integration test harness with cascade and recovery verification"`

---

## Phase 14 — Final Wrap & DoD Verification
**Recommended agent**: Codex CLI.

### Step 14.1 — Run full suite

- [ ] `go build ./...` succeeds.
- [ ] `go vet ./journey/...` clean.
- [ ] `go test -race -count=1 ./...` passes.
- [ ] `go test -cover ./journey/...` ≥ 80%.
- [ ] `go test -bench=. -benchmem ./journey/...` shows BuildPlan < 1ms, per-step overhead < 50µs.
- [ ] `golangci-lint run ./journey/...` passes.
- [ ] `go test -tags=integration ./journey/integration/...` passes (with Docker).

### Step 14.2 — Create `aidlc-docs/construction/u2-journey/code/code-generation-summary.md`

- [ ] File list with line counts.
- [ ] Verification results (coverage %, bench numbers).
- [ ] Deviations (if any) — especially around U1 FaultOverlay actual API adaptation, Edge.LatencyDist sampling parameters, U3 Outcome.Cascaded extension.
- [ ] Recent commits (`git log --oneline | head -20`).

### Step 14.3 — Mark all plan checkboxes [x]

- [ ] Walk back through this plan; verify every `[ ]` is `[x]`.

### Step 14.4 — Update `aidlc-docs/aidlc-state.md`

- [ ] Mark U2 complete. Set Current Unit to U5 (k6 JS Module).

### Phase 14 commit

- [ ] `git add aidlc-docs/ && git commit -m "chore(u2-journey): finalize code-generation-summary and checkbox state"`

---

## Anti-patterns to AVOID during implementation

(Per NFR-D `nfr-design-patterns.md` §13)

- ❌ Explicit-stack executeNode (use direct recursion)
- ❌ Goroutine-per-node CSP model
- ❌ Per-step defer recover (use two-tier only: Execute top + parallel children)
- ❌ Generic `overlay.Lookup(target)` interface (use the actual 3-method U1 API)
- ❌ `overlay.LookupBaseLatency` consolidation (Edge.LatencyDist is the source)
- ❌ Outcome builder helper explosion (use local var)
- ❌ Mutable shared Outcome across goroutines
- ❌ Skipping cascade child span emission
- ❌ Adopting stateful PBT (PBT-06)
- ❌ Splitting helpers_test.go into mocks/fixtures/asserts
- ❌ Splitting executor.go by concern (executor_seq / executor_parallel / executor_cascade)
- ❌ Direct OTel SDK import (always go through synth)
- ❌ math/rand (v1) — use rand/v2
- ❌ golang.org/x/sync/errgroup — sync.WaitGroup is sufficient

---

## Notes for the implementing agent

1. **U1 FaultOverlay reality**: Use `NodeFaults`, `OperationFaults`, `EdgeFaults` (each returning `[]FaultSpec`). Do NOT invent new methods on FaultOverlay.
2. **U1 Edge.LatencyDist reality**: Use `edge.Latency.{Distribution, P50, P95}`. There is no Operation.Latency field; entry nodes (Edge==nil) use the package-local `defaultEntryLatency = 10 * time.Millisecond`.
3. **U1 FaultSpec.Severity shape**: Inspect actual U1 source (`topology/types.go`) for FaultSpec struct. If Severity has Delay/Multiplier/Rate fields, use them in sampleInflation / foldFaults. If shape differs from FD/NFR-D expectations, adapt (but log as deviation in code-generation-summary.md).
4. **U3 Outcome extension (Phase 7)**: Carefully patch synth/ in a separate phase. Tests on the synth side must continue passing. Update U3 docs to reflect the new field.
5. **Conventional Commits**: After each Phase, propose a commit per the per-Phase template. Include `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
6. **Sandbox mode**: This run uses `--sandbox danger-full-access` per `scripts/run-codex.sh u2`.
7. **Test parallelism**: All journey unit tests can run with `t.Parallel()` (Engine is shared-immutable after construction). Integration tests within the same test function may share a Collector and run serially.
8. **mockSynth thread-safety**: Use sync.Mutex around the call log slices (test code; not perf-critical).
9. **Capacity caveat**: U2 is the most complex unit. If Codex hits capacity mid-run, the natural split point is after Phase 8 (Executor complete) — Phases 9-14 can be picked up in a fresh session.
