# U7 (testutil/generators) — Code Generation Plan

> **This file is the Single Source of Truth (SSOT) for the U7 implementation.**
>
> **Audience**: Codex CLI (`gpt-5.5 xhigh`) for autonomous batch execution + Cursor Composer 2.5 for interactive editing. Read `AGENTS.md` for role boundaries before starting.
>
> **Execution model**: Work through the checkboxes top-to-bottom. Mark each `[ ]` → `[x]` immediately upon completion of that step. Do not re-order or skip without updating this plan first.
>
> **Source artifacts to reference while implementing**:
> - FD: `aidlc-docs/construction/u7-testutil/functional-design/{business-logic-model,business-rules,domain-entities}.md`
> - NFR-R: `aidlc-docs/construction/u7-testutil/nfr-requirements/{nfr-requirements,tech-stack-decisions}.md`
> - NFR-D: `aidlc-docs/construction/u7-testutil/nfr-design/{nfr-design-patterns,logical-components}.md`
> - Application Design (types): `aidlc-docs/inception/application-design/component-methods.md`
> - PBT rules: `.aidlc-rule-details/extensions/testing/property-based/property-based-testing.md`
> - Agent contract: `AGENTS.md`
> - Shared memory: `.agent-memory/MEMORY.md`

---

## Unit Context

- **Unit ID**: U7
- **Purpose**: PBT (Property-Based Testing) domain generators for the xk6-otel-gen project
- **Workspace root**: `/home/ymotongpoo/repos/xk6-otel-gen/`
- **Go module path**: `github.com/ymotongpoo/xk6-otel-gen`
- **Construction order**: U7 is **first** (per `unit-of-work-dependency.md`). U7 includes scaffolding `topology/` for the subsequent U1 unit (P-SKEL-1..4).
- **PBT requirements satisfied by this unit**:
  - **PBT-09** (Framework Selection) — `pgregory.net/rapid` registered in `go.mod`
  - **PBT-07** (Generator Quality) — domain-specific, range-realistic, atomic + composed
  - **PBT-08** (Shrinking & Reproducibility) — rapid defaults preserved
- **NFR DoD** (from `nfr-requirements.md` §5):
  - `go build ./...` succeeds
  - `go test -race ./testutil/generators/...` passes
  - `go test -cover ./testutil/generators/...` shows ≥ 80% coverage
  - `BenchmarkValidSchemaDraw` runs at ≤ 1 ms/draw target (NFR-U7-6)
  - All U7 test functions call `t.Parallel()`
  - No package-level mutable state
  - All public identifiers have GoDoc
- **Dependencies on other units**: none (U7 is first). U7 *scaffolds* `topology/` but does not depend on its methods.

---

## Phase 0 — Pre-U1 Topology Type Skeleton

> This phase exists because U7 is built **before** U1 (Topology), yet U7 generators need `topology.*` types. We write the **type definitions only** (no methods) so that U1 Code Generation later fills in the methods (`Parse`, `Validate`, `MarshalYAML`, `Equal`, etc.).
>
> Reference: `nfr-design-patterns.md` §6 (P-SKEL-1..4), `component-methods.md` C1.

### Step 0.1 — Initialize Go module (if not done)

- [x] From workspace root, verify `go.mod` exists. If not, run `go mod init github.com/ymotongpoo/xk6-otel-gen`.
- [x] Add Go directive: `go 1.23` (or the latest stable on your machine; record the value used in audit.md).
- [x] Run `go mod tidy` (will be a no-op if no source files yet).

### Step 0.2 — Create `topology/` package directory

- [x] Create directory: `topology/` at workspace root.

### Step 0.3 — Write `topology/doc.go`

- [x] Create `topology/doc.go` with package-level documentation:
  ```go
  // Package topology defines the schema and types for declarative
  // microservice topologies consumed by xk6-otel-gen.
  //
  // NOTE: This package was scaffolded during U7 (testutil/generators)
  // Code Generation. Method implementations (Parse, ParseFile, Validate,
  // ApplyFaults, Equal, MarshalYAML, etc.) are deferred to U1 (topology)
  // Code Generation; for now they are panic stubs marked
  // AUTOGEN-MARKER-U1. See aidlc-docs/inception/application-design/
  // component-methods.md §C1 for the canonical type definitions.
  package topology
  ```

