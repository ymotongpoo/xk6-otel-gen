# U1 (topology) — Code Generation Plan

> **This file is the Single Source of Truth (SSOT) for the U1 implementation.**
>
> **Audience**: Codex CLI (`gpt-5.5 xhigh`) for autonomous batch execution + Cursor Composer 2.5 for both interactive editing AND batch editing via `agent -p "<prompt>"` (Cursor Agent CLI binary is `agent`). Read `AGENTS.md` for role boundaries and the agent-selection guideline before starting.
>
> **Recommended agent per Phase** (see AGENTS.md §2 "Codex と Cursor の使い分けガイドライン"):
> - **Phase 0-12 → Codex** (deep algorithmic reasoning, long-running, multi-file new construction). Run via `scripts/run-codex.sh u1` (or the older `scripts/run-codex-u7.sh` style for one-off).
> - **Phase 13 → Cursor batch** (pattern-conforming additive work; the 18 new generators follow established U7 generator style). Run via `scripts/run-cursor.sh u1 --phases 13`.
> - You MAY use either agent for any phase if you have reason; the recommendation is performance-based, not contractual.
>
> **Execution model**: Work through the checkboxes top-to-bottom. Mark each `[ ]` → `[x]` immediately upon completion of that step. Do not re-order or skip without updating this plan first.
>
> **Source artifacts to reference while implementing**:
> - FD: `aidlc-docs/construction/u1-topology/functional-design/{business-logic-model,business-rules,domain-entities}.md`
> - NFR-R: `aidlc-docs/construction/u1-topology/nfr-requirements/{nfr-requirements,tech-stack-decisions}.md`
> - NFR-D: `aidlc-docs/construction/u1-topology/nfr-design/{nfr-design-patterns,logical-components}.md`
> - Application Design (types): `aidlc-docs/inception/application-design/component-methods.md`
> - U7 's existing topology scaffold: `topology/{doc.go,enums.go,types.go,stubs.go}`
> - U7 's testutil/generators (will need 18 new generator functions added per FD §6)
> - PBT rules: `.aidlc-rule-details/extensions/testing/property-based/property-based-testing.md`
> - Agent contract: `AGENTS.md`
> - Shared memory: `.agent-memory/MEMORY.md`

---

## Unit Context

- **Unit ID**: U1
- **Purpose**: Replace U7's panic-stub methods in `topology/` with full implementations (Parse / ParseFile / Validate / MarshalYAML / Equal / ApplyFaults / ExportJSONSchema / FindServiceByName / JourneyNames / Lint)
- **Workspace root**: `/home/ymotongpoo/repos/xk6-otel-gen/`
- **Go module path**: `github.com/ymotongpoo/xk6-otel-gen`
- **Construction order position**: U7 done → **U1 (this)** → U4 → U3 → U2 → U5 → U6 → U8
- **PBT requirements satisfied by this unit**:
  - **PBT-01** (Property Identification) — 8 properties documented (TP-U1-1..8)
  - **PBT-02** (Round-trip) — TP-U1-1 (Parse↔Marshal), TP-U1-8 (JSON Schema)
  - **PBT-03** (Invariants) — TP-U1-2, TP-U1-3, TP-U1-4, TP-U1-5
  - **PBT-04** (Idempotency) — TP-U1-6 (Validate), TP-U1-7 (ApplyFaults)
- **NFR DoD** (from `nfr-requirements.md` §5):
  - `go build ./...` succeeds
  - `go test -race -count=1 ./topology/...` passes
  - `go test -cover ./topology/...` shows ≥ 80% coverage
  - `BenchmarkParse` runs at ≤ 10 ms / draw target on typical YAML
  - All U1 test functions call `t.Parallel()`
  - No package-level mutable state
  - All public identifiers have GoDoc
  - U7's previously-skipped `TestValidSchema_ValidatePlaceholder` (in `testutil/generators/schema_test.go`) is un-skipped and passing
- **Dependencies added by this unit**:
  - `gopkg.in/yaml.v3` (body)
  - `github.com/santhosh-tekuri/jsonschema/v5` (test-only)

---

## Phase 0 — Environment Setup
**Recommended agent**: Codex CLI (algorithmic care needed for go.mod toolchain choice and dep resolution).


> Bring the existing repository (last touched by U7) up to U1's requirements: Go 1.25, new deps, deleted stubs.

### Step 0.1 — Bump `go.mod` to Go 1.25

- [x] Run `go mod edit -go=1.25`.
- [x] If the local toolchain is older than 1.25, add `toolchain go1.25.x` line (use the latest available).
- [x] Run `go mod tidy`.
- [x] Verify: `head -3 go.mod` shows `go 1.25`.

### Step 0.2 — Add dependencies

- [x] Run `go get gopkg.in/yaml.v3@latest`.
- [x] Run `go get github.com/santhosh-tekuri/jsonschema/v5@latest`.
- [x] Run `go mod tidy`.
- [x] Verify: `go.mod` `require` section contains both packages.
- [x] Note: `jsonschema/v5` is test-only; if Go marks it as direct (since `_test.go` imports it), that's fine. Do NOT add it to body imports of any non-test file.

