# Build and Test — Summary

本書は xk6-otel-gen project の **Build and Test stage の全体像** を要約する。
Construction phase が完了し、deliverable が build / test 可能な状態にあることを記録する。

## 1. Construction Phase 完了状況

全 8 unit が **Functional Design + NFR Requirements + NFR Design + Code Generation** を完了:

| Unit | Package | Production Lines | Test Lines | Coverage | Key Bench |
|---|---|---:|---:|---:|---|
| U7 | `testutil/generators/` | (PBT generators) | — | — | — |
| U1 | `topology/` | (Parse/Validate/ApplyFaults/Lint/ExportJSONSchema) | — | ≥ 80% | BenchmarkParse |
| U4 | `exporter/` | 8 files + 7 test | + integration | 82.5% | BenchmarkNew 6.8ms |
| U3 | `synth/` | 7 files | + integration | 84.0% | BeginSpan 7.3µs |
| U2 | `journey/` | 8 files | + integration | 80.9% | Execute 2µs/step |
| U5 | `k6otelgen/` | 6 files | + integration | 82.2% | NewModuleInstance 5.5µs |
| U6 | `k6output/` | 5 files | + integration | 86.6% | AddMetricSamples 84ns |
| U8 | `cmd/` + `examples/` + docs | 2 file cmd + ~2,300 lines samples | + cmd test | 86.4% | (no bench) |

合計: **production code ~3,000 行 + test code ~5,500 行 + samples/docs ~3,300 行**。

## 2. Document Suite

本 Build and Test stage は以下の **5 instruction files** を提供:

| File | 内容 |
|---|---|
| `build-instructions.md` | Go module / xk6 / kustomize の build target、prerequisites、build verification checklist |
| `unit-test-instructions.md` | 全 unit の test target、coverage 目標、PBT 適用、CI workflow snippet |
| `integration-test-instructions.md` | 各 unit の integration test (Docker Collector + xk6 build) の harness、CI workflow snippet |
| `performance-test-instructions.md` | benchmark inventory + strict/soft 区別、regression detection、profiling 手順 |
| `build-and-test-summary.md` | 本 file — overall summary |

## 3. CI Workflow Recommendations

Build and Test stage では CI の推奨構成を設計した。
その後、実際の workflow YAML も `.github/workflows/` に配置済み。

| Workflow | Trigger | Target |
|---|---|---|
| `.github/workflows/ci.yml` | PR + push to main | build, race tests, Go lint, markdown lint |
| `.github/workflows/docs.yml` | push to main + manual | Hugo docs build and GitHub Pages deploy |
| `.github/workflows/security-scanners.yml` | push / PR / schedule / manual | gitleaks, semgrep, grype, bandit |
| `.github/workflows/release*.yml` / `tag-on-merge.yml` | manual / release branch / tag | release PR, tag, and binary release |

詳細 snippet は各 instruction file の "CI Workflow" section 参照。

## 4. DoD (Definition of Done) Aggregate

各 unit の DoD を aggregate した project-wide DoD。
`reverified 2026-06-19` は本設計レビュー修正時に再実行した項目を示す。
`stage-recorded` は各 unit の code-generation summary または Build and Test audit に記録済みの項目を示す。

- [x] `go build ./...` succeeds (`reverified 2026-06-19`)
- [x] `go vet ./...` clean (`reverified 2026-06-19` with `GOFLAGS=-buildvcs=false`)
- [x] `go test -race -count=1 ./...` passes (full repo, `reverified 2026-06-19` with `GOFLAGS=-buildvcs=false`)
- [x] `go test -cover ./...` shows per-package coverage targets met (80% 標準 / 70% for cmd, `stage-recorded`)
- [x] `go test -bench=. -benchmem ./...` shows strict benchmarks within NFR budget (`stage-recorded`)
- [x] `golangci-lint run` clean (SPDX header enforce 含む, `reverified 2026-06-19`)
- [x] `xk6 build --with .` succeeds (`reverified 2026-06-19`, output under `/tmp`)
- [x] `kustomize build examples/{minimal,astroshop}/k8s/` succeeds (`reverified 2026-06-19`)
- [x] Integration tests (`-tags=integration`) pass with Docker + xk6 available (`stage-recorded`; not rerun in the 2026-06-19 doc-fix pass)
- [ ] README link check (lychee) passes (not rerun in the 2026-06-19 doc-fix pass)
- [x] All exported identifiers have GoDoc (`stage-recorded`)
- [x] 3 Example functions per major unit (U2-U6) (`stage-recorded`)
- [x] All `.go` files have SPDX header (`stage-recorded`)
- [x] All examples/topology.yaml passes topology.Parse + Validate (`reverified 2026-06-19` through `go test ./...`)
- [x] `.claude/skills/sync-astroshop/SKILL.md` exists with proper frontmatter (`stage-recorded`)