### Step 0.4 — Write `topology/enums.go`

- [x] Create `topology/enums.go` with the enum types defined in `component-methods.md`:
  - `ServiceKind` (int-based with constants `KindApplication`, `KindDatabase`, `KindExternalAPI`, `KindCache`, `KindQueue`)
  - `Protocol` (constants `ProtocolHTTP`, `ProtocolGRPC`, `ProtocolMessaging`)
  - `ExhaustedAction` (constants `ExhaustedPropagate`, `ExhaustedReturnDefault`, `ExhaustedSucceedSilently`)
  - `FaultKind` (constants `FaultLatencyInflation`, `FaultErrorRateOverride`, `FaultDisconnect`, `FaultCrash`)
  - `TargetKind` (constants `TargetNode`, `TargetOperation`, `TargetEdge`)
  - `BackoffPolicy` (constants `BackoffExponential`, `BackoffLinear`, `BackoffConstant`)
- [x] Each enum should have a `String() string` method (use `stringer`-style switch).
- [x] GoDoc comment for each type.

### Step 0.5 — Write `topology/types.go`

- [x] Create `topology/types.go` containing **exactly** the type definitions from `component-methods.md` §C1:
  - `ServiceID` (`type ServiceID string`)
  - `Schema` (struct with `Services map[ServiceID]*Service`, `Journeys map[string]*Journey`, `Faults []FaultSpec`)
  - `Service` (with `Name ServiceID`, `Kind ServiceKind`, `Replicas int`, `Language string`, `Framework string`, `Version string`, `Operations map[string]*Operation`)
  - `Operation` (with `Name string`, `Service *Service`, `Calls []*CallNode`)
  - `CallNode` (with `Edge *Edge`, `Parallel []*CallNode` — variant)
  - `Edge` (with `From *Operation`, `To *Operation`, `Protocol`, `Latency LatencyDist`, `ErrorRate`, `Timeout`, `Retries`, `RetryBackoff`, `OnFailure *RecoveryPolicy`)
  - `LatencyDist` (struct — define minimally: `Distribution string`, `P50 time.Duration`, `P95 time.Duration`)
  - `RecoveryPolicy` (with `Fallback []*Edge`, `OnExhausted ExhaustedAction`, `DefaultResponse map[string]any`)
  - `Journey` (with `Name string`, `Steps []*Step`, `Weight float64`)
  - `Step` (with `Op *Operation`, `Parallel []*Step`)
  - `FaultTarget` (with `Kind TargetKind`, `Service *Service`, `Operation *Operation`, `Edge *Edge`)
  - `FaultSpec` (with `Target FaultTarget`, `Kind FaultKind`, `Severity SeverityParams`)
  - `SeverityParams` (struct — define minimally: `Probability float64`, `Multiplier float64`, `Add time.Duration`, `Value float64`; not all fields used by all FaultKind)
  - `FaultOverlay` (opaque — empty struct with an unexported `lookup` field can suffice; methods will be added by U1)
- [x] Add `// AUTOGEN-MARKER-U1` comment at the top of the file.
- [x] Use `gopkg.in/yaml.v3` struct tags (`yaml:"name"`) on fields that will be YAML-marshaled. NOTE: The `Parse`/`MarshalYAML` actual handling of `*Service`/`*Operation` pointer ↔ name string conversion is U1's responsibility; in this skeleton, just put the struct tags for the natural field names (e.g., `yaml:"name"`, `yaml:"protocol"`).

### Step 0.6 — Write top-level function stubs