### Step 0.3 — Delete U7 's panic stubs

- [x] Delete file `topology/stubs.go`.
- [x] Run `go build ./topology/...` — **expected to FAIL** at this point, since `Parse` etc. no longer exist. This is OK; subsequent phases will reinstate them.

### Step 0.4 — Update `topology/doc.go`

- [x] Replace the existing U7 placeholder doc.go with the final package documentation per `logical-components.md` LC-0:
  - Package overview (3-4 paragraphs)
  - Explicit `IMMUTABILITY:` section per NFR-U1-5 (P-IMM-1)
  - Explicit `CONCURRENCY:` section per NFR-U1-6 (P-CONC-1)
  - `ERROR REPORTING:` brief on `errors.As` with `*ParseError` / `*ValidationError`
  - Remove the U7 `AUTOGEN-MARKER-U1` comment

### Step 0.5 — Build verification (expected partial failure is OK)

- [x] Run `go vet ./topology/...` — should pass (only types + enums + doc remain, no stale code).
- [x] **Acceptance**: `topology/` contains only doc.go, enums.go, types.go. All ready for additive implementation.

---

## Phase 1 — `errors.go` (LC-9)

> Other phases depend on these error types; build them first.

### Step 1.1 — Implement `*ParseError` and `*ValidationError`

- [x] Create `topology/errors.go` per `nfr-design-patterns.md` §2 P-ERR-1:
  - `type ParseError struct { Path, Message string; Inner error }` with `Error()` and `Unwrap()`
  - `type ValidationError struct { Path, Rule, Message string }` with `Error()`
  - GoDoc for both types

### Step 1.2 — Build check

- [x] `go build ./topology/...` — succeeds.
- [x] **Acceptance**: errors compile, no warnings.

---

## Phase 2 — `raw.go` (LC-1)

> Internal types used by Parse and MarshalYAML.

### Step 2.1 — Implement raw structs

- [x] Create `topology/raw.go` per `logical-components.md` LC-1:
  - `rawSchema`, `rawService`, `rawOperation`, `rawCallNode`, `rawCallTarget`, `rawJourney`, `rawStep`, `rawFault`, `rawRecoveryPolicy`, `rawLatencyDist`, `rawSeverity`
  - All unexported (lowercase prefix `raw*`)
  - Pointer fields (`*int`, `*float64`, `*time.Duration`) for nil-detection of YAML-omitted values
  - `yaml:"..."` struct tags matching the schema per `topology-yaml-schema.md`

### Step 2.2 — Build check

- [x] `go build ./topology/...` — succeeds.

---

## Phase 3 — `parse.go` (LC-2)

> Coverage: Parse, ParseFile, decodeRaw, buildSchema, resolveReferences + reference helpers.

### Step 3.1 — Top-level Parse / ParseFile + decodeRaw

- [x] Create `topology/parse.go`.
- [x] Implement `Parse(r io.Reader) (*Schema, error)` per `business-logic-model.md` §1 and NFR-D P-PERF-1, P-PERF-4:
  - `io.ReadAll(r)` (P-PERF-4)
  - Call `decodeRaw(bytes.NewReader(data), false)` (lax)
  - Call `buildSchema(raw)`
  - Call `resolveReferences(schema, raw)` — if error, return
  - Call `Validate(schema)` — if error, return
  - Return `*Schema, nil`
- [x] Implement `ParseFile(path string) (*Schema, error)` — `os.Open` + `defer Close` + delegate to `Parse`.
- [x] Implement `decodeRaw(r io.Reader, strict bool) (*rawSchema, error)` (P-PERF-1):
  - `yaml.NewDecoder(r)` + `dec.KnownFields(strict)`
  - On error: wrap in `*ParseError{Path: "<root>", Message: "yaml decode failed", Inner: err}`

### Step 3.2 — buildSchema (Phase 2a — typed objects + defaults)

- [x] Implement `buildSchema(raw *rawSchema) *Schema` per `business-logic-model.md` §1 "Phase 2a":
  - Create Service map with `make(map[ServiceID]*Service, len(raw.Services))` (P-PERF-2)
  - For each raw service: create `*Service` with defaults applied
  - For each raw operation: create `*Operation` with back-pointer to its Service (R-STR-2 invariant satisfied at construction)
- [x] Implement default helpers (top of `parse.go` or shared `helpers.go`):
  ```go
  func intDefault(p *int, def int) int
  func float64Default(p *float64, def float64) float64
  func durationDefault(p *time.Duration, def time.Duration) time.Duration
  ```
- [x] Implement enum parsers: `parseServiceKind`, `parseProtocol`, `parseBackoff`, `parseFaultKind`. Return zero value for unknown strings (validate.go will report invalid enum as error D-?, but parser stays lenient to defer YAML errors to Validate).
  - **Actually**: on unknown enum strings, return a sentinel "Invalid" value (e.g., `ServiceKind(-1)`). validate.go checks this.

### Step 3.3 — resolveReferences (Phase 2b — collect errors)

