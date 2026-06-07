---
name: feedback-conventional-commits
description: User requires Conventional Commits format and per-stage commits integrated into the AI-DLC workflow for xk6-otel-gen
metadata:
  type: feedback
---

For the **xk6-otel-gen** project, the user requires Conventional Commits (1.0.0) format and wants commit checkpoints built into the AI-DLC workflow, not deferred to the end.

**Why:** The user explicitly asked to "make a conventional commit" for the bootstrap and to "integrate conventional commits into the workflow at appropriate AI-DLC checkpoints." The repo already has a `.github/workflows/pull-request-lint.yml` workflow that benefits from canonical types.

**How to apply:**
- After every approved AI-DLC stage that produces tracked artifacts, propose a Conventional Commits-formatted commit BEFORE proceeding to the next stage. Don't batch up multiple stages into one commit.
- Use canonical types only: `feat`, `fix`, `docs`, `chore`, `refactor`, `test`, `build`, `ci`, `perf`, `style`, `revert`. The previous repo style `add(skill): ...` is non-canonical and should NOT be reused.
- Stage→type mapping is documented in `CLAUDE.md` under "MANDATORY: Conventional Commits at Stage Boundaries" — refer to it instead of guessing.
- Include `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` trailer for Claude-authored commits. For Codex/Cursor commits, use the appropriate Co-Authored-By trailer for that agent (e.g., `Co-Authored-By: OpenAI Codex (gpt-5.5) <noreply@openai.com>`).
- Propose the commit message first; commit only after user confirms (do not auto-commit). The user typically stages files themselves and asks the agent only to commit.
- In Construction, prefer one commit per unit per stage to preserve traceability with the matching design doc.

Related: [[user-tooling-preferences]].