- [x] Add to `topology/types.go` (or a new `topology/stubs.go`):
  ```go
  // Parse decodes a topology YAML from r.
  // AUTOGEN-MARKER-U1: implementation deferred to U1 Code Generation.
  func Parse(r io.Reader) (*Schema, error) {
      panic("topology.Parse: not yet implemented (U1 deferred)")
  }

  // ParseFile is a thin wrapper around Parse for filesystem paths.
  // AUTOGEN-MARKER-U1: implementation deferred to U1 Code Generation.
  func ParseFile(path string) (*Schema, error) {
      panic("topology.ParseFile: not yet implemented (U1 deferred)")
  }

  // Validate checks structural invariants of the Schema.
  // AUTOGEN-MARKER-U1: implementation deferred to U1 Code Generation.
  func Validate(s *Schema) error {
      panic("topology.Validate: not yet implemented (U1 deferred)")
  }

  // Equal compares two schemas by identifier-based deep equality.
  // NOTE: U7 needs a minimal Equal for PBT round-trip checks. This skeleton
  // provides a best-effort implementation that may be replaced by U1.
  func Equal(a, b *Schema) bool {
      // TODO(u1): implement identifier-based deep comparison
      // For U7's purposes, this is unused; the round-trip property is checked
      // by U1's own tests once Parse/MarshalYAML exist.
      return a == b
  }
  ```
- [x] Add similar stubs for `(*Schema).FindServiceByName`, `(*Schema).JourneyNames`, `(*Schema).ApplyFaults`, `(*Schema).ExportJSONSchema`, `(*Schema).MarshalYAML`. Each panics with `not yet implemented (U1 deferred)`.

### Step 0.7 — Verify topology builds

- [x] Run `go build ./topology/...`. Must succeed (panic stubs are valid Go).
- [x] Run `go vet ./topology/...`. No warnings.
- [x] **Acceptance**: `topology/` compiles cleanly, all public identifiers have GoDoc, AUTOGEN-MARKER-U1 comments are visible in `types.go` and `stubs.go`.

---

## Phase 1 — U7 Logical Components Implementation

> Reference: `logical-components.md` §2 (LC-0..LC-5), `nfr-design-patterns.md`.

### Step 1.1 — Create `testutil/generators/` directory and `doc.go` (LC-0)

- [ ] Create directory: `testutil/generators/` at workspace root.
- [ ] Create `testutil/generators/doc.go`. Copy the package-level documentation from `logical-components.md` §2 [LC-0].

### Step 1.2 — Implement `options.go` (LC-1)

- [ ] Create `testutil/generators/options.go`.
- [ ] Declare:
  - `type SchemaOption func(*schemaOptions)`
  - `type ServiceOption func(*serviceOptions)`
  - unexported `schemaOptions` struct with fields: `maxServices`, `maxOpsPerService`, `maxCallsPerOp`, `maxFaults`, `biasValid`
  - unexported `serviceOptions` struct with fields: `maxOpsPerService`, `fixedKind *topology.ServiceKind`
  - `defaultSchemaOptions() schemaOptions` returning struct literal with defaults from `business-rules.md` §3
  - `defaultServiceOptions() serviceOptions`
- [ ] Implement option constructors:
  - `MaxServices(n int) SchemaOption` (clamp n ≥ 1)
  - `MaxOpsPerService(n int) SchemaOption` (clamp n ≥ 1)
  - `MaxCallsPerOp(n int) SchemaOption` (clamp n ≥ 0)
  - `MaxFaults(n int) SchemaOption` (clamp n ≥ 0)
  - `BiasValid(p float64) SchemaOption` (clamp p ∈ [0, 1])
  - `WithKind(k topology.ServiceKind) ServiceOption`
- [ ] Each option function has a GoDoc one-liner.
- [ ] All Option constructors must be **side-effect free** apart from the closure they return.

### Step 1.3 — Implement `primitives.go` (LC-2)

