# Remediation Prompt — Requirements Gap Fixes (Post-AIDLC Verification)

**Date**: 2026-06-11
**Audience**: Codex CLI / Cursor Composer (autonomous implementation agent)
**Source**: Post-completion verification of `aidlc-docs/inception/requirements/requirements.md` against the implemented codebase
**Status of repo at verification**: `go build ./...` OK, `go test ./...` all packages pass (commit `92ac4c7`)

このドキュメントは、AI-DLC ワークフロー完了後の要件照合レビューで発見されたギャップの修正指示書です。各タスクは独立して実装・コミット可能です。タスクごとに 1 コミット（Conventional Commits 形式）としてください。

---

## Project Context (read first)

- `xk6-otel-gen` is a k6 extension (Go) that synthesizes OpenTelemetry traces/metrics/logs from a declarative YAML topology and exports them via OTLP (gRPC / HTTP-protobuf).
- Package map: `topology/` (YAML parse/validate/JSON Schema), `journey/` (execution engine, fault injection), `synth/` (OTel signal synthesis, semconv attributes), `exporter/` (OTLP pipeline, config, stats), `k6otelgen/` (k6 JS module `k6/x/otel-gen`), `k6output/` (k6 output extension `--out otel-gen=...`).
- **Span timestamps are synthetic**: the journey executor computes latencies from distributions and sets span start/end times arithmetically — it does NOT sleep wall-clock time. All timing-related fixes below MUST follow this model (no `time.Sleep` in the execution path).
- Requirements doc: `aidlc-docs/inception/requirements/requirements.md`. Per-unit design docs: `aidlc-docs/construction/u1-topology/` … `u6-k6output/`.

## Hard Constraints (apply to every task)

1. **Property-Based Testing is FULL-enforcement** in this project (`pgregory.net/rapid`). Any new invariant-bearing logic MUST get a PBT in the package's `pbt_test.go`, alongside table-driven unit tests.
2. Every new/changed `.go` file keeps the `// SPDX-License-Identifier: Apache-2.0` header (enforced by goheader lint).
3. After each task: `go build ./... && go test -race -count=1 ./<changed packages>/... && golangci-lint run ./<changed packages>/...` must pass. Before the final commit, run `go test -race -count=1 ./...`.
4. Conventional Commits 1.0.0; one commit per task; suggested type/scope given per task. Subject ≤ 72 chars, imperative mood.
5. Update affected docs in the same commit: `README.md`, `topology/jsonschema/topology.schema.json` (if YAML schema changes), examples, and the relevant `aidlc-docs/construction/<unit>/functional-design/*.md` (append a "Remediation 2026-06-11" note rather than rewriting history).
6. Do not break public JS API compatibility: `configure`, `load`, `stats`, `journeys`, `handle.runJourney(name)`, `handle.journeys()` must keep working unchanged.

---

## Task 1 — FR-6.1: Add missing required Semantic Conventions attributes

**Priority: High** | **Suggested commit**: `feat(synth): add missing required semconv attributes (FR-6.1)`

### Current state (evidence)

FR-6.1 lists these as **required**, but they are never emitted:

- `service.namespace` — absent from `synth/resource.go` (which sets `service.name`, `service.instance.id`, `telemetry.sdk.*`, `service.version` at `synth/resource.go:39-45`).
- `url.scheme` — absent from `synth/attributes.go` `httpStaticAttrs` (`synth/attributes.go:112-130`).
- `server.port` — present in the `allowedAttrKeys` allowlist (`synth/attributes.go:191`) but never set.
- `network.peer.address` — absent entirely.
- `exception.type` / `exception.message` — absent; only `error.type` is set on failure (`synth/attributes.go:179-181`).

### Required behavior

1. **`service.namespace`** (Resource attribute):
   - Add an optional `namespace` field to the topology YAML: top-level `namespace: <string>` applying to all services, overridable per-service with `services.<name>.namespace`.
   - Default when unset: `"xk6-otel-gen"` (synthetic but present — FR-6.1 marks it required).
   - Wire through `topology` types/parse/validate → `synth/resource.go`. Update `topology/jsonschema/topology.schema.json` and the schema generator if schema is code-generated.
