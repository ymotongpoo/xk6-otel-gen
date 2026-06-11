# U8 (Samples & Distribution) — Code Generation Plan

> **This file is the Single Source of Truth (SSOT) for the U8 implementation. Final unit of Construction phase.**
>
> **Audience**: Codex CLI (`gpt-5.5 xhigh`) + Cursor Composer 2.5.
>
> **Recommended agent per Phase**:
> - **Phase 0-12 → Codex** via `scripts/run-codex.sh u8`. No U1-U7 coordination patches needed.
> - **Phase 5 (astroshop topology) MAY be split off to Cursor batch** — large pattern-following YAML authoring.
>
> **Execution model**: top-to-bottom, mark `[ ]` → `[x]` immediately upon completion.
>
> **Source artifacts**:
> - FD: `aidlc-docs/construction/u8-samples/functional-design/{business-logic-model,business-rules,domain-entities}.md`
> - NFR-R: `aidlc-docs/construction/u8-samples/nfr-requirements/{nfr-requirements,tech-stack-decisions}.md`
> - NFR-D: `aidlc-docs/construction/u8-samples/nfr-design/{nfr-design-patterns,logical-components}.md` ← **most prescriptive**
> - Application Design: `aidlc-docs/inception/application-design/unit-of-work.md` §U8
> - U1 topology package: `topology/` (Parse, Validate, ApplyFaults, ExportJSONSchema)
> - U5 k6otelgen: JS API reference for `examples/*/script.js`
> - U6 k6output: `--out otel-gen=...` args reference for k6 invocations
> - PBT rules: not applicable to U8
> - Agent contract: `AGENTS.md`

---

## Unit Context

- **Unit ID**: U8 (final unit)
- **Purpose**: Distribute the project — examples/, cmd/xk6-otel-gen-schema/, project root README/LICENSE, AI maintenance skill, CI configs.
- **Workspace root**: `/home/ymotongpoo/repos/xk6-otel-gen/`
- **Go module path**: `github.com/ymotongpoo/xk6-otel-gen`
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → U2 ✓ → U5 ✓ → U6 ✓ → **U8 (this — final unit)**
- **PBT requirements**: N/A (Q12 in FD)
- **NFR DoD** (from `nfr-requirements.md` §4):
  - `go build ./cmd/...` succeeds
  - `go vet ./cmd/...` clean
  - `go test -race -count=1 ./cmd/...` passes
  - `go test -cover ./cmd/...` shows ≥ 70%
  - `xk6 build --with .` succeeds (if xk6 available in CI env)
  - `golangci-lint run ./cmd/...` passes (with SPDX header enforce)
  - `examples/*/topology.yaml` passes `topology.Parse + Validate` via `go test ./test/examples/...`
  - `kubectl apply --dry-run=server -k examples/*/k8s/` clean (cluster-available env)
  - README.md (project root) has 12 sections with concrete example per section
  - LICENSE at project root (Apache-2.0 fulltext)
  - `.claude/skills/sync-astroshop/SKILL.md` exists with proper frontmatter
  - `.github/dependabot.yml`, `.lychee.toml`, `.goheader.txt` exist
  - `lychee` link check passes for README + per-example README
- **Dependencies added by this unit**:
  - Zero new Go external deps (cmd uses stdlib + topology)
