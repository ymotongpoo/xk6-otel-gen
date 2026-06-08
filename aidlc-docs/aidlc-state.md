# AI-DLC State Tracking

## Project Information
- **Project Name**: xk6-otel-gen
- **Project Type**: Greenfield
- **Start Date**: 2026-06-07T00:00:00Z
- **Current Stage**: INCEPTION - Requirements Analysis
- **Description**: A k6 extension that consumes a declarative description of microservice component relationships (YAML / Mermaid) and synthesizes pseudo OpenTelemetry telemetry signals (metrics, logs, distributed traces), sending them to an OTLP endpoint — without requiring real microservices to exist.

## Workspace State
- **Existing Code**: No
- **Reverse Engineering Needed**: No
- **Workspace Root**: /home/ymotongpoo/repos/xk6-otel-gen
- **Programming Languages**: (to be determined — likely Go, as k6 extensions use xk6/Go)
- **Build System**: (to be determined)
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
- [ ] Functional Design — EXECUTE
- [ ] NFR Requirements — EXECUTE
- [ ] NFR Design — EXECUTE
- [ ] Infrastructure Design — SKIPPED (binary distribution, no IaC)
- [ ] Code Generation — EXECUTE
- [ ] Build and Test — EXECUTE

### 🟡 OPERATIONS PHASE
- [ ] Operations — PLACEHOLDER

## Current Status
- **Lifecycle Phase**: CONSTRUCTION
- **Current Unit**: U7 (testutil/generators) — NFR Requirements (Part 1: plan + questions awaiting answers)
- **Construction Progress**: U7 [FD done | NFR-R: in progress] ← U1 ← U4 ← U3 ← U2 ← U5 ← U6 ← U8
- **Status**: U7 FD (fea577c), NFR-R (7bdf3c3), NFR-D (f9fcc99) committed. Code Generation Planning complete — `code-generation-plan.md` ready for Codex CLI / Cursor execution. Awaiting approval to start Code Generation (Generation part — handed off to implementation agents).

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