2. **`url.scheme`** (HTTP spans, client side): set `"http"` statically (synthetic services have no TLS).
3. **`server.port`** (HTTP/gRPC client spans, alongside existing `server.address`): synthesize deterministically by target service kind/protocol — http `8080`, grpc `50051`, database(postgres) `5432`, cache(redis) `6379`, queue(kafka) `9092`, external_api `443`. Centralize the mapping in one function with a unit test.
4. **`network.peer.address`** (client spans): synthesize a deterministic fake IP per target service instance, e.g. `10.<h1>.<h2>.<instance_idx+1>` where `h1`,`h2` derive from a stable hash of the service name. Must be deterministic for the same (service, instance) — same approach as the existing deterministic `service.instance.id` UUID in `synth/resource.go`.
5. **`exception.type` / `exception.message`** (on failed spans): in the span finish path (`synth/synthesizer.go` finish func, around lines 100-103), when `outcome.Success == false`, record a span **event** named `exception` carrying `exception.type` (map from `outcome.ErrorType`, e.g. `http.500` → `ServerError`/keep raw type if no natural mapping) and `exception.message` (short synthetic message, e.g. `"simulated failure: http.500 on payment.authorize_card"`). Keep the existing `error.type` attribute as-is. Also add `exception.type`/`exception.message` to the error-path log record attributes in `EmitLog` (`synth/synthesizer.go:130-159`).
6. Add every new key to `allowedAttrKeys` (`synth/attributes.go:185+`) and to any attribute-policy tests.

### Acceptance criteria

- Integration test (`synth/integration` or extend `journey/integration`) asserts: resource has `service.namespace`; HTTP client spans carry `url.scheme`, `server.port`, `network.peer.address`; a forced-failure span has an `exception` event with both attributes.
- PBT: `network.peer.address` generation is deterministic and yields a valid dotted-quad for arbitrary service names / instance indices.
- Existing examples (`examples/minimal`, `examples/astroshop`) still validate: `go test ./test/examples/...`.
- README Topology YAML Reference documents the new `namespace` field.

---

## Task 2 — FR-7.1: Enforce `Edge.Timeout` during journey execution

**Priority: High** | **Suggested commit**: `feat(journey): enforce edge timeout as simulated failure (FR-7.1)`

### Current state (evidence)

- `Edge.Timeout` is parsed (`topology/types.go:49`) and validated non-negative (`topology/validate.go:451-453`) but the journey executor never reads it — `grep Timeout journey/*.go` (production files) returns nothing.
- FR-7.1 requires: "高レイテンシ・タイムアウト (`timeout` 超過時に span は error として記録される)".
- The error type `"timeout"` already exists in `journey.AllowedErrorTypes` (`journey/errors.go:72`) but is never produced.

### Required behavior

In `journey/executor.go` (around the latency computation at lines ~80-81 where `effectiveLatency := baseLatency + ff.latencyInflate`):

1. If `node.Edge != nil && node.Edge.Timeout > 0 && effectiveLatency > node.Edge.Timeout`:
   - Clamp the simulated span duration to `node.Edge.Timeout` (the caller gives up at the timeout; synthetic timestamps, no sleeping).
   - Mark the outcome as failure with protocol-appropriate values: HTTP → `ErrorType: "timeout"`, status code `504`; gRPC → `ErrorType: "grpc.deadline_exceeded"`, gRPC status `DEADLINE_EXCEEDED (4)`; messaging/db → `"timeout"`.
2. Timeout failure must interact correctly with existing machinery: it triggers the same cascade/recovery paths as an `error_rate` failure (verify against `executeCascade` at `journey/executor.go:178-196` and `journey/recovery.go`).
3. The downstream (callee) span still runs its full simulated latency; only the caller-side outcome is a timeout. If the current single-outcome-per-node model makes this caller/callee split impractical, clamp both and note the simplification in the functional-design remediation note.

### Acceptance criteria

