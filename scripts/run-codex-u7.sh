#!/usr/bin/env bash
# SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
# SPDX-License-Identifier: Apache-2.0
# run-codex-u7.sh — Long-running batch execution of U7 implementation by Codex CLI.
#
# Hand-off contract (also documented in scripts/README.md):
#   - This script feeds the U7 code-generation-plan.md to `codex exec` in
#     non-interactive mode.
#   - Codex operates under workspace-write sandbox + network=enabled
#     (defined in .codex/config.toml) and auto-approves all in-workspace
#     operations (approval_policy=never, set in config and re-asserted
#     via -c). Network is enabled so Codex can autonomously fetch Go
#     modules (`go get`, `go mod tidy`). Aborts are still possible via
#     Ctrl+C or `kill <pid>`.
#   - Logs are tee'd to logs/codex-u7-<timestamp>.log AND streamed to
#     stdout so you can `tail -f` the file from another shell.
#   - The script aborts before launching Codex if the working tree is
#     dirty, so any code Codex writes is attributable to a clean baseline
#     commit. Resume after rollback by re-running.
#
# Usage:
#   foreground (recommended first time, watch live):
#     ./scripts/run-codex-u7.sh
#
#   background (unattended; recommended once you trust the setup):
#     nohup ./scripts/run-codex-u7.sh > /dev/null 2>&1 &
#     # then `tail -f logs/codex-u7-*.log` to watch
#
#   dry-run (validate prerequisites only, don't launch Codex):
#     ./scripts/run-codex-u7.sh --dry-run
#
# Stop conditions:
#   - Codex completes Phase 0..5 (success exit 0)
#   - Codex hits an "Implementation-time Question" and exits (per
#     code-generation-plan.md "Boundaries" section)
#   - Hard timeout (default 4 hours, override with CODEX_TIMEOUT)
#   - User interrupt (Ctrl+C or SIGTERM)

set -euo pipefail

# ----------------------------------------------------------------------------
# Configuration
# ----------------------------------------------------------------------------
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly PLAN_PATH="aidlc-docs/construction/u7-testutil/code/code-generation-plan.md"
readonly AGENTS_PATH="AGENTS.md"
readonly LOG_DIR="${REPO_ROOT}/logs"
readonly TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
readonly LOG_FILE="${LOG_DIR}/codex-u7-${TIMESTAMP}.log"
readonly CODEX_TIMEOUT="${CODEX_TIMEOUT:-14400}"  # seconds, default 4h

DRY_RUN=false

# ----------------------------------------------------------------------------
# Helpers
# ----------------------------------------------------------------------------
log()  { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*"; }
die()  { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1 ($2)"
}

# ----------------------------------------------------------------------------
# Argument parsing
# ----------------------------------------------------------------------------
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    -h|--help)
      sed -n '2,40p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) die "unknown argument: $arg (use --help)" ;;
  esac
done

# ----------------------------------------------------------------------------
# Preflight checks
# ----------------------------------------------------------------------------
log "preflight checks…"

cd "${REPO_ROOT}"

require_cmd git   "install git"
require_cmd codex "install OpenAI Codex CLI: see https://github.com/openai/codex"
require_cmd go    "install Go toolchain (Codex will need it during the run)"

[[ -f "${PLAN_PATH}" ]]   || die "plan file missing: ${PLAN_PATH}"
[[ -f "${AGENTS_PATH}" ]] || die "agents file missing: ${AGENTS_PATH}"
[[ -f ".codex/config.toml" ]] || die ".codex/config.toml missing — run Claude Code session first to set it up"

# Git baseline
if [[ -n "$(git status --porcelain)" ]]; then
  die "working tree is dirty — commit or stash before running Codex"
fi
BASELINE_COMMIT="$(git rev-parse HEAD)"
BASELINE_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
log "baseline branch=${BASELINE_BRANCH} commit=${BASELINE_COMMIT:0:10}"

# Quick Codex version note (best-effort)
CODEX_VERSION="$(codex --version 2>/dev/null || echo 'unknown')"
log "codex version: ${CODEX_VERSION}"

if "${DRY_RUN}"; then
  log "--dry-run: preflight OK, exiting without launching Codex"
  exit 0
fi

mkdir -p "${LOG_DIR}"

# ----------------------------------------------------------------------------
# Compose the prompt
# ----------------------------------------------------------------------------
# Keep this lean: AGENTS.md and the plan file carry the full context. The
# prompt here only points Codex at them and reiterates the global guardrails.
read -r -d '' PROMPT <<'PROMPT_EOF' || true
You are the implementation agent for the xk6-otel-gen project. Read
AGENTS.md first to understand your role boundaries (§2) and Definition
of Done (§7), then execute the U7 Code Generation plan in full:

  aidlc-docs/construction/u7-testutil/code/code-generation-plan.md