- [ ] Create `testutil/generators/primitives.go`.
- [ ] Define the helper struct: `type LatencyPair struct { P50, P95 time.Duration }`.
- [ ] Implement each primitive generator listed in `logical-components.md` §2 [LC-2]:
  - `ValidServiceID()` — `rapid.StringMatching(`^[a-z][a-z0-9-]{2,30}$`)` cast to `topology.ServiceID`
  - `AnyServiceID()` — wider regex or pure `rapid.String()` cast (may include uppercase / empty / over-length)
  - `ValidOperationName()` — UTF-8 string 1–120 chars; mix of plain words, HTTP-path-like (`GET /products/{id}`), RPC method names
  - `AnyOperationName()` — may include empty or over-length strings
  - `ValidProbability()` — `rapid.Float64Range(0, 1)`
  - `AnyProbability()` — `rapid.Float64Range(-1, 2)` plus rare NaN / +Inf via `rapid.OneOf`
  - `ValidReplicaCount()` — `rapid.IntRange(1, 100)`
  - `AnyReplicaCount()` — `rapid.IntRange(-10, 1000)`
  - `ValidLatencyPair()` — draw p50 ∈ [1ms, 5s], then p95 ∈ [p50, 30s] (per R-DOM-1)
  - `AnyLatencyPair()` — may produce p95 < p50 or negative durations
  - `ValidTimeout()` — `rapid.IntRange(100, 60_000)` ms
  - `AnyTimeout()` — wider range including 0 / negative
  - `ValidServiceKind()` — `rapid.SampledFrom([]topology.ServiceKind{KindApplication, KindDatabase, KindExternalAPI, KindCache, KindQueue})`
  - `ValidProtocol()` — `rapid.SampledFrom([]topology.Protocol{ProtocolHTTP, ProtocolGRPC, ProtocolMessaging})`
- [ ] All return `*rapid.Generator[T]` built via `rapid.Custom` (P-PERF-1) where useful labels can be added with `Draw(t, "label")`.
- [ ] GoDoc one-liner per generator with a reference to the relevant rule (e.g., `// See business-rules.md §3.`).

### Step 1.4 — Implement `service.go` (LC-3)

- [ ] Create `testutil/generators/service.go`.
- [ ] Implement `ValidService(opts ...ServiceOption) *rapid.Generator[*topology.Service]` following the sketch in `logical-components.md` §2 [LC-3]:
  - Set Name from `ValidServiceID()`
  - Set Kind from `o.fixedKind` if non-nil, else `ValidServiceKind()`
  - Set Replicas from `ValidReplicaCount()`
  - Set `Operations` to a fresh map
  - Draw 1..`o.maxOpsPerService` operation names with `rapid.SliceOfNDistinct`
  - For each, create `*topology.Operation` with `Service: svc` back-pointer; leave `Calls` nil (caller — `ValidSchema` — fills these in)
- [ ] Implement `AnyService(opts ...ServiceOption) *rapid.Generator[*topology.Service]`:
  - Start from `ValidService` output but **may set Name from `AnyServiceID()` or omit/scramble the back-pointer with low probability** (use `rapid.Float64Range(0, 1)` to gate destructive paths)
- [ ] GoDoc for both functions.

### Step 1.5 — Implement `schema.go` (LC-4)

- [ ] Create `testutil/generators/schema.go`.
- [ ] Implement `ValidSchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema]` per `logical-components.md` §2 [LC-4]:
  - `buildServicesAndOperations(t, schema, o) []*topology.Operation` — generate services and their operations, return all operations in topological order (lexicographic by service-index, then operation-index)
  - `buildEdges(t, topoOrder, o)` — for each operation, draw 0..`o.maxCallsPerOp` target operations from `topoOrder[i+1:]` (strictly downstream, see P-PERF-2). Each becomes a single-Edge `CallNode`. With some probability (say 20%), wrap multiple calls in a `Parallel` group.
  - `buildJourneys(t, schema, topoOrder, o)` — draw 1..3 journeys; each is a sequence of 1..3 steps pointing to random operations (preferably ones with shallow downstream depth).
  - `buildFaults(t, schema, topoOrder, o)` — draw 0..`o.maxFaults` faults; each randomly targets an existing Service / Operation / Edge.
- [ ] Implement `AnySchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema]` per P-COMP-3:
  - Draw a `ValidSchema(opts...)` output as baseline
  - Draw a `Float64Range(0, 1)` and compare against `o.biasValid` (default 0.5). If below, return the valid schema unchanged.
  - Otherwise, pick a random mutator from `schemaMutators` (from `mutators.go`) and apply it.
- [ ] GoDoc for both functions.

### Step 1.6 — Implement `mutators.go` (LC-5)