- [x] Implement `resolveReferences(schema *Schema, raw *rawSchema) error`:
  - Iterate raw services and resolve each operation's `Calls` via `resolveCallNode`
  - Iterate raw journeys and resolve each step's operation via `resolveStep`
  - Iterate raw faults and resolve targets via `resolveFaultTarget`
  - Collect errors into `[]error`, return `errors.Join(errs...)`
- [x] Implement `resolveCallNode(schema, owningSvc, owningOp, rc, path string)`:
  - Variant check (hasTo XOR hasParallel) → R-STR-7
  - If Parallel: recursively call resolveCallNode for each child
  - If To: lookup target Operation, build Edge with defaults via float64Default etc., recurse into RecoveryPolicy
- [x] Implement `resolveStep(schema, rs, path string)`:
  - Variant check (Op or Parallel)
  - If Op set: lookup Operation via service+operation strings
  - If Parallel: recurse
- [x] Implement `resolveFaultTarget(schema, spec, path string)`:
  - Parse the `target` string: `node:<svc>` | `operation:<svc>.<op>` | `edge:<svc>.<op>-><svc>.<op>`
  - Return `FaultTarget{Kind, Service|Operation|Edge}` with resolved pointer
  - For `edge:` target, search the schema for matching `*Edge` (linear scan acceptable for small schemas)
- [x] Implement `resolveRecoveryPolicy(schema, owningOp, rp, path string)`:
  - Each fallback edge's `From` is set to `owningOp` (R-STR-8 invariant satisfied at construction)
  - Parse `OnExhausted` enum
- [x] Implement `lookupOperation(schema, svcName, opName string)`:
  - Return `&ParseError{Path: path, Message: ...}` on miss

### Step 3.4 — Build verification (still expected to fail without Validate / MarshalYAML)

- [x] `go vet ./topology/parse.go` — should compile in isolation if Validate is declared. We'll need to stub Validate temporarily:
  - Add a temporary `func Validate(s *Schema) error { return nil }` in parse.go — REMOVE in Phase 4 after validate.go is created.

---

## Phase 4 — `validate.go` (LC-3)

> Replace the temporary Validate stub with the real one. Implement 8 structural + 1 domain checks.

### Step 4.1 — Top-level Validate

- [x] Create `topology/validate.go`.
- [x] Remove the temporary `Validate` stub from `parse.go`.
- [x] Implement `Validate(s *Schema) error` per `nfr-design-patterns.md` §1 P-PERF-5:
  - Phase A (structural): call 8 validateXxx (R-STR-1..8) in order, collect errors
  - Phase B (domain): call `validateDomainRanges` (D-1..D-14), collect errors
  - Return `errors.Join(errs...)`

### Step 4.2 — Structural validators