### 4.1 Current Review Verification

The design-review cleanup on 2026-06-19 re-ran these checks:

```bash
go build ./...
GOFLAGS=-buildvcs=false go vet ./...
GOFLAGS=-buildvcs=false go test ./...
GOFLAGS=-buildvcs=false go test -race -count=1 ./...
golangci-lint run
npx --yes markdownlint-cli2 "**/*.md" # uses .markdownlint-cli2.yaml; aidlc-docs/** is ignored there
GOFLAGS=-buildvcs=false go run ./cmd/xk6-otel-gen-schema > /tmp/xk6-otel-gen-schema.json
diff -u topology/jsonschema/topology.schema.json /tmp/xk6-otel-gen-schema.json
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=. --output /tmp/xk6-otel-gen-review-k6
kustomize build examples/minimal/k8s/
kustomize build examples/astroshop/k8s/
```

## 5. Quality Highlights

### 5.1 Performance achievements (vs budget)

| Metric | Budget | Achieved | Margin |
|---|---|---|---|
| journey BuildPlan | 1 ms | 15.46 ns | **64,683x** under |
| journey Execute per-step | 50 µs | 2,065 ns | **24x** under |
| k6output AddMetricSamples | 1 µs | 84.41 ns | **12x** under (zero alloc) |
| synth RecordMetric | 5 µs | 1,531 ns | **3.3x** under |
| k6otelgen NewModuleInstance | 5 ms | 5.5 µs | **~1000x** under |

### 5.2 Test coverage

すべての unit が target を上回る coverage を達成 (lowest is journey 80.9%, highest cmd 86.4%)。

### 5.3 PBT compliance

Initial Construction implemented and verified 22 testable properties (TP-U1-1..8, TP-U2-1..5, TP-U3-1..4, TP-U4-1..4, TP-U5-1..3, TP-U6-1..3).
Later remediation and endpoint-config work added further rapid-backed properties, including timeout and retry invariants, weighted journey selection invariants, TLS/config invalid-combination generation, and endpoint-resolution TP-U4-5..7.
The current status is "PBT full enforcement maintained"; the 22-property count is the initial Construction baseline, not the final project total.

### 5.4 Coordination patches successfully landed

複数の cross-unit coordination patches が clean に landing:
- U2 `NewEngineWithSeed` (for U5)
- U3 `Outcome.Cascaded` field (for U2 cascade emit)
- U4 `Pipeline.MetricExporter()` (for U6 runner MeterProvider)
- U5/U6 integration test guards で順序依存を吸収

## 6. Out of Scope for Build and Test (handled by next stage / separate process)

これらは **本 stage の deliverable ではない**:

| Item | Note |
|---|---|
| `.github/workflows/*.yml` actual files | recommendations は本書、actual YAML は別 implementation task |
| GitHub release automation | post-v1 で release pipeline 設計 |
| `SECURITY.md` | NFR-U8-10 で placeholder 言及、project が成熟したら |
| `CONTRIBUTING.md` | README §11 で言及、project 成熟段階で詳細化 |
| CODEOWNERS / issue templates / PR templates | community 形成段階で追加 |
| SBOM generation (cyclonedx-gomod) | future revisit |
| Pre-built binary signing (cosign) | self-build only 方針なので不要 (rejected) |

## 7. Pre-flight Verification Checklist (本 stage 完了前)

本 stage を完了 (Operations stage に進む) 前に確認:

- [x] 5 instruction files が `aidlc-docs/construction/build-and-test/` に存在
- [x] 全 unit の code generation が complete
- [x] aidlc-state.md が "Build and Test" stage を反映
- [x] audit.md が本 stage の進行を記録
- [ ] git working tree が clean (commit 済)

## 8. Next Stage: Operations (PLACEHOLDER)

AIDLC workflow per CLAUDE.md:
> **Status**: This stage is currently a placeholder for future expansion.
> The Operations stage will eventually include:
> - Deployment planning and execution
> - Monitoring and observability setup
> - Incident response procedures
> - Maintenance and support workflows
> - Production readiness checklists

→ **本 project では Operations は placeholder のまま**、Construction + Build and Test 完了で AIDLC workflow としては full pass。

## 9. Project Status Summary

```
🟦 INCEPTION PHASE     ✅ Complete (Requirements / Application Design / Units Generation)
🟩 CONSTRUCTION PHASE  ✅ Complete (8 units × FD/NFR-R/NFR-D/CG)
🟦 BUILD AND TEST       ✅ Complete (5 instruction files generated)
🟨 OPERATIONS           ⏸ Placeholder (no work in this iteration)
```

xk6-otel-gen project は **production-ready state** に到達。User が xk6 build + kind cluster + k6 run で immediate に試せる。
