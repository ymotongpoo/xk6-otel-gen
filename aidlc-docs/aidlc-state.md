# AI-DLC State Tracking

## Project Information
- **Project Name**: xk6-otel-gen
- **Project Type**: Greenfield
- **Start Date**: 2026-06-07T00:00:00Z
- **Completion Date**: 2026-06-11 (initial delivery)
- **Current Stage**: CHANGE REQUEST — Per-Signal Endpoint Support (see "Active Change Request" below)
- **Description**: A k6 extension that consumes a declarative description of microservice component relationships (YAML / Mermaid) and synthesizes pseudo OpenTelemetry telemetry signals (metrics, logs, distributed traces), sending them to an OTLP endpoint — without requiring real microservices to exist.

## Workspace State
- **Existing Code**: Yes (delivered through AIDLC workflow)
- **Reverse Engineering Needed**: No
- **Workspace Root**: /home/ymotongpoo/repos/xk6-otel-gen
- **Programming Languages**: Go 1.25+
- **Build System**: Go modules + xk6 + kustomize
- **Project Structure**: Empty

## Code Location Rules
- **Application Code**: Workspace root (NEVER in aidlc-docs/)
- **Documentation**: aidlc-docs/ only
- **Structure patterns**: See code-generation.md Critical Rules

## Extension Configuration
| Extension | Enabled | Mode | Decided At |
|---|---|---|---|
| Security Baseline | No | — | Requirements Analysis |
| Resiliency Baseline | No | — | Requirements Analysis |
| Property-Based Testing | Yes | Full (all rules blocking) | Requirements Analysis |

## Multi-Agent Workflow Policy
- **Claude Code**: planning + design only — all Markdown deliverables under `aidlc-docs/**`; does NOT write application code.
- **OpenAI Codex CLI (gpt-5.5 xhigh)**: autonomous batch implementation per unit; reads `aidlc-docs/construction/<unit>/code/code-generation-plan.md` and `AGENTS.md`.
- **Cursor Composer 2.5**: interactive edits, reviews, refactors; rules in `.cursor/rules/*.mdc`.
- Config files: `AGENTS.md`, `.codex/config.toml`, `.cursor/rules/{00-project-handoff,10-go-conventions,20-pbt-enforcement,30-otel-semantic-conventions}.mdc` — decided at Workflow Planning (2026-06-07).

## Execution Plan Summary
- **Total Stages**: 11 (including conditional & placeholder)
- **Stages to Execute**: Workspace Detection, Requirements Analysis, Workflow Planning, Application Design, Units Generation, Functional Design (per unit), NFR Requirements (per unit), NFR Design (per unit), Code Generation (per unit), Build and Test
- **Stages to Skip**: Reverse Engineering (greenfield), User Stories (single-stakeholder OSS lib), Infrastructure Design (no IaC/cloud resources)

## Stage Progress

### 🔵 INCEPTION PHASE
- [x] Workspace Detection
- [x] Reverse Engineering — SKIPPED (Greenfield)
- [x] Requirements Analysis
- [x] User Stories — SKIPPED
- [x] Workflow Planning
- [ ] Application Design — EXECUTE
- [ ] Units Generation — EXECUTE

### 🟢 CONSTRUCTION PHASE (per unit)
- [x] Functional Design
- [x] NFR Requirements
- [x] NFR Design
- [x] Infrastructure Design — SKIPPED (binary distribution, no IaC)
- [x] Code Generation
- [x] Build and Test

### 🟡 OPERATIONS PHASE
- [ ] Operations — PLACEHOLDER

## Current Status
- **Lifecycle Phase**: BUILD AND TEST (complete)
- **Completed Units**: U7, U1, U4, U3, U2, U5, U6, U8
- **Construction Progress**: [✓ U7 complete] → [✓ U1 complete] → [✓ U4 complete] → [✓ U3 complete] → [✓ U2 complete] → [✓ U5 complete] → [✓ U6 complete] → [✓ U8 complete]
- **Status**: All CONSTRUCTION units complete. Build and Test stage produced 5 instruction files (build / unit-test / integration-test / performance-test / summary) under aidlc-docs/construction/build-and-test/. Next stage is Operations (PLACEHOLDER per AIDLC workflow).

## Unit Inventory
- **U1**: Topology Schema & Parser (`topology/`)
- **U2**: Journey Engine (`journey/`)
- **U3**: Signal Synthesizer (`synth/`)
- **U4**: OTLP Exporter Pipeline incl. shared Pipeline holder (`exporter/`)
- **U5**: k6 JS Module (`k6otelgen/`)
- **U6**: k6 Output Module (`k6output/`)
- **U7**: PBT Test Utilities (`testutil/generators/`)
- **U8**: Samples & Distribution (`examples/`, `cmd/`, build config)

## Construction Order
U7 (skeleton) → U1 → U4 → U3 → U2 → U5 → U6 → U8 (Q2=A bottom-up + Q4=A U7 先行 + Q6=A U8 末尾 + Q3=A 完全逐次)

## Active Change Request — Per-Signal Endpoint Support (2026-06-12)
- **Requirements**: aidlc-docs/inception/requirements/endpoint-config-requirements.md (approved, commit 46a38dd)
- **Execution Plan**: aidlc-docs/inception/plans/endpoint-config-execution-plan.md
- **Affected Units**: U4 exporter → U5 k6otelgen → U6 k6output → U8 examples/README (sequential)

### Stage Progress (this change request)
- [x] Requirements Analysis
- [x] User Stories — SKIPPED (single-stakeholder config feature)
- [x] Workflow Planning — approved 2026-06-12
- [ ] Application Design — SKIP (no new components)
- [ ] Units Generation — SKIP (existing unit inventory reused)
- [x] Functional Design (U4 only) — approved 2026-06-12
- [ ] NFR Requirements — SKIP (NFRs captured in requirements doc)
- [ ] NFR Design — SKIP
- [ ] Infrastructure Design — SKIP (no infra)
- [ ] Code Generation (U4 → U5 → U6 → U8) — EXECUTE
- [ ] Build and Test — EXECUTE