- [x] Implement `validateMapKeyConsistency(s)` — R-STR-1: `s.Services[id].Name == id`.
- [x] Implement `validateBackPointers(s)` — R-STR-2: `op.Service == svc && svc.Operations[op.Name] == op`.
- [x] Implement `validateNoOrphanReferences(s)` — R-STR-3: every `Edge.From` and `Edge.To` is in schema. (Note: resolveReferences should already prevent this, but Validate re-checks as a safety net.)
- [x] Implement `validateDAG(s)` per P-VAL-DAG (Kahn's algorithm):
  - Build map[*Operation]int of in-degrees
  - BFS from in-degree-0 operations
  - If visited count < total ops, report cycle with affected operation names sorted alphabetically
- [x] Implement `validateJourneyReachability(s)` — R-STR-5: each `step.Op != nil` for non-Parallel steps; recurse into Parallel.
- [x] Implement `validateFaultTargets(s)` — R-STR-6: each `FaultSpec.Target` has exactly one of Service/Operation/Edge set per Kind.
- [x] Implement `validateCallNodeVariants(s)` — R-STR-7: each `CallNode` has exactly one of Edge or Parallel set.
- [x] Implement `validateRecoveryPolicyOwnership(s)` — R-STR-8: each `fallback[i].From == 親 Edge.From`.

### Step 4.3 — Domain validators

- [x] Implement `validateDomainRanges(s)` per `business-rules.md` §3.2 (D-1..D-14):
  - `Service.Replicas >= 1`
  - `Edge.ErrorRate ∈ [0,1]`
  - `Edge.Timeout >= 0`
  - `Edge.Retries >= 0`
  - `Edge.Latency.P95 >= Edge.Latency.P50 >= 0`
  - `LatencyDist.Distribution ∈ {constant, lognormal, normal, exponential}`
  - `FaultSpec.Severity.Probability ∈ [0,1]` (for kinds that use it)
  - `FaultSpec.Severity.Multiplier > 0` (for FaultLatencyInflation)
  - `Journey.Weight > 0`
  - `Journey.Steps` non-empty
  - `Service.Operations` non-empty
  - `Schema.Services` non-empty
  - `Schema.Journeys` non-empty

### Step 4.4 — Traversal helpers

- [x] Implement `forEachOutgoingEdge(s, fn)`, `forEachEdgeInCalls(calls, fn)`, `outgoingTargets(op)`, `identifyOp(op)` per `nfr-design-patterns.md` §4.

### Step 4.5 — Build check

- [x] `go build ./topology/...` — succeeds.

---

## Phase 5 — `marshal.go` (LC-4)

> `(*Schema).MarshalYAML` returning rawSchema with sorted order.

### Step 5.1 — Implement Schema-level Marshaler

- [x] Create `topology/marshal.go`.
- [x] Implement `(*Schema).MarshalYAML() (any, error)` per P-MARSHAL-1:
  - Build `*rawSchema` with services sorted by ServiceID, journeys by name, faults in declaration order.

### Step 5.2 — Helper functions

- [x] `sortedServiceIDs(m map[ServiceID]*Service) []ServiceID`
- [x] `sortedKeys[V any](m map[string]V) []string`
- [x] `marshalService(svc *Service) *rawService`
- [x] `marshalOperations(ops map[string]*Operation) []*rawOperation` (operations sorted by name, then call sequence preserved)
- [x] `marshalCallNodes(nodes []*CallNode) []*rawCallNode`
- [x] `marshalCallNode(n *CallNode) *rawCallNode` (handles variant)
- [x] `marshalEdge(e *Edge) *rawCallNode`
- [x] `marshalRecoveryPolicy(rp *RecoveryPolicy) *rawRecoveryPolicy`
- [x] `marshalJourney(j *Journey) *rawJourney`
- [x] `marshalStep(s *Step) *rawStep`
- [x] `marshalFault(f FaultSpec) *rawFault`
- [x] `marshalFaultTarget(t FaultTarget) string` (returns `node:<svc>` / `operation:<svc>.<op>` / `edge:<svc>.<op>-><svc>.<op>`)
- [x] `ptrInt(v int) *int`, `ptrFloat64(v float64) *float64`, `ptrDuration(v time.Duration) *time.Duration`

### Step 5.3 — Build check

- [x] `go build ./topology/...` — succeeds.

---

## Phase 6 — `equal.go` (LC-5)

### Step 6.1 — Implement Equal

- [x] Create `topology/equal.go`.
- [x] Implement `Equal(a, b *Schema) bool` per `business-logic-model.md` §4 and `business-rules.md` §4.
- [x] Implement helpers: `equalServices`, `equalService`, `equalOperations`, `equalOperation`, `equalCalls`, `equalCallNode`, `equalEdge`, `equalRecoveryPolicy`, `equalJourneys`, `equalJourney`, `equalSteps`, `equalStep`, `equalFaults`, `equalFaultSpec`, `equalFaultTarget`, `equalLatency`, `equalSeverity`.
- [x] Use `identifyOp` (from validate.go) for *Operation comparison via "<svc>.<op>" string.

### Step 6.2 — Build check

- [x] `go build ./topology/...` — succeeds.

---

## Phase 7 — `faults.go` (LC-6)

### Step 7.1 — Implement ApplyFaults + Overlay lookup

- [ ] Create `topology/faults.go`.
- [ ] Implement `(*Schema).ApplyFaults() *FaultOverlay` per `business-logic-model.md` §6.
- [ ] Implement `(o *FaultOverlay).NodeFaults / OperationFaults / EdgeFaults` (O(1) map lookups).
- [ ] Implement `FaultOverlayEqual(a, b *FaultOverlay) bool` (used by TP-U1-7).
- [ ] The `FaultOverlay` struct (currently opaque from U7) needs its 3 maps as unexported fields. If U7 left it as `type FaultOverlay struct{}`, replace with proper fields here.

### Step 7.2 — Build check

- [ ] `go build ./topology/...` — succeeds.

---

## Phase 8 — `jsonschema.go` (LC-7) + JSON Schema template

### Step 8.1 — Write the JSON Schema template

- [ ] Create directory `topology/jsonschema/`.
- [ ] Create `topology/jsonschema/topology.schema.json` per `business-rules.md` §9:
  - `$schema: https://json-schema.org/draft/2020-12/schema`
  - `$id: https://github.com/ymotongpoo/xk6-otel-gen/schemas/topology.schema.json`
  - `title`, `type: object`, `required: [services, journeys]`, `additionalProperties: true`
  - `$defs` for: Service, Operation, CallNode (oneOf: Edge or Parallel), Edge, RecoveryPolicy, Journey, Step, FaultSpec, FaultTarget, LatencyDist, SeverityParams
  - enum values for ServiceKind / Protocol / ExhaustedAction / FaultKind / BackoffPolicy
  - `description` fields summarizing each type's purpose
  - One minimal `examples` block at root

### Step 8.2 — Implement ExportJSONSchema

- [ ] Create `topology/jsonschema.go`.
- [ ] Add `//go:embed jsonschema/topology.schema.json` and the variable.
- [ ] Implement `(*Schema).ExportJSONSchema() ([]byte, error)` returning a copy of the embedded bytes (defensive copy so callers can't mutate the embed).

### Step 8.3 — Build check

- [ ] `go build ./topology/...` — succeeds.

---

## Phase 9 — `lint.go` (LC-8)

### Step 9.1 — Implement Lint API

- [ ] Create `topology/lint.go`.
- [ ] Implement `LintIssue` struct and `LintSeverity` enum with `String()` method (per `business-rules.md` §6).
- [ ] Implement `Lint(r io.Reader) ([]LintIssue, error)`:
  - Read input via `io.ReadAll`
  - Call `decodeRaw(strict=true)` — capture unknown-field errors from yaml.v3 as LintWarning entries
  - Build schema (same as Parse) — convert ParseError to LintError entries
  - Validate — convert each ValidationError into LintError entries
  - Return all issues (sorted by Path?) and nil error (unless decodeRaw at the syntax level failed)

### Step 9.2 — Implement `FindServiceByName` and `JourneyNames`

- [ ] Add to a new file `topology/schema_methods.go` (or fold into parse.go):
  - `(*Schema).FindServiceByName(id ServiceID) (*Service, bool)`
  - `(*Schema).JourneyNames() []string` — returns sorted slice
- [ ] These are 1-line wrappers (per `domain-entities.md` §2.8, §2.9).

### Step 9.3 — Build verification

- [ ] `go build ./...` (full repo) — **succeeds**.
- [ ] `go vet ./...` — no warnings.
- [ ] **Acceptance**: All public API is back in place. U7's `testutil/generators` should still build and pass its tests (Validate is now real, no longer panic stub).

---

## Phase 10 — Tests

> Reference: `logical-components.md` §3 (LC-T0..T12), NFR-D P-TEST-1..3 (inline fixtures, t.Parallel() everywhere).

### Step 10.1 — Example-based tests for Parse (LC-T1)

- [ ] Create `topology/parse_test.go` with `t.Parallel()` on each function:
  - `TestParse_MinimalSchema` — inline YAML with 1 svc + 1 journey
  - `TestParse_DefaultsApplied` — confirm Replicas=1 etc. when omitted
  - `TestParse_YAMLSyntaxError_FailFast` — malformed YAML returns single error
  - `TestParse_UnresolvedReference_ErrorPath` — bad `services.X.operations.Y.calls[0].to.service` → error contains the path
  - `TestParse_UnknownYAMLKey_IgnoredInParse` — lax behavior (Parse succeeds with unknown keys)
  - `TestParseFile_NotFound` — returns wrapped `*os.PathError`

### Step 10.2 — TP-U1-1: Round-trip PBT (LC-T2)

- [ ] Create `topology/parse_roundtrip_test.go`:
  ```go
  func TestParse_RoundTrip(t *testing.T) {
      t.Parallel()
      rapid.Check(t, func(t *rapid.T) {
          s := generators.ValidSchema().Draw(t, "schema")
          yamlBytes, err := yaml.Marshal(s)
          require.NoError(t, err)
          s2, err := topology.Parse(bytes.NewReader(yamlBytes))
          require.NoError(t, err)
          require.True(t, topology.Equal(s, s2), "round-trip lost or altered fields")
      })
  }
  ```

### Step 10.3 — TP-U1-2: Non-nil pointers (LC-T3)

- [ ] Create `topology/parse_pointers_test.go`:
  ```go
  func TestParse_NoNilPointers(t *testing.T) {
      t.Parallel()
      rapid.Check(t, func(t *rapid.T) {
          s := generators.ValidSchema().Draw(t, "s")
          yamlBytes, _ := yaml.Marshal(s)
          s2, err := topology.Parse(bytes.NewReader(yamlBytes))
          require.NoError(t, err)
          // walk s2: every *Service / *Operation / *Edge / *Step.Op is non-nil
      })
  }
  ```

### Step 10.4 — TP-U1-3: Map-key consistency (LC-T4)

- [ ] Create `topology/parse_consistency_test.go`:
  ```go
  func TestParse_MapKeyConsistency(t *testing.T) {
      t.Parallel()
      rapid.Check(t, func(t *rapid.T) {
          s := generators.ValidSchema().Draw(t, "s")
          // For all id in s.Services: s.Services[id].Name == id
          // For all op in s.Services[*].Operations: op.Service.Operations[op.Name] == op
      })
  }
  ```

### Step 10.5 — TP-U1-4: DAG (LC-T5)

- [ ] Create `topology/validate_dag_test.go`:
  - PBT: `TestValidate_AlwaysDAG` — for every ValidSchema, topology sort succeeds
  - Example: `TestValidate_DetectsCycle` — construct a 2-operation cycle manually and confirm validateDAG returns the right error message containing both operation names

### Step 10.6 — TP-U1-6: Validate idempotent (LC-T6)

- [ ] Create `topology/validate_idempotent_test.go`:
  ```go
  func TestValidate_Idempotent(t *testing.T) {
      t.Parallel()
      rapid.Check(t, func(t *rapid.T) {
          s := generators.ValidSchema().Draw(t, "s")
          err1 := topology.Validate(s)
          err2 := topology.Validate(s)
          require.Equal(t, err1 == nil, err2 == nil)
      })
  }
  ```

### Step 10.7 — Example-based Validate (LC-T7)

- [ ] Create `topology/validate_test.go`:
  - `TestValidate_StructuralRules` — table-driven, 1 example per R-STR-1..8 violation
  - `TestValidate_DomainRanges` — table-driven, sample D-1..D-14 violations
  - `TestValidate_OnValidSchemaReturnsNil` — sanity, build a known-valid Schema and confirm Validate==nil

### Step 10.8 — TP-U1-5 + TP-U1-7: ApplyFaults (LC-T8)

- [ ] Create `topology/applyfaults_test.go`:
  - `TestApplyFaults_OverlayCovers` (PBT, TP-U1-5)
  - `TestApplyFaults_Idempotent` (PBT, TP-U1-7) — use `FaultOverlayEqual`

### Step 10.9 — TP-U1-8: JSON Schema round-trip (LC-T9)

- [ ] Create `topology/jsonschema_roundtrip_test.go`:
  ```go
  func TestExportJSONSchema_RoundTrip(t *testing.T) {
      t.Parallel()
      rapid.Check(t, func(t *rapid.T) {
          s := generators.ValidSchema().Draw(t, "s")
          schemaBytes, _ := s.ExportJSONSchema()
          yamlBytes, _ := yaml.Marshal(s)
          jsonBytes, err := yamlToJSON(yamlBytes)
          require.NoError(t, err)
          c, err := jsonschema.CompileString("topology.json", string(schemaBytes))
          require.NoError(t, err)
          var doc any
          require.NoError(t, json.Unmarshal(jsonBytes, &doc))
          require.NoError(t, c.Validate(doc))
      })
  }
  // helper yamlToJSON
  ```

### Step 10.10 — Example-based for MarshalYAML (LC-T10)

- [ ] Create `topology/marshal_test.go`:
  - `TestMarshal_AlphabeticalOrder` — confirm services / operations / journeys appear sorted in output
  - `TestMarshal_PreservesSequenceOrder` — confirm faults / calls / fallback / steps preserve declaration order
  - `TestMarshal_OmitsZeroValues` — confirm `replicas: 1` omitted when default, `language: ""` omitted, etc.

### Step 10.11 — Example-based for Equal (LC-T11)

- [ ] Create `topology/equal_test.go`:
  - `TestEqual_Reflexive`
  - `TestEqual_Symmetric` (build pairs, swap and re-test)
  - `TestEqual_DistinguishesDifferentCallsOrder` (R-V vs order-preserving fields)
  - `TestEqual_IgnoresMapIterationOrder` (Services / Operations / Journeys are set-equal)

### Step 10.12 — testdata + BenchmarkParse (LC-T12)

- [ ] Create `topology/testdata/typical.yaml` — hand-crafted, 10 services / 30 operations / 50 edges / 5 journeys / 3 faults. Match the demo flavor of `examples/minimal/` style.
- [ ] Create `topology/bench_test.go` with `BenchmarkParse` per `nfr-design-patterns.md` §1 P-PERF-3.
- [ ] Run `go test -bench=BenchmarkParse -benchmem ./topology/...`. Record ns/op in `code-generation-summary.md`. Verify ≤ 10,000,000 ns/op.

### Step 10.13 — Un-skip U7's TestValidSchema_ValidatePlaceholder

- [ ] Open `testutil/generators/schema_test.go`.
- [ ] Find `TestValidSchema_ValidatePlaceholder` (was `t.Skip(...)` per U7 Phase 2 plan).
- [ ] Remove the `t.Skip(...)` line and the `TODO(u1):` comment.
- [ ] The body should now call `topology.Validate(s)` and assert `nil`.
- [ ] Run `go test -run TestValidSchema ./testutil/generators/...` — confirm pass.

### Step 10.14 — Test verification

- [ ] `go test -race -count=1 ./...` — passes (all).
- [ ] `go test -cover ./topology/...` — ≥ 80%.
- [ ] **Acceptance**: race-free, coverage ≥ 80%, all TP-U1-1..8 active.

---

## Phase 11 — Documentation Polish

### Step 11.1 — Example functions

- [ ] Create `topology/doc_test.go` with:
  - `ExampleParse` — minimal YAML in, walk Schema, print svc name
  - `ExampleSchema_MarshalYAML` — Parse then Marshal, demonstrate round-trip
  - `ExampleLint` — load YAML with an unknown key, print the issue

### Step 11.2 — GoDoc completeness review

- [ ] Run `go doc -all ./topology/` and visually verify every public identifier has GoDoc.
- [ ] Confirm `Schema` type GoDoc has "immutable after Parse" warning (P-IMM-1).
- [ ] Confirm package doc.go contains the IMMUTABILITY / CONCURRENCY / ERROR REPORTING sections.

---

## Phase 12 — Final Verification & Summary

### Step 12.1 — Full DoD battery

- [ ] `go build ./...` — passes
- [ ] `go test -race -count=1 ./...` — passes
- [ ] `go test -cover ./topology/...` — ≥ 80%
- [ ] `golangci-lint run ./...` — no warnings (if not installed, note in summary)
- [ ] `go vet ./...` — no warnings
- [ ] No `TODO(agent):` markers in `topology/` or `testutil/generators/`
- [ ] `go list -deps ./topology/...` includes `gopkg.in/yaml.v3` but does NOT include `log/*` (NFR-U1-4)

### Step 12.2 — Write `code-generation-summary.md`

- [ ] Create `aidlc-docs/construction/u1-topology/code/code-generation-summary.md` containing:
  - List of files created/modified (paths)
  - LOC per file (rough)
  - `BenchmarkParse` result (ns/op, B/op, allocs/op)
  - `go test -cover` percentage for `topology` and `testutil/generators`
  - List of TP-U1-1..8 tests that are passing
  - U7 `TestValidSchema_ValidatePlaceholder` status (un-skipped, passing)
  - Any deviations from the plan (rationale)
  - List of `TODO(u4):` / `TODO(u2):` / `TODO(u3):` markers (none expected in this unit, but record if Codex finds any)

### Step 12.3 — Mark all checkboxes [x]

- [ ] Mark all `[ ]` items in this document as `[x]` (including this one).
- [ ] Update `aidlc-docs/aidlc-state.md` Current Status: "U1 complete, ready for U4."

### Step 12.4 — Audit log entry

- [ ] Append to `aidlc-docs/audit.md`:
  ```
  ## U1 topology — Code Generation Complete (by implementation agent)
  **Timestamp**: <ISO 8601 now>
  **Implementation agent**: Codex CLI gpt-5.5 xhigh / Cursor Composer 2.5
  **Files created/modified**: <list>
  **Coverage (topology)**: <pct>%
  **BenchmarkParse**: <ns/op>
  **U7 test un-skip**: TestValidSchema_ValidatePlaceholder now passing
  **Deviations**: <list or "none">
  ```

---

## Phase 13 — Extend U7 testutil/generators with 18 new generators
**Recommended agent**: **Cursor Composer 2.5 (batch mode via `agent -p`)** — this phase is **additive, pattern-conforming work**: the 18 new generators follow the established U7 generator style (atomic helpers, `Valid<Type>` / `Any<Type>` naming, functional options, R-V/R-A invariants). Cursor's codebase-aware nature makes it ideal for matching existing style precisely. Run via `scripts/run-cursor.sh u1 --phases 13`.

> Per FD `domain-entities.md` §6 (Q13=A). These 9 Valid + 9 Any pairs are added to the existing U7 package without disturbing existing generators.

### Step 13.1 — Add primitives / sub-generators (LC-2 of U7)

- [ ] Add to `testutil/generators/primitives.go`:
  - `ValidErrorRate / AnyErrorRate` (alias of ValidProbability / AnyProbability if not already present)
  - `ValidTimeoutDuration / AnyTimeoutDuration` (already exist as ValidTimeout / AnyTimeout)

### Step 13.2 — Add Operation / Edge generators

- [ ] Create `testutil/generators/operation.go`:
  - `ValidOperation(svc *topology.Service, opts ...OperationOption) *rapid.Generator[*topology.Operation]`
  - `AnyOperation(svc *topology.Service, opts ...OperationOption) *rapid.Generator[*topology.Operation]`
  - `OperationOption` type with `MaxCalls(n int)`, `WithName(s string)`
- [ ] Create `testutil/generators/edge.go`:
  - `ValidEdge(from, to *topology.Operation, opts ...EdgeOption) *rapid.Generator[*topology.Edge]`
  - `AnyEdge(from, to *topology.Operation, opts ...EdgeOption) *rapid.Generator[*topology.Edge]`
  - `EdgeOption` type with `WithProtocol(p)`, `WithLatency(p50, p95 time.Duration)`, `WithErrorRate(r)`, `WithOnFailure(rp *topology.RecoveryPolicy)`

### Step 13.3 — Add CallNode / RecoveryPolicy generators

- [ ] Create `testutil/generators/callnode.go`:
  - `ValidCallNode(from *topology.Operation, target *topology.Operation, opts ...CallNodeOption) *rapid.Generator[*topology.CallNode]`
  - `AnyCallNode(...)` — including degenerate (both Edge and Parallel set, or neither)
- [ ] Create `testutil/generators/recovery.go`:
  - `ValidRecoveryPolicy(from *topology.Operation, fallbackTargets []*topology.Operation, opts ...RecoveryPolicyOption) *rapid.Generator[*topology.RecoveryPolicy]`
  - `AnyRecoveryPolicy(...)`

### Step 13.4 — Add Journey / Step generators

- [ ] Create `testutil/generators/journey.go`:
  - `ValidJourney(schema *topology.Schema, opts ...JourneyOption) *rapid.Generator[*topology.Journey]`
  - `AnyJourney(...)`
  - `ValidStep(schema *topology.Schema, opts ...StepOption) *rapid.Generator[*topology.Step]`
  - `AnyStep(...)`

### Step 13.5 — Add Fault generators

- [ ] Create `testutil/generators/fault.go`:
  - `ValidFaultSpec(schema *topology.Schema, opts ...FaultOption) *rapid.Generator[topology.FaultSpec]`
  - `AnyFaultSpec(...)`
  - `ValidFaultTarget(schema *topology.Schema, opts ...FaultTargetOption) *rapid.Generator[topology.FaultTarget]`
  - `AnyFaultTarget(...)`
  - `ValidFaultOverlay(schema *topology.Schema, opts ...FaultOverlayOption) *rapid.Generator[*topology.FaultOverlay]` — synthesizes by calling Schema.ApplyFaults after building a random Schema with faults
  - `AnyFaultOverlay(...)`

### Step 13.6 — Update U7's domain-entities.md §8 with this completed entry

- [ ] Append to `aidlc-docs/construction/u7-testutil/functional-design/domain-entities.md` §8:
  ```markdown
  ### Request from U1 FD (COMPLETED in U1 Code Generation Phase 13)
  - ValidOperation, AnyOperation
  - ValidEdge, AnyEdge
  - ValidCallNode, AnyCallNode
  - ValidRecoveryPolicy, AnyRecoveryPolicy
  - ValidJourney, AnyJourney
  - ValidStep, AnyStep
  - ValidFaultSpec, AnyFaultSpec
  - ValidFaultTarget, AnyFaultTarget
  - ValidFaultOverlay, AnyFaultOverlay
  Files added: testutil/generators/{operation,edge,callnode,recovery,journey,fault}.go
  ```

### Step 13.7 — Test verification

- [ ] `go test -race -count=1 ./testutil/generators/...` — passes
- [ ] `go test -cover ./testutil/generators/...` — ≥ 80% (was 88.5% before; should stay above)
- [ ] **Acceptance**: All 18 new generator pairs exist and have at least a smoke test.

---

## Files Expected (final inventory)

```text
go.mod (updated: go 1.25, +yaml.v3, +jsonschema/v5 test)
go.sum (updated)

topology/
├── doc.go                         (UPDATED from U7)
├── enums.go                       (unchanged from U7)
├── types.go                       (unchanged from U7)
├── (stubs.go DELETED)
├── raw.go                         (NEW)
├── parse.go                       (NEW)
├── validate.go                    (NEW)
├── marshal.go                     (NEW)
├── equal.go                       (NEW)
├── faults.go                      (NEW)
├── jsonschema.go                  (NEW)
├── lint.go                        (NEW)
├── errors.go                      (NEW)
├── schema_methods.go              (NEW, or inline into parse.go)
├── jsonschema/
│   └── topology.schema.json       (NEW)
├── testdata/
│   └── typical.yaml               (NEW, bench fixture)
├── doc_test.go                    (NEW, Example functions)
├── parse_test.go                  (NEW)
├── parse_roundtrip_test.go        (NEW)
├── parse_pointers_test.go         (NEW)
├── parse_consistency_test.go      (NEW)
├── validate_dag_test.go           (NEW)
├── validate_idempotent_test.go    (NEW)
├── validate_test.go               (NEW)
├── applyfaults_test.go            (NEW)
├── jsonschema_roundtrip_test.go   (NEW)
├── marshal_test.go                (NEW)
├── equal_test.go                  (NEW)
└── bench_test.go                  (NEW)

testutil/generators/                  (existing U7 + Phase 13 additions)
├── (existing U7 files unchanged)
├── operation.go                   (NEW Phase 13)
├── edge.go                        (NEW Phase 13)
├── callnode.go                    (NEW Phase 13)
├── recovery.go                    (NEW Phase 13)
├── journey.go                     (NEW Phase 13)
└── fault.go                       (NEW Phase 13)

testutil/generators/schema_test.go    (MODIFIED: un-skip TestValidSchema_ValidatePlaceholder)

aidlc-docs/construction/u1-topology/code/
└── code-generation-summary.md      (NEW Phase 12)
```

Total estimated LOC for production code (non-test): ~975 LOC for `topology/` + ~600 LOC for U7 generator additions = ~1575 LOC.
Test code: ~800-1000 LOC.

---

## Boundaries (do NOT cross)

- Do NOT modify `aidlc-docs/**` except as instructed (audit.md append, code-generation-summary.md create, this plan's checkboxes, U7's domain-entities.md §8 append).
- Do NOT change `AGENTS.md`, `CLAUDE.md`, `.cursor/rules/**`, `.codex/**`, `.aidlc-rule-details/**`.
- Do NOT add dependencies other than `gopkg.in/yaml.v3` and `github.com/santhosh-tekuri/jsonschema/v5` (test-only).
- Do NOT add `log/*` to body imports of any non-test file in `topology/` (NFR-U1-4).
- Do NOT introduce package-level mutable state in `topology/` (NFR-U1-6 / P-CONC-1).
- Do NOT modify `topology/enums.go` or `topology/types.go` (they are final per U7 scaffold and Application Design `component-methods.md`). If types absolutely must change, STOP and ask via audit.md.
- If you discover an ambiguity or a need to deviate, **STOP and append a question to `audit.md`** under an "Implementation-time Question" heading. Do not silently resolve.

---

## Out of Scope (handled in later units / stages)

- U2 (journey) will use this Schema + FaultOverlay to execute journeys.
- U4 (exporter) will implement the OTLP send pipeline (independent of topology types but uses them).
- CI workflow integration (golangci-lint, coverage gating, race testing in matrix) — Build and Test stage.
- Release / GoReleaser config, README updates beyond what `examples/` requires — U8.
- `Mermaid` input parsing — deferred per requirements.md FR-2.1.
- Schema migration / versioning between v1 / v2 — deferred per FD `domain-entities.md` §7.