- Unit test: edge with `timeout: 50ms` and constant latency `200ms` → span error, `error.type` per protocol, duration == 50ms.
- Unit test: latency below timeout → unaffected.
- PBT: for arbitrary latency distributions and timeouts, simulated duration never exceeds `Timeout` when `Timeout > 0`, and outcome failure ⇔ sampled latency exceeded timeout.
- Integration test in `journey/integration` shows a timeout span exported with `status=ERROR`.

---

## Task 3 — FR-7.1: Apply `Edge.RetryBackoff` timing in retry simulation

**Priority: Medium** | **Suggested commit**: `feat(journey): apply retry backoff timing to simulated retries (FR-7.1)`

### Current state (evidence)

- `Edge.Retries` / `Edge.RetryBackoff` (exponential/linear/constant) are parsed (`topology/parse.go:227-228`) and validated (`topology/validate.go:454-456`), and retries are simulated as sequential fallback executions in `journey/recovery.go:11-46` — but **no backoff delay separates attempts**, so retry storms do not propagate latency upstream as FR-7.1 requires ("`retries` と `retry_backoff` を尊重し、上流の遅延が下流に伝播する").

### Required behavior

1. First, read `journey/recovery.go` and the plan/executor to understand exactly how retry attempts map to spans (the AI-DLC design used recovery/fallback edges; confirm whether `Edge.Retries` generates attempt spans, and if it currently does not, make it do so: a failed call with `retries: N` produces up to `1+N` attempt spans as children/siblings under the caller).
2. Insert **simulated** backoff gaps between attempt start times (synthetic timestamps, never `time.Sleep`):
   - `constant`: `d, d, d, ...`
   - `linear`: `d, 2d, 3d, ...`
   - `exponential`: `d, 2d, 4d, ...`
   - Base delay `d`: add optional YAML field `retry_base_delay` on the edge (duration, default `100ms`), validated ≥ 0. Update JSON Schema + README.
3. The caller span's total simulated duration must include all attempt latencies plus backoff gaps, so the delay propagates upstream through parent spans (this is the "retry storm" visibility requirement).
4. Cap: if the edge also has `timeout`, Task 2's clamp applies to the total.

### Acceptance criteria

- Unit test per policy: with `retries: 2`, `retry_base_delay: 100ms`, failing edge — attempt span start times are spaced per policy and parent duration ≥ sum(attempt latencies) + sum(backoff gaps).
- PBT: backoff gap sequence is monotonically non-decreasing for linear/exponential; total duration is the exact arithmetic sum (synthetic-time invariant).
- `examples/astroshop/topology.yaml` gains at least one edge demonstrating `retries` + `retry_backoff` (keep faults subtle, consistent with the existing example style).

---

## Task 4 — FR-5.1: Weighted journey selection API

**Priority: Medium** | **Suggested commit**: `feat(k6otelgen): add weighted random journey selection (FR-5.1)`

### Current state (evidence)

- `Journey.Weight` is parsed with default 1.0 (`topology/parse.go:150`) and validated > 0 (`topology/validate.go:415-417`) but nothing consumes it; users must hand-roll weighted selection in JS. FR-5.1 requires "重み付き選択をサポート".

### Required behavior

1. Add `Engine.PickJourney() string` (or similar) in `journey/` performing weighted random selection over journeys using the engine's existing seeded RNG (see `NewEngineWithSeed` — keep determinism for tests).
2. Expose it on the JS handle: `handle.runRandomJourney()` — picks by weight then executes (equivalent to `runJourney(picked)`), returning the chosen journey name to JS. Also expose `handle.journeyWeights()` returning `{name: weight}` so scripts can implement custom logic.
3. Document in README Usage table and in both example READMEs; switch `examples/astroshop/script.js` (or one scenario in it) to `runRandomJourney()` to demonstrate.

### Acceptance criteria

- Unit test: with seeded RNG, selection frequencies over N=10k draws approximate weights (tolerance based on seed-fixed expected counts, not statistical flakiness — use a fixed seed and golden expected distribution).
- PBT: PickJourney always returns a defined journey; a journey with weight w>0 is selectable; single-journey topology always returns it.
- JS-level test in `k6otelgen` (module test pattern already exists in `k6otelgen/module_test.go`) covering `runRandomJourney`.