- **No U1-U7 coordination patches needed**: U8 is purely additive (consumes U1's ExportJSONSchema, references U5/U6 in docs).

---

## Phase 0 — Environment Setup
**Recommended agent**: Codex CLI.

### Step 0.1 — Verify project state

- [x] Confirm `go build ./...` succeeds at current main HEAD.
- [x] Confirm `topology.ExportJSONSchema()` is available in `topology/` package.

### Step 0.2 — Create U8 directory skeleton

- [x] Create `cmd/xk6-otel-gen-schema/` directory.
- [x] Create `examples/minimal/` and `examples/astroshop/` (with `k8s/` subdirs each).
- [x] Create `test/examples/` directory.
- [x] Create `.claude/skills/sync-astroshop/` directory.

### Phase 0 commit

- [x] `git add cmd/xk6-otel-gen-schema/ examples/ test/examples/ .claude/skills/ && git commit -m "build(samples): scaffold U8 directories"` (only creates empty dirs via `.gitkeep` or via the next phase files)

---

## Phase 1 — cmd/xk6-otel-gen-schema/ Implementation (LC-1)
**Recommended agent**: Codex CLI.

### Step 1.1 — Create `cmd/xk6-otel-gen-schema/main.go`

- [x] Implement `main()` as thin wrapper calling `os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))` per NFR-D §1.1.
- [x] Implement `run(args []string, stdout, stderr io.Writer) int` with:
  - `flag.NewFlagSet` with `-output` flag
  - `flag.ContinueOnError` returns exit code 2 on parse failure
  - call `topology.ExportJSONSchema()`, return 1 on error
  - write to stdout or `-output` file, return 1 on write failure
  - return 0 on success
- [x] Add SPDX header at top of file (`// SPDX-License-Identifier: Apache-2.0`).
- [x] Add package GoDoc comment describing the CLI.

### Step 1.2 — Create `cmd/xk6-otel-gen-schema/main_test.go`

- [x] `TestRun_StdoutDefault` — no flags → stdout contains `$schema` marker.
- [x] `TestRun_OutputToFile` — `-output <tmpfile>` → file exists with schema content.
- [x] `TestRun_FlagParseError` — `-unknown` → exit 2.
- [x] `TestRun_FileCreateFailure` — invalid path (e.g., `/dev/null/x/schema.json`) → exit 1.
- [x] `TestRun_HelpFlag` — `-h` or `-help` should be handled gracefully by `flag.ContinueOnError`.
- [x] All tests call `t.Parallel()`.
- [x] Use `testing.TempDir()` for file outputs.
- [x] Add SPDX header.

### Step 1.3 — Verify coverage

- [x] Run `go test -cover ./cmd/...` and confirm ≥ 70%.

### Phase 1 commit

- [x] `git add cmd/xk6-otel-gen-schema/main.go cmd/xk6-otel-gen-schema/main_test.go && git commit -m "feat(cmd): add xk6-otel-gen-schema CLI for JSON Schema export"`

---

## Phase 2 — Examples Validation Test (LC-4)
**Recommended agent**: Codex CLI.

### Step 2.1 — Create `test/examples/examples_test.go`

- [x] `TestExamples_TopologyValidates`:
  - Loop over `os.ReadDir("../../examples")`
  - For each directory entry, run sub-test
  - Read `<dir>/topology.yaml`, call `topology.Parse` + `Schema.Validate`
  - Assert no errors
- [x] Add SPDX header.
- [x] Test marked `t.Parallel()` at top + each sub-test also parallel.

> **NOTE**: This test will skip example directories that don't yet exist (Phase 3+ will add them). At Phase 2 completion, the test is in place but may pass with 0 examples; Phase 3+ make it exercise actual content.

### Phase 2 commit

- [x] `git add test/examples/examples_test.go && git commit -m "test(examples): add topology validation test for example yamls"`

---

## Phase 3 — examples/minimal/ (LC-2)
**Recommended agent**: Codex CLI.

### Step 3.1 — Create `examples/minimal/topology.yaml`

- [x] 3-tier topology (frontend / backend / database) per FD §2.
- [x] 1 journey: `checkout`.
- [x] 1 fault: `error_rate_override` on `frontend_to_backend` edge with rate=0.05.
- [x] Inline comments describing each field for first-time readers.
- [x] Must pass `topology.Parse + Validate` (verify via Phase 2 test).

### Step 3.2 — Create `examples/minimal/script.js`

- [x] 3-phase k6 script per FD §2.3:
  - `import otelgen from "k6/x/otel-gen"`
  - `export const options = { vus: 10, duration: '30s' }`
  - `setup()` calls `otelgen.configure` + `otelgen.load`
  - default function calls `data.topology.runJourney("checkout")`
  - `teardown()` is empty (with explanatory comment about U6 Output.Stop)

### Step 3.3 — Create `examples/minimal/otel-collector-config.yaml`

- [x] OTLP receiver (grpc + http)
- [x] batch processor
- [x] 3 exporters: otlp/tempo, prometheus, otlphttp/loki
- [x] 3 pipelines (traces / metrics / logs) routing to the right exporter

### Step 3.4 — Create `examples/minimal/k8s/` manifests

- [x] `kustomization.yaml` per NFR-D §3.2 with resources + configMapGenerator
- [x] `namespace.yaml` — `xk6-otel-gen-demo` namespace
- [x] `collector.yaml` — Collector Deployment + Service
- [x] `tempo.yaml` — Tempo Deployment + Service + inline ConfigMap with minimal Tempo config
- [x] `loki.yaml` — Loki Deployment + Service + inline ConfigMap with minimal Loki config
- [x] `prometheus.yaml` — Prometheus Deployment + Service + ConfigMap with scrape config (target `otel-collector:8889`)
- [x] `grafana.yaml` — Grafana Deployment + Service with volumeMounts for datasources + dashboards
- [x] `datasources.yaml` — Grafana provisioning for Tempo (default? per FD), Prometheus, Loki
- [x] `dashboard-overview.json` — 3-panel dashboard (Tempo / Prometheus / Loki) per NFR-D §4.3
- [x] All images pinned to specific tags per NFR-R §1.7 / NFR-D §4 (Tempo 2.6.0, Loki 3.2.0, Prometheus v2.55.0, Grafana 11.3.0, Collector contrib 0.105.0 — verify latest at implementation time)
- [x] resources.requests: 256Mi/100m baseline, Tempo/Loki 512Mi/100m

### Step 3.5 — Create `examples/minimal/README.md`

- [x] 7 sections per business-rules.md §4.2: Description / Prerequisites / Setup / Run / View results / Cleanup / Customize.
- [x] Code blocks with concrete commands.

### Step 3.6 — Create `examples/minimal/k8s/README.md`

- [x] kind cluster setup + kubectl apply + port-forward + cleanup commands.

### Step 3.7 — Verify

- [x] `go test ./test/examples/...` passes (minimal/topology.yaml validates).
- [x] `kubectl apply --dry-run=client -k examples/minimal/k8s/` clean (server dry-run requires cluster).

### Phase 3 commit

- [x] `git add examples/minimal/ && git commit -m "feat(examples): add minimal 3-tier example with LGTM-lite k8s stack"`

---

## Phase 4 — examples/minimal/k8s/k8s validation script (optional smoke)
**Recommended agent**: Codex CLI.

### Step 4.1 — Verify kustomize build

- [x] `kustomize build examples/minimal/k8s/ > /tmp/minimal.yaml` succeeds without error
- [x] `kubectl apply --dry-run=client -f /tmp/minimal.yaml` succeeds

(No commit — verification only. If failures, fix Phase 3 manifests.)

---

## Phase 5 — examples/astroshop/ (LC-3)
**Recommended agent**: Codex CLI or Cursor batch (large pattern-following YAML).

### Step 5.1 — Create `examples/astroshop/topology.yaml`

- [x] 18 services per FD §3.1 (14 application + 4 dependency: redis-cache / postgres / kafka / flagd).
- [x] Section comments per NFR-D §9.1 (4 groups: Frontend & API / Core commerce / Support / Infrastructure deps).
- [x] Inline 1-line description per service.
- [x] 5 journeys per FD §3.2: browse / search / add-to-cart / checkout / place-order.
- [x] Fault demonstrations per FD §3.3: payment error_rate spike, shipping latency_inflation, recommendation crash, email disconnect.
- [x] Header comment: "Modeled after the OpenTelemetry Demo (astronomy shop) v<X.Y.Z>" — substitute current OTel Demo release tag at implementation time.
- [x] Must pass `topology.Parse + Validate`.

### Step 5.2 — Create `examples/astroshop/script.js`

- [x] k6 scenarios per FD §3.4: `browse` (vus=20, 60s) + `checkout` (vus=5, 60s) with separate `exec` functions.

### Step 5.3 — Create `examples/astroshop/otel-collector-config.yaml`

- [x] Same as minimal (3 pipelines, same exporters).

### Step 5.4 — Create `examples/astroshop/k8s/`

- [x] Same layout as minimal/k8s/.
- [x] resources.requests can be slightly larger for astroshop scale (e.g., Tempo 1Gi instead of 512Mi).
- [x] dashboard-overview.json may include a 4th panel showing 5-journey breakdown.

### Step 5.5 — Create `examples/astroshop/README.md`

- [x] Same 7 sections as minimal/README.md.
- [x] **Mention OTel Demo upstream snapshot version** in Description section (for sync skill reference).

### Step 5.6 — Create `examples/astroshop/k8s/README.md`

- [x] kind cluster setup + apply + port-forward + cleanup.

### Step 5.7 — Verify

- [x] `go test ./test/examples/...` passes for both minimal and astroshop.
- [x] `kustomize build examples/astroshop/k8s/` succeeds.

### Phase 5 commit

- [x] `git add examples/astroshop/ && git commit -m "feat(examples): add astroshop 18-service example modeled after OTel Demo"`

---

## Phase 6 — Project root README and LICENSE (LC-0)
**Recommended agent**: Codex CLI.

### Step 6.1 — Create `LICENSE`

- [x] Apache-2.0 fulltext from SPDX official source (https://www.apache.org/licenses/LICENSE-2.0.txt).

### Step 6.2 — Create `README.md` (project root)

- [x] Single-file with TOC + 12 sections per NFR-D §10.1.
- [x] Sections per business-rules.md §4.1:
  1. Project description
  2. Badges (Go version, License)
  3. Quick Start (5-step xk6 build + kind + apply + k6 run)
  4. Features
  5. Building (`xk6 build --with github.com/ymotongpoo/xk6-otel-gen`)
  6. Usage — JS API summary (`otelgen.configure`, `load`, `handle.runJourney`, `stats`, `journeys`) + link to examples
  7. Topology YAML Reference — brief overview + `xk6-otel-gen-schema` usage
  8. Configuration — JS API > `--out` > env > default priority table
  9. Examples — link to examples/minimal/, examples/astroshop/
  10. Security — self-build-only rationale
  11. Contributing — brief guideline (TODO: link to CONTRIBUTING.md if exists later)
  12. License — Apache-2.0
  13. Compatibility — minimum version table (Go 1.25+, kubectl 1.27+, kind 0.20+, Docker latest)
- [x] TOC at top with markdown anchors to each section.
- [x] Each section has at least one concrete code example or table.

### Phase 6 commit

- [x] `git add README.md LICENSE && git commit -m "docs(project): add full project README and Apache-2.0 LICENSE"`

---

## Phase 7 — `.claude/skills/sync-astroshop/SKILL.md` (LC-5)
**Recommended agent**: Codex CLI.

### Step 7.1 — Create SKILL.md

- [x] Frontmatter with `name: sync-astroshop` and detailed `description`.
- [x] Body markdown per NFR-D §7.1:
  - When to use
  - Out of scope (daily/monthly auto sync, code dep updates)
  - Steps (4 numbered steps: Survey upstream / Diff astroshop / Propose changes / Apply checklist)
  - Anti-patterns (no 1:1 reproduction, no blocking on minor churn)
  - Output (Markdown summary for PR description)
- [x] Reference upstream repo: `open-telemetry/opentelemetry-demo`
- [x] Reference local file: `examples/astroshop/topology.yaml`
- [x] Description mentions checklist items.

### Phase 7 commit

- [x] `git add .claude/skills/sync-astroshop/SKILL.md && git commit -m "docs(skill): add sync-astroshop AI maintenance skill"`

---

## Phase 8 — CI Config (LC-6)
**Recommended agent**: Codex CLI.

### Step 8.1 — Create `.github/dependabot.yml`

- [x] Per NFR-D §5.1:
  - gomod weekly grouped (`otel`, `k6` patterns)
  - docker monthly for `examples/minimal/k8s/`
  - docker monthly for `examples/astroshop/k8s/`
  - github-actions weekly

### Step 8.2 — Create `.lychee.toml`

- [x] exclude `localhost`, `127.0.0.1`, `example.com`, `example.org`
- [x] max-retries = 3, timeout = 10
- [x] include-fragments = true
- [x] exclude_path = `node_modules`, `.git`

### Step 8.3 — Create `.goheader.txt`

- [x] Single line: `SPDX-License-Identifier: Apache-2.0`

### Step 8.4 — Update `.golangci.yml`

- [x] If `.golangci.yml` exists at project root, add `goheader` linter to enabled list + `linters-settings.goheader.template-path: .goheader.txt`
- [x] If `.golangci.yml` doesn't exist, create with project-standard config + goheader

### Step 8.5 — Verify lint passes

- [x] `golangci-lint run ./cmd/...` passes (all .go files have SPDX header)
- [x] If existing files (U1-U7) lack SPDX header, backfill or update goheader template to be lenient

### Phase 8 commit

- [x] `git add .github/dependabot.yml .lychee.toml .goheader.txt .golangci.yml && git commit -m "build(ci): add dependabot, lychee, goheader configs"`

---

## Phase 9 — SPDX header backfill (cross-cutting)
**Recommended agent**: Codex CLI.

> If Phase 8 reveals U1-U7 .go files missing SPDX headers, backfill them now (since goheader linter would fail CI otherwise).

### Step 9.1 — Audit and backfill

- [x] Run `grep -L "SPDX-License-Identifier" $(find . -name '*.go' -not -path './vendor/*')` to list files lacking SPDX header.
- [x] For each missing file, prepend `// SPDX-License-Identifier: Apache-2.0\n\n` before the `package` declaration.
- [x] Verify `golangci-lint run` clean for the entire project.

### Phase 9 commit (only if files modified)

- [x] `git add <modified .go files> && git commit -m "chore(license): backfill SPDX headers across all packages"`

---

## Phase 10 — Final wrap & DoD verification
**Recommended agent**: Codex CLI.

### Step 10.1 — Run full suite

- [x] `go build ./...` succeeds.
- [x] `go vet ./cmd/... ./test/examples/...` clean.
- [x] `go test -race -count=1 ./...` passes (including cmd and examples tests).
- [x] `go test -cover ./cmd/...` shows ≥ 70%.
- [x] `golangci-lint run` (full repo) passes including goheader enforce.
- [x] `kustomize build examples/minimal/k8s/` succeeds.
- [x] `kustomize build examples/astroshop/k8s/` succeeds.
- [x] `xk6 build --with .` succeeds (if xk6 available locally).
- [x] If `lychee` is available: `lychee --config .lychee.toml README.md 'examples/**/README.md'` passes.

### Step 10.2 — Create `aidlc-docs/construction/u8-samples/code/code-generation-summary.md`

- [x] File list with line counts.
- [x] Verification results.
- [x] Deviations from plan.
- [x] Recent commits.

### Step 10.3 — Mark all plan checkboxes [x]

- [x] Walk back through this plan; verify all `[ ]` are `[x]`.

### Step 10.4 — Update `aidlc-docs/aidlc-state.md`

- [x] Mark **U8 complete**.
- [x] **Mark all CONSTRUCTION phase units complete**.
- [x] Next stage: **Build and Test** (per AIDLC workflow).

### Phase 10 commit

- [x] `git add aidlc-docs/ && git commit -m "chore(u8-samples): finalize code-generation-summary and complete CONSTRUCTION phase"`

---

## Anti-patterns to AVOID during implementation

(Per NFR-D `nfr-design-patterns.md` §13, 14 items)

- ❌ Single `main()` test-impossible (use `main` thin + `run()` testable)
- ❌ interface-based Runner (over-engineering)
- ❌ examples/_test.go per directory (use `test/examples/` shared)
- ❌ kustomize base+overlays (use kustomize-friendly per-service single manifests)
- ❌ Helm chart (FD Q7 rejected)
- ❌ inline Grafana ConfigMap in grafana.yaml (use configMapGenerator)
- ❌ Multi-panel rich dashboard (use 3-panel minimum viable)
- ❌ Separate dependabot config files (single .github/dependabot.yml)
- ❌ markdown-link-check (use lychee, Rust binary)
- ❌ Shell grep for SPDX (use goheader linter)
- ❌ Multiple YAML files for one topology (Schema is single YAML)
- ❌ Multi-file README with docs/ subdir (use single-file with TOC)
- ❌ cleanup.sh / Makefile (use README copy-paste commands)
- ❌ examples flat layout (use examples/<example>/k8s/ structure)

---

## Notes for the implementing agent

1. **No U1-U7 coordination patches needed**: U8 consumes existing `topology.ExportJSONSchema()` and is otherwise additive. SPDX header backfill in Phase 9 is the only "touches other units" work, and it's a chore-grade non-functional change.
2. **Test directory naming**: `test/examples/` rather than `examples/test/` to keep examples/ free of Go files (cleaner for user inspection).
3. **kustomize commands**: prefer `kustomize build` over `kubectl apply -k` for offline validation. Both should work.
4. **Image tag freshness**: verify latest stable tags at implementation time (Tempo / Loki / Grafana evolve quickly). The plan lists 2.6.0 / 3.2.0 / 11.3.0 / v2.55.0 / 0.105.0 as planning-time anchors but actual implementation should use latest compatible versions.
5. **Anonymous Grafana**: anonymous Editor auth is intentional for demo convenience. Document in grafana.yaml comment that this is NOT for production.
6. **OTel Demo upstream tag**: when authoring astroshop/topology.yaml, pin the snapshot version reference (e.g., "v1.12.0") in the header comment so the AI skill knows the baseline.
7. **AI skill placement**: `.claude/skills/<name>/SKILL.md` is the Claude Code convention. Other AI tooling (Codex, Cursor) may not recognize the path — that's fine, the skill targets Claude Code first.
8. **Conventional Commits**: each phase produces one commit; include `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
9. **Sandbox mode**: `--sandbox danger-full-access` per `scripts/run-codex.sh u8`.
10. **Capacity note**: U8 is medium-large (extensive YAML + JSON dashboards + README). Should fit in 1 Codex session; natural break point after Phase 6 (README+LICENSE) if needed.

---

## After U8: Construction → Build and Test stage

Once U8 is complete, the AIDLC workflow transitions from CONSTRUCTION to the **Build and Test** stage. That stage's responsibilities (out of U8 scope but worth flagging):

- Define CI workflows (`.github/workflows/*.yml`) that run the cmd tests, examples validation, lychee link check, kustomize build, xk6 build, integration tests
- Configure release automation (if/when releases are cut)
- Set up SECURITY.md (placeholder mentioned in NFR-U8-10)
- Set up CONTRIBUTING.md (placeholder mentioned in README §11)
- Set up CODEOWNERS / issue templates / PR templates

These are explicitly **out of scope for U8 Code Generation**; they belong to the post-Construction Build and Test stage.