- [ ] Create `testutil/generators/mutators.go`.
- [ ] Define `type mutator func(t *rapid.T, s *topology.Schema) *topology.Schema` and the package-level `var schemaMutators = []mutator{ ... }` containing the 8 mutators listed in `logical-components.md` §2 [LC-5].
- [ ] Implement each mutator:
  - `unresolveEdgeTarget` — pick a random Edge and set `Edge.To` to a freshly-allocated `*topology.Operation` not registered in `schema.Services`
  - `introduceCycle` — pick two operations A → B (existing edge) and add a reverse edge B → A
  - `misreferenceJourney` — pick a journey, set one Step.Op to a freshly-allocated unregistered Operation
  - `misreferenceFault` — pick a fault, swap its Target to a stale pointer
  - `dropServiceMap` — pick a Service that has incoming edges, `delete()` it from `schema.Services` map (other edges to it become orphan refs)
  - `breakBackPointer` — pick an Operation, change its `Service` back-pointer to another Service in the schema
  - `violateCallNodeVariant` — pick a `*CallNode` with non-nil Edge and also set `Parallel` to a non-empty slice (violates R-STR-7)
  - `misownFallback` — pick an Edge with non-nil OnFailure, change the first fallback's `From` to a different Operation
- [ ] Each mutator must operate on a **deep-copy** of the input schema to avoid test cross-contamination. Provide an unexported `cloneSchema(*topology.Schema) *topology.Schema` helper (use whatever depth is necessary — for these mutators, top-level map copies plus per-Operation copies are sufficient).
- [ ] GoDoc one-liner for each mutator referencing the R-STR-* rule it violates.

### Step 1.7 — Build verification

- [ ] Run `go build ./...`. Must succeed.
- [ ] Run `go vet ./...`. No warnings.
- [ ] **Acceptance**: All `testutil/generators/` files compile, no `// TODO(agent):` comments remain unless explicitly intentional with a paired audit.md note.

---

## Phase 2 — Tests

> Reference: `logical-components.md` §3 (LC-T1..T5), `business-rules.md` §10 (TP-U7-1..6), `nfr-design-patterns.md` §5 (P-CONC-2).
>
> **Every test function must call `t.Parallel()`** as its first statement (NFR-U7-4).

### Step 2.1 — Tests for primitives (`primitives_test.go`)

- [ ] Create `testutil/generators/primitives_test.go`.
- [ ] Add PBT for `ValidServiceID` matching `^[a-z][a-z0-9-]{2,30}$` (verify via `regexp`).
- [ ] Add PBT for `ValidLatencyPair` enforcing `p95 >= p50` (TP-U7-6, R-DOM-1).
- [ ] Add statistical PBT for `AnyServiceID`: 100 draws, expect at least one invalid (per R-A-3 / R-A-4).
- [ ] Add PBT for each enum primitive (`ValidServiceKind`, `ValidProtocol`) ensuring the returned value is in the enumerated set.
- [ ] Each test function calls `t.Parallel()`.
- [ ] Each test function uses `rapid.Check(t, func(t *rapid.T) { ... })`.

### Step 2.2 — Tests for options (`options_test.go`)

- [ ] Create `testutil/generators/options_test.go`.
- [ ] Add PBT for TP-U7-4: `ValidSchema(MaxServices(N))` always produces `len(schema.Services) <= N`.
- [ ] Add PBT for `MaxOpsPerService(N)` and `MaxCallsPerOp(N)` constraints.
- [ ] Add example-based tests for clamp behavior: `MaxServices(-5)` produces ≥ 1 services, `BiasValid(2.0)` is clamped to 1.0 (always valid output for `AnySchema`).

### Step 2.3 — Tests for service generator (`service_test.go`)

