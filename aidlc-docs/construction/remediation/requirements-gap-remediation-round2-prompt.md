# Remediation Prompt — Round 2: Requirements Gap Fixes (Post-Remediation Re-Verification)

**Date**: 2026-06-11
**Audience**: Codex CLI / Cursor Composer (autonomous implementation agent)
**Source**: Second verification pass of `aidlc-docs/inception/requirements/requirements.md` against the codebase at HEAD `e71320a` (after the Round 1 remediation commits `3224f7f`..`e71320a`)
**Status of repo at verification**: `go build ./...` OK, `go test -race -count=1 ./...` all packages pass

このドキュメントは Round 1 修正後の再検証で見つかった残ギャップの修正指示書です。Round 1 の指示書は `requirements-gap-remediation-prompt.md`（同ディレクトリ）。Round 1 の 6 修正はすべて正しく実装済みでリグレッションなしと確認済み — 本ドキュメントは**それ以外の**残項目のみを扱います。

---

## ⚠️ COMMIT DISCIPLINE — READ FIRST

**一作業区切り（= 本ドキュメントの 1 タスク）が完了するたびに、必ずその場でコミットを作成すること。**

- 各タスクの完了条件は「実装 + テストが green + そのタスク分のみをステージしてコミット済み」である。コミットせずに次のタスクへ進んではならない。
- 複数タスクをまとめた一括コミットは**禁止**。`git add -A` ではなく、そのタスクで触れたファイルのみを明示的に `git add` すること。
- コミット前に最低限 `go build ./...` と変更パッケージの `go test -race -count=1 ./<pkg>/...` を実行し、green であることを確認すること。
- コミットメッセージは Conventional Commits 1.0.0 形式（各タスクに例を記載）。subject は命令形・72 文字以内。body には WHY を書く。末尾に実装エージェント自身の `Co-Authored-By:` トレーラーを付けること（`.agent-memory/feedback-conventional-commits.md` 参照）。

---

## Project Context (read first)

- `xk6-otel-gen` is a k6 extension (Go) that synthesizes OpenTelemetry traces/metrics/logs from a declarative YAML topology and exports them via OTLP (gRPC / HTTP-protobuf).
- Package map: `topology/` (YAML parse/validate/JSON Schema), `journey/` (execution engine, fault injection, weighted selection), `synth/` (OTel signal synthesis, semconv attributes), `exporter/` (OTLP pipeline, config, sampler, stats), `k6otelgen/` (k6 JS module `k6/x/otel-gen`, native k6 metrics), `k6output/` (k6 output extension `--out otel-gen=...`).
- **Span timestamps are synthetic**: the journey executor computes latencies from distributions and sets span start/end times arithmetically — it does NOT sleep wall-clock time. Never add `time.Sleep` to the execution path.
- Requirements doc: `aidlc-docs/inception/requirements/requirements.md`.

## Hard Constraints (apply to every task)

1. **Commit per task** — see COMMIT DISCIPLINE above. This is non-negotiable.
2. **Property-Based Testing is FULL-enforcement** (`pgregory.net/rapid`). Any new invariant-bearing logic MUST get a PBT in the package's `pbt_test.go` (or a dedicated `*_pbt_test.go`), alongside table-driven unit tests.
3. Every new/changed `.go` file keeps the `// SPDX-License-Identifier: Apache-2.0` header (goheader lint).
4. After each task: `go build ./... && go test -race -count=1 ./<changed packages>/... && golangci-lint run ./<changed packages>/...` must pass. Before the final commit, run `go test -race -count=1 ./...`.
5. Do not break the public JS API: `configure`, `load`, `stats`, `journeys`, `handle.runJourney(name)`, `handle.runRandomJourney()`, `handle.journeyWeights()`, `handle.journeys()` must keep working unchanged.
6. Update affected docs in the same task commit: `README.md`, `topology/jsonschema/topology.schema.json` (if the YAML schema changes — it does NOT in this round), examples, and relevant `aidlc-docs/` notes.

---

## Task 1 — NFR-5.1: Route JS-module logs through the k6 logging mechanism

**Priority: Medium**

**Suggested commit**:
```
feat(k6otelgen): route module logs through k6 logger (NFR-5.1)
```

### Current state (evidence)

- NFR-5.1 requires: "拡張自身の動作ログ (info/warn/error) は k6 のログ機構を通じて出力する。"
- The output extension complies: `k6output/output.go:79-88` captures `params.Logger` and uses it via `o.warn()` (`k6output/output.go:477-482`).
- The JS module does NOT: `grep -rn 'Logger' k6otelgen/*.go` (production files) hits only `pipeline.LoggerProvider()` at `k6otelgen/instance.go:197`, which is the OTel log SDK — not k6's logger. Load/configure events and export failures are invisible on the k6 console.

