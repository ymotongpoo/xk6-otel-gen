---
name: user-tooling-preferences
description: User's multi-agent tooling setup for xk6-otel-gen — Claude plans, Codex CLI implements autonomously, Cursor Composer edits interactively
metadata:
  type: user
---

For the **xk6-otel-gen** project the user has established a strict 3-agent division of labor:

- **Claude Code** — planning and design only. All Markdown deliverables under `aidlc-docs/**`. Does NOT write Go application code (narrow exception: agent-config files like `AGENTS.md`, `.cursor/rules/`, `.codex/config.toml`, `.agent-memory/`).
- **OpenAI Codex CLI** (model: `gpt-5.5`, reasoning effort: `xhigh`) — autonomous batch implementation. Reads `aidlc-docs/construction/<unit>/code/code-generation-plan.md` and `AGENTS.md`, then implements one unit at a time.
- **Cursor Composer 2.5** — interactive editing, microadjustments, debugging, refactoring. Reads `.cursor/rules/*.mdc` (new MDC format with frontmatter `description`/`globs`/`alwaysApply`).

Hand-off boundary: Claude produces the `code-generation-plan.md` (Code Generation — Planning); Codex/Cursor execute the Generation portion.

Implementation-time issues that require requirements/design changes are NOT to be silently resolved by implementation agents — they must leave a `TODO(agent):` comment in code and append a note to `aidlc-docs/audit.md`, and the change is then arbitrated in a Claude session.

Related: [[feedback-conventional-commits]].
