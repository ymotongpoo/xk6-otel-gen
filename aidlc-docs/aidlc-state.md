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
- **Lifecycle Phase**: INCEPTION
- **Current Stage**: Workflow Planning (awaiting approval)
- **Next Stage**: Application Design
- **Status**: Ready to proceed pending user approval