### Required behavior

1. Acquire the k6 logger in the module instance: `vu.InitEnv().Logger` during init context, falling back to `vu.State().Logger` in VU context (both are `logrus.FieldLogger` in k6 v1.x). Store it on `ModuleInstance` with a nil-safe accessor (test fakes may provide neither — follow the nil-guard style already used in `k6otelgen/metrics.go:63,74`).
2. Emit through it:
   - **info** on successful `load`: topology path, service count, journey count.
   - **info** on `configure`: endpoint and protocol actually applied (do NOT log header values — they may contain credentials).
   - **warn** when export failures increase: in `emitExporterStats` (`k6otelgen/metrics.go`), when any `*Failed` delta > 0, log one warn per emission cycle summarizing the failed counts. Rate-limit naturally by the per-journey emission cadence; do not log per-signal-per-failure.
3. No logging on the hot path beyond the failure-delta warn (NFR-1.2: keep journey execution overhead negligible).

### Acceptance criteria

- Unit test with a fake VU whose logger is a `logrus` test hook (`logrus/hooks/test`): after `load` an info entry exists; after a journey run with a stats stub showing failures, a warn entry exists.
- Unit test: all paths safe (no panic) when both `InitEnv()` and `State()` return nil or have nil loggers.
- README does not need changes (internal behavior), but `aidlc-docs/construction/u5-k6otelgen/` gets a one-line remediation note appended (do not rewrite history; add a "Remediation 2026-06-11 (Round 2)" section).

---

## Task 2 — FR-4.2: TLS certificate options (env + JS), beyond `insecure`

**Priority: Medium**

**Suggested commit**:
```
feat(exporter): add TLS certificate options (FR-4.2)
```

### Current state (evidence)