---

## Task 5 — NFR-5.2: Expose exporter stats as native k6 metrics in the summary

**Priority: Medium** | **Suggested commit**: `feat(k6otelgen): publish exporter stats as k6 metrics (NFR-5.2)`

### Current state (evidence)

- Send success/failure counters exist (`exporter/stats.go:15-43`) and are reachable from JS via `otelgen.stats()` (`k6otelgen/instance.go:116-131`), but NFR-5.2 requires they appear **as k6 metrics in `k6 summary`** without user JS code. Today nothing registers them in the k6 metrics registry.

### Required behavior

1. In `k6otelgen`, register custom k6 counters via the VU's metrics registry (`vu.InitEnv().Registry`): `otel_gen_traces_exported`, `otel_gen_traces_failed`, `otel_gen_metrics_exported`, `otel_gen_metrics_failed`, `otel_gen_logs_exported`, `otel_gen_logs_failed`, and a gauge `otel_gen_queue_drops` (k6output drop counter, if reachable; otherwise scope to the JS-module pipeline and note it).
2. Emit deltas as k6 samples — the natural hook is after each `runJourney` / `runRandomJourney` call (compute counter deltas since last emission and push samples to `vu.State().Samples`). Guard against running outside an active VU state (init context) without panicking.
3. The k6output extension (`k6output/`) already counts drops (`k6output/output.go:60`); have its `Stop()` log final stats at info level (it already warns on errors) — do not attempt cross-output metric registration there.
4. Document the new metric names in README (Configuration or a new "Built-in metrics" subsection).

### Acceptance criteria

- Module-level test asserting that after a journey run, the registry contains the `otel_gen_*` metrics and samples were pushed with the correct deltas.
- A manual verification note in the commit body: `./k6 run examples/minimal/script.js ...` summary shows `otel_gen_traces_exported` (cannot run in CI; the Go-level test is the gate).

---

## Task 6 — FR-8.3 (minor): `sampler` option in `configure()`

**Priority: Low** | **Suggested commit**: `feat(exporter): support sampler option in configure (FR-8.3)`

### Current state (evidence)

- FR-8.3 lists `sampler` among configure options; `k6otelgen/config.go:14-90` and `exporter.Config` have no sampler support — the tracer provider always exports everything.

### Required behavior

1. Add `sampler` to JS options and `OTEL_TRACES_SAMPLER`/`OTEL_TRACES_SAMPLER_ARG` env support: accept `always_on` (default), `always_off`, `traceidratio` (with ratio arg, JS form: `{ sampler: "traceidratio", samplerArg: 0.1 }`).
2. Wire into `sdktrace.NewTracerProvider` (`exporter/pipeline.go:75-82`) via `sdktrace.WithSampler`. JS option overrides env, consistent with the existing `MergeWith` priority.
3. Note in README that sampling applies to traces only (metrics/logs unaffected), and that logs still carry trace IDs of unsampled traces (document the behavior you implement).

### Acceptance criteria

- Unit tests for option parsing (invalid sampler name → ConfigError; ratio bounds [0,1]).
- Integration-style test: `traceidratio 0.0` exports zero spans while metrics/logs still flow.

---

## Explicitly NOT in scope (do not implement)

- **Mermaid / Service Graph JSON / Kubernetes manifest input**: descoped by approved requirement decision Q2 / assumption A-1 (`requirements.md` FR-2.1, §6 A-1). Future phase only.
- Reworking the synthetic-time execution model, OTLP exporter architecture, or YAML schema beyond the fields named above.
- CI workflow YAML files, SECURITY.md, CONTRIBUTING.md (deferred post-stage items per build-and-test-summary).

## Final verification (after all tasks)

```bash
go build ./...
go test -race -count=1 ./...
golangci-lint run
go run ./cmd/xk6-otel-gen-schema > /tmp/schema.json && diff /tmp/schema.json topology/jsonschema/topology.schema.json
go test ./test/examples/...
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=. && ./k6 version
```

Each commit message ends with the implementing agent's own co-author trailer per repo convention (see `.agent-memory/feedback-conventional-commits.md`).