Authoritative source documents (read as needed):
  - aidlc-docs/construction/u7-testutil/functional-design/*.md
  - aidlc-docs/construction/u7-testutil/nfr-requirements/*.md
  - aidlc-docs/construction/u7-testutil/nfr-design/*.md
  - aidlc-docs/inception/application-design/component-methods.md
  - .aidlc-rule-details/extensions/testing/property-based/property-based-testing.md
  - .agent-memory/MEMORY.md  (shared cross-agent memory)
  - CLAUDE.md (project workflow rules, especially the Conventional Commits policy)

Execution rules:
  1. Work through all 27 numbered steps across Phase 0 -> Phase 5
     sequentially. Mark each `[ ]` checkbox `[x]` in the plan
     immediately upon completion of that step.
  2. After completing each Phase, create a Conventional Commits-style
     commit grouping the files produced by that Phase. Use canonical
     commit types per CLAUDE.md "MANDATORY: Conventional Commits at
     Stage Boundaries":
        Phase 0  -> feat(u7-testutil): scaffold topology package skeleton
        Phase 1  -> feat(u7-testutil): implement generators (split into
                    smaller commits if it helps reviewability)
        Phase 2  -> test(u7-testutil): add property-based and example tests
        Phase 3  -> test(u7-testutil): add BenchmarkValidSchemaDraw
        Phase 4  -> docs(u7-testutil): add Example functions and GoDoc
        Phase 5  -> chore(u7-testutil): finalize code-generation-summary
                    and checkbox state
     Include a Co-Authored-By trailer identifying yourself (Codex,
     gpt-5.5 xhigh).
  3. Do NOT modify aidlc-docs/**, AGENTS.md, CLAUDE.md, .aidlc-rule-details/**,
     .codex/**, .cursor/**, or .agent-memory/** outside the explicit
     instructions in the plan (audit.md append, code-generation-summary.md
     create, plan checkbox updates).
  4. Do NOT add Go dependencies beyond `pgregory.net/rapid` and
     `gopkg.in/yaml.v3`.
  5. Do NOT introduce package-level mutable state.
  6. If you encounter genuine ambiguity that the source documents do
     not resolve, STOP. Append an "Implementation-time Question"
     entry to aidlc-docs/audit.md with the question and exit. Do not
     guess.
  7. Final acceptance per AGENTS.md §7 and code-generation-plan.md
     Phase 5: `go build ./...`, `go test -race -count=1 ./...`,
     coverage >= 80% in testutil/generators, no remaining
     `TODO(agent):` markers.

When you finish (success or ambiguity stop), print a one-paragraph
summary stating the final commit hash, total commits made, coverage
percentage, and benchmark result (ns/op), then exit cleanly.
PROMPT_EOF

# ----------------------------------------------------------------------------
# Launch Codex
# ----------------------------------------------------------------------------
log "launching codex exec (auto-approval, workspace-write sandbox, network=enabled)"
log "log file: ${LOG_FILE}"
log "tip: tail -f ${LOG_FILE}  (from another shell)"
log "hard timeout: ${CODEX_TIMEOUT}s"

# Print baseline info into the log header for postmortem ease.
{
  echo "=================================================================="
  echo "codex-u7 batch run"
  echo "  timestamp:       ${TIMESTAMP}"
  echo "  repo:            ${REPO_ROOT}"
  echo "  baseline branch: ${BASELINE_BRANCH}"
  echo "  baseline commit: ${BASELINE_COMMIT}"
  echo "  codex version:   ${CODEX_VERSION}"
  echo "  timeout:         ${CODEX_TIMEOUT}s"
  echo "=================================================================="
  echo
  echo "--- PROMPT ---"
  echo "${PROMPT}"
  echo "--- END PROMPT ---"
  echo
  echo "--- CODEX OUTPUT FOLLOWS ---"
  echo
} | tee -a "${LOG_FILE}"

# Run codex exec. Flag rationale (codex-cli >= 0.137):
#   --sandbox workspace-write       : confine writes to the workspace
#   -c approval_policy="never"      : fully unattended (re-asserts the config value)
#   --skip-git-repo-check           : we already validated the git state above
# The prompt is passed via stdin (`-` argument) to avoid argv length and
# quoting concerns.
#
# If your codex version uses different flag names, run `codex exec --help`
# and adjust below. The contract intended is "non-interactive,
# workspace-write sandbox, no approval prompts, skip the redundant
# git-clean-tree pre-check (we already did it ourselves)".
set +e
timeout --kill-after=30s "${CODEX_TIMEOUT}" \
  codex exec \
    --sandbox workspace-write \
    -c approval_policy="never" \
    --skip-git-repo-check \
    - < <(printf '%s' "${PROMPT}") \
  2>&1 | tee -a "${LOG_FILE}"
CODEX_EXIT=${PIPESTATUS[0]}
set -e

# ----------------------------------------------------------------------------
# Postflight summary
# ----------------------------------------------------------------------------
log "codex exited with status ${CODEX_EXIT}"

NEW_HEAD="$(git rev-parse HEAD)"
if [[ "${NEW_HEAD}" != "${BASELINE_COMMIT}" ]]; then
  COMMITS_MADE=$(git rev-list "${BASELINE_COMMIT}..HEAD" --count)
  log "${COMMITS_MADE} new commit(s) on ${BASELINE_BRANCH}:"
  git log --oneline "${BASELINE_COMMIT}..HEAD" | tee -a "${LOG_FILE}"
else
  log "no new commits (Codex may have stopped on an ambiguity — check audit.md)"
fi

if [[ -n "$(git status --porcelain)" ]]; then
  log "WARNING: working tree is dirty after Codex run — review with 'git status' and 'git diff'"
fi

case ${CODEX_EXIT} in
  0)   log "SUCCESS — return to Claude Code for U7 final review and Continue to Next Stage" ;;
  124) log "TIMEOUT (${CODEX_TIMEOUT}s) — inspect ${LOG_FILE} and decide whether to resume" ;;
  *)   log "Codex exited non-zero (status ${CODEX_EXIT}) — inspect ${LOG_FILE}" ;;
esac

exit "${CODEX_EXIT}"