- FR-4.2 requires endpoint/headers/**TLS settings** configurable via standard `OTEL_EXPORTER_OTLP_*` env vars AND JS options, JS taking priority.
- Today the only TLS-related knob is the `insecure` bool (`k6otelgen/config.go:37-42`, wired to `WithInsecure()` in `exporter/exporters.go`). The standard certificate env vars are unsupported, so a private-CA or mTLS collector endpoint cannot be used.

### Required behavior

1. Add to `exporter.Config`: `Certificate string` (CA bundle file path), `ClientCertificate string`, `ClientKey string` (both required together for mTLS).
2. Env support in `ConfigFromEnv` (`exporter/config.go:158+`), reusing the existing `lookupSignalEnv` fallback chain (`exporter/config.go:211-223`):
   - `OTEL_EXPORTER_OTLP_CERTIFICATE` (+ `OTEL_EXPORTER_OTLP_{TRACES,METRICS,LOGS}_CERTIFICATE`)
   - `OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE` (+ signal variants)
   - `OTEL_EXPORTER_OTLP_CLIENT_KEY` (+ signal variants)
3. JS options in `optsToConfig` (`k6otelgen/config.go`): `caCert`, `clientCert`, `clientKey` (string paths). JS overrides env via the existing `MergeWith` ordering (`exporter/config.go:143+`). Also accept the same keys as `--out otel-gen=...` params in `k6output/params.go` if that file follows a key registry pattern — match the existing param plumbing.
4. Build a `*tls.Config` once per pipeline: load the CA into a cert pool, load the client keypair if both are set. Wire into all 6 exporters (`exporter/exporters.go`): gRPC via `credentials.NewTLS` + the `WithTLSCredentials` option of each otlp*grpc exporter; HTTP via the `WithTLSClientConfig` option of each otlp*http exporter.
5. Validation (fail fast, NFR-2.2): unreadable file or invalid PEM → `ConfigError`-style error at `Validate()`/pipeline build time with the offending path in the message. `ClientCertificate` xor `ClientKey` set → validation error. `Certificate` set together with `Insecure: true` → validation error (contradictory).
6. README: extend the Configuration section and the Security section's production-style example with `caCert`/mTLS usage.

### Acceptance criteria

- Unit tests: valid CA path accepted; missing file → error naming the path; cert-without-key → error; insecure+certificate → error. Use `t.TempDir()` with generated self-signed PEMs (`crypto/x509` + `crypto/ecdsa` in test helpers — no fixtures checked in).
- Test that env vars populate Config and that JS opts override env (extend the existing MergeWith tests in `exporter/config_test.go`).
- PBT: extend `testutil/generators/exporter_config.go` `AnyConfig` mutation cases with the new invalid combinations so `Validate()` round-trip properties cover them.
- Integration note: actually exercising TLS against a collector is covered by the existing `//go:build integration` suite only if convenient; not required for this task.

---

## Task 3 — NFR-1.1: End-to-end 1,000 RPS sustained benchmark

**Priority: Medium**

**Suggested commit**:
```
test(journey): add end-to-end 1k RPS sustained benchmark (NFR-1.1)
```

### Current state (evidence)

- NFR-1.1 requires sustaining ~1,000 journeys/s per runner on a 4 vCPU host. Existing benchmarks are unit-scoped (`journey/bench_test.go` measures ~2µs/step pure overhead; `synth/bench_test.go` similar). There is no benchmark that runs the full path journey engine → synthesizer → SDK pipeline at a realistic topology size, so the 1k RPS claim has never been demonstrated end-to-end.

### Required behavior

1. Add an e2e benchmark (suggested: `journey/e2e_bench_test.go` or a new `test/bench/` package if cross-package imports get awkward) that:
   - Loads `examples/astroshop/topology.yaml` (18 services — realistic size; read it from the repo path via `runtime.Caller`-relative or `../examples/...`).
   - Builds a real `synth.Synthesizer` backed by SDK providers with **no-op/in-memory exporters** (e.g. `sdktrace` with a discarding SpanProcessor, metric ManualReader, log processor that drops) so the benchmark measures synthesis cost without network noise.
   - Runs weighted journeys (`PickJourney` + `Execute`) in `b.RunParallel` to model multiple VUs.
   - Reports `journeys/sec` via `b.ReportMetric`.
2. Add a guard test `TestSustained1kRPSBudget` (skipped with `-short` and in race mode if too slow) that executes for ~3 seconds of wall time and fails if measured throughput < 1,000 journeys/s. Keep the threshold conservative so CI-class machines pass with margin; document that the formal NFR target hardware is 4 vCPU / 8 GB.
3. Append a "1k RPS e2e benchmark" subsection to `aidlc-docs/construction/build-and-test/performance-test-instructions.md` with the exact `go test -bench` / `go test -run TestSustained1kRPSBudget` commands and interpretation guidance.

### Acceptance criteria

- `go test -bench BenchmarkE2E -benchtime=2s ./journey/...` (or the chosen package) runs and reports journeys/sec.
- The guard test passes on the dev machine and is `testing.Short()`-skippable.
- No `time.Sleep` introduced into production code; the benchmark itself may use wall-clock measurement.

---

## Task 4 — FR-8.3 (low): Fail fast with a clear error on invalid `OTEL_TRACES_SAMPLER`

**Priority: Low**

**Suggested commit**:
```
fix(exporter): fail fast on invalid OTEL_TRACES_SAMPLER value (FR-8.3)
```

### Current state (evidence)

- `parseSampler` (`exporter/config.go:259-269`) maps an unsupported env value to the magic string `"invalid:<value>"`, which is silently stored in `Config.Sampler` and only rejected later by `Validate()` with a generic message. The user sees neither the original value nor the allowed set at the point of failure.

### Required behavior

1. Keep the deferred-validation architecture (ConfigFromEnv stays error-free by design) but make the failure self-explanatory: `Validate()` must produce an error message containing the **original env value** and the allowed set `always_on | always_off | traceidratio`. Either keep the `invalid:` marker and unpack it in the validation message, or store the raw value plus a `samplerSource` note — implementer's choice, but the resulting message must read like: `sampler: unsupported value "parentbased_always_on" from OTEL_TRACES_SAMPLER (allowed: always_on, always_off, traceidratio)`.
2. README Configuration section: document the three allowed sampler values and `samplerArg` range in the existing sampler paragraph.

### Acceptance criteria

- Unit test: `OTEL_TRACES_SAMPLER=bogus` → pipeline construction fails with a message containing both `bogus` and `traceidratio`.
- Existing sampler tests keep passing unchanged (no public behavior change for valid values).

---

## Task 5 — FR-5.1 (low): Defensive fallback in `PickJourney`

**Priority: Low**

**Suggested commit**:
```
fix(journey): defensive fallback in PickJourney weight selection (FR-5.1)
```

### Current state (evidence)

- `journey/engine.go:115`: the final fallback `return e.impl.journeyKeys[len(e.impl.journeyKeys)-1]` does not check the journey's weight. With validated topologies (weight > 0 enforced at `topology/validate.go:415-417`) this is unreachable, but the Engine can be constructed against hand-built schemas in tests, and floating-point cumulative comparison makes the fallback the real handler for `roll == total` edge cases.

### Required behavior

1. Replace the fallback with a reverse scan returning the **last journey with `Weight > 0`**; return `""` if none (mirrors the existing `total <= 0` early return).
2. Add a PBT case mixing zero-weight and positive-weight journeys (construct `topology.Schema` directly, bypassing `Parse`, as `journey/pick_test.go` already does) asserting the returned name always has positive weight.

### Acceptance criteria

- New PBT passes; existing `journey/pick_test.go` seeded-frequency test unchanged and passing.

---

## Task 6 — NFR-6.1 (low): Complete the README topology YAML reference

**Priority: Low**

**Suggested commit**:
```
docs(readme): complete topology YAML reference (NFR-6.1)
```

### Current state (evidence)

- README's "Topology YAML Reference" documents one fault kind (`error_rate_override` in the Features section) but not the other three (`latency_inflation`, `disconnect`, `crash`) nor the `severity` parameters (`probability`, `multiplier`, `add`, `value`). Journey `weight` and edge `timeout` are absent from the reference (weight appears only in the opening example). The authoritative enums live in `topology/enums.go:91-119` and the JSON Schema.

### Required behavior

1. Add a "Faults" subsection to the YAML reference: a table of the four fault kinds with target syntax (`service:`/`operation:`/`edge:` prefixes — check `topology/validate.go:236-285` for exact target grammar) and which severity fields each kind reads, plus one YAML example per kind (keep them short).
2. Document `weight` in the journeys part of the reference (default 1.0, used by `runRandomJourney()`), and `timeout` in the edge/calls part (zero = disabled; on exceed the span is clamped and marked error per FR-7.1).
3. Cross-check every documented field name against `topology/raw.go` YAML tags — the doc must not invent fields.

### Acceptance criteria

- All four fault kinds and the `severity` fields appear in README and match `topology/enums.go` / the JSON Schema exactly.
- `go run ./cmd/xk6-otel-gen-schema` output unchanged (this task is docs-only).

---

## Task 7 — Record approved design deviations

**Priority: Low**

**Suggested commit**:
```
docs(aidlc): record approved design deviations from round-2 review
```

### Required behavior

Create `aidlc-docs/construction/remediation/design-deviations.md` recording three decisions made by the user on 2026-06-11 (do NOT modify the original FD documents beyond an optional pointer line):

1. **Single-span-per-hop model (vs U3 FD client/server pairs)**: `aidlc-docs/construction/u3-synth/functional-design/business-logic-model.md:156-157` envisions a CLIENT span at the caller and a SERVER span at the callee per app→app edge. The implementation creates one node per hop (`journey/plan.go:127` builds nodes from `Edge.To` only) so `inferDirection` (`synth/synthesizer.go:272-291`) yields SERVER for app→app hops; CLIENT direction fires only for database/cache/external_api/queue targets, and `http.client.request.duration` never fires for app→app. **Decision: keep the current model.** Consequences to note: span count stays lower; OTel Collector service-graph connectors that require client/server pairs may not derive edges from app→app hops (parent-child-based derivation still works). Future option: caller-side client spans as an opt-in.
2. **No probabilistic call firing**: FR-5.1's example "一部の依存は条件分岐で発火する" is satisfied via weighted multi-journey selection; per-call `probability` is recorded as a future extension, not a gap.
3. **k6 version compatibility CI matrix (NFR-3.1)**: deferred together with the other CI workflow artifacts that the Build and Test stage explicitly left as post-stage work.

### Acceptance criteria

- The deviations file exists, names the requirement IDs, the evidence locations, the decision, the date, and the decision-maker (user).
- `aidlc-docs/audit.md` is NOT modified by this task (the orchestrating agent maintains the audit log).

---

## Explicitly NOT in scope (do not implement)

- **Client/server span pairs for app→app edges** — user decided to keep the single-span model (Task 7 records this).
- **Probabilistic per-call firing** — future extension (Task 7 records this).
- **k6 version compatibility CI matrix / CI workflow YAMLs** — deferred post-stage work.
- **Mermaid / Service Graph JSON / Kubernetes manifest input** — descoped by approved requirement decision Q2/A-1.
- Any change to the YAML topology schema fields (this round adds none; JSON Schema must remain byte-identical to `go run ./cmd/xk6-otel-gen-schema` output).

## Final verification (after all tasks, before reporting done)

```bash
go build ./...
go test -race -count=1 ./...
golangci-lint run
go run ./cmd/xk6-otel-gen-schema > /tmp/schema.json && diff /tmp/schema.json topology/jsonschema/topology.schema.json
go test ./test/examples/...
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=. && ./k6 version
git log --oneline   # must show one commit per completed task, in order
```