- [ ] Create `testutil/generators/service_test.go`.
- [ ] PBT: `ValidService` output satisfies R-STR-2 (every Operation's `Service` back-pointer matches the parent Service).
- [ ] PBT: `ValidService.Name` matches `ValidServiceID` regex.
- [ ] PBT: `ValidService.Operations` is non-empty.
- [ ] PBT: `ValidService(WithKind(KindDatabase))` always returns a Service whose `Kind == KindDatabase`.

### Step 2.4 — Tests for schema generator (`schema_test.go`)

- [ ] Create `testutil/generators/schema_test.go`.
- [ ] Add `TestValidSchema_StructuralInvariants` (PBT): verify R-STR-1, R-STR-2, R-STR-3, R-STR-7, R-STR-8 (the ones checkable without `topology.Validate`).
- [ ] Add `TestValidSchema_IsDAG` (PBT, TP-U7-2): topological sort the Operation graph and confirm no cycles. Implement an internal `isOperationDAG(s *topology.Schema) bool` test helper.
- [ ] Add `TestAnySchema_ContainsInvalid_Statistical` (TP-U7-3, statistical PBT): draw 100 schemas; at least one must violate at least one R-STR-* rule (use a meta-validator that checks structural rules without calling `topology.Validate` since it's a panic stub).
- [ ] Add `TestValidSchema_NotDegenerate_Statistical` (TP-U7-5): draw 20 schemas; ensure not all are byte-identical (verify by hashing serialized fingerprint).
- [ ] NOTE: TP-U7-1 (`topology.Validate(s) == nil`) **cannot run until U1 implements Validate**. Add `t.Skip("U1: topology.Validate not implemented yet")` placeholder in the body and a `TODO(u1):` comment.

### Step 2.5 — Test verification

- [ ] Run `go test -race ./testutil/generators/...`. Must pass.
- [ ] Run `go test -cover ./testutil/generators/...`. Coverage ≥ 80% (NFR-U7-5).
- [ ] If coverage < 80%, add additional unit tests for under-covered files (esp. `mutators.go` if needed — each mutator should have at least one example-based test).
- [ ] **Acceptance**: race-free, coverage ≥ 80%.

---

## Phase 3 — Benchmark

> Reference: `nfr-design-patterns.md` §1 (P-PERF-5), `nfr-requirements.md` NFR-U7-6.

### Step 3.1 — Implement `bench_test.go`

- [ ] Create `testutil/generators/bench_test.go`.
- [ ] Implement `BenchmarkValidSchemaDraw`:
  ```go
  func BenchmarkValidSchemaDraw(b *testing.B) {
      gen := ValidSchema()
      b.ReportAllocs()
      b.ResetTimer()
      for i := 0; i < b.N; i++ {
          _ = gen.Example()
      }
  }
  ```

### Step 3.2 — Run benchmark, record baseline

- [ ] Run `go test -bench=BenchmarkValidSchemaDraw -benchmem ./testutil/generators/...`.
- [ ] Append the result (ns/op and allocs/op) to `aidlc-docs/construction/u7-testutil/code/code-generation-summary.md` (created in Step 5.2). Verify target ≤ 1,000,000 ns/op (= 1 ms/draw) on commodity hardware.
- [ ] If above target, profile via `go test -bench=BenchmarkValidSchemaDraw -cpuprofile=cpu.prof ...` and add findings to `audit.md` under a new "Implementation-time Insight" entry. Do not aggressively refactor without a separate user-approved change.

---

## Phase 4 — Documentation & Examples

> Reference: `nfr-design-patterns.md` §4 (P-DOC-1, P-DOC-2).

### Step 4.1 — Example functions

- [ ] Add `ExampleValidSchema` (in `schema_test.go` or a separate `example_test.go`).
  - Uses `rapid.Check(&testing.T{}, func(t *rapid.T) { ... })`.
  - Demonstrates `MaxServices(3)`, `MaxOpsPerService(2)`.
  - No `// Output:` line (rapid example function has no deterministic output).
- [ ] Add `ExampleValidService` showing `WithKind(KindDatabase)`.
- [ ] Add `ExampleAnySchema` showing `BiasValid(0.0)` (always degraded).

### Step 4.2 — GoDoc completeness review

- [ ] Run `go doc -all ./testutil/generators/` and visually verify every public identifier has GoDoc.
- [ ] Run `go doc -all ./topology/` and verify every public type/function has GoDoc (AUTOGEN-MARKER-U1 comments are acceptable for stubs).

---

## Phase 5 — Final Verification & Definition of Done

> Reference: `AGENTS.md` §7, `nfr-requirements.md` §5.

### Step 5.1 — Full verification battery

- [ ] `go build ./...` — succeeds
- [ ] `go test -race -count=1 ./...` — passes (all tests, including the panic-stub-skipped ones)
- [ ] `go test -cover ./testutil/generators/...` — coverage ≥ 80%
- [ ] `golangci-lint run ./...` — no warnings (if golangci-lint is not installed locally, document in audit.md and let CI in Build and Test stage handle it)
- [ ] `go vet ./...` — no warnings
- [ ] No `TODO(agent):` comments remain unless paired with an `audit.md` entry

### Step 5.2 — Write Code Generation summary

- [ ] Create `aidlc-docs/construction/u7-testutil/code/code-generation-summary.md` with:
  - List of files created (paths)
  - Lines of code per file (rough)
  - `BenchmarkValidSchemaDraw` result (ns/op, B/op, allocs/op)
  - `go test -cover` percentage
  - Any deviations from the plan (with rationale)
  - List of `TODO(u1):` markers introduced and their location

### Step 5.3 — Mark all checkboxes [x]

- [ ] Mark all `[ ]` items in this document as `[x]` (including this one once everything above is done).
- [ ] Update `aidlc-docs/aidlc-state.md` Current Status to "U7 Code Generation complete, ready for U1".

### Step 5.4 — Audit log entry

- [ ] Append an entry to `aidlc-docs/audit.md` under a new section:
  ```
  ## U7 testutil — Code Generation Complete (by implementation agent)
  **Timestamp**: <ISO 8601 now>
  **Implementation agent**: <Codex CLI gpt-5.5 xhigh | Cursor Composer 2.5>
  **Files created**: <list>
  **Coverage**: <pct>%
  **BenchmarkValidSchemaDraw**: <ns/op>
  **Deviations from plan**: <list or "none">
  **TODO(u1) markers**: <list>
  ```

---

## Files Expected (final inventory)

```text
go.mod
go.sum
topology/
├── doc.go
├── enums.go
├── types.go
└── stubs.go            # (optional — can be folded into types.go)
testutil/
└── generators/
    ├── doc.go
    ├── options.go
    ├── primitives.go
    ├── service.go
    ├── schema.go
    ├── mutators.go
    ├── primitives_test.go
    ├── options_test.go
    ├── service_test.go
    ├── schema_test.go
    ├── example_test.go (optional — can be folded into other test files)
    └── bench_test.go
aidlc-docs/construction/u7-testutil/code/
└── code-generation-summary.md
```

Total estimated LOC for production code (non-test): ~650 lines (per `logical-components.md` §4).
Test code: ~400-500 lines.

---

## Boundaries (do NOT cross)

- Do NOT implement any methods in `topology/` beyond what is in this plan. `Parse`, `Validate`, `MarshalYAML` etc. remain panic stubs — they are U1's job.
- Do NOT modify `aidlc-docs/**` except as instructed (audit.md append, code-generation-summary.md create, this plan's checkboxes).
- Do NOT change `AGENTS.md`, `CLAUDE.md`, `.cursor/rules/**`, `.codex/**`, `.aidlc-rule-details/**`.
- Do NOT add dependencies other than `pgregory.net/rapid` and `gopkg.in/yaml.v3` (the latter only needed for struct tags on `topology` types; no runtime use in U7).
- Do NOT introduce package-level mutable state (NFR-U7-8 violation).
- If you discover an ambiguity or a need to deviate, **STOP and append a question to `audit.md`** under an "Implementation-time Question" heading. Do not silently resolve.

---

## Out of Scope (handled in later units / stages)

- U1 will implement `topology.Parse`, `Validate`, `MarshalYAML`, `ApplyFaults`, `Equal`, `ExportJSONSchema`, journey/fault override logic.
- U7's generators for U2-U6 types (`Plan`, `Outcome`, `MetricInput`, etc.) are added incrementally during each unit's FD/CG (Q8 of U7 FD).
- CI workflow integration (RAPID_SEED logging, coverage threshold gating) — Build and Test stage.
- Release / GoReleaser config — U8 (Samples & Distribution).
