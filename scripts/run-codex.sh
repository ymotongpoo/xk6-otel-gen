#!/usr/bin/env bash
# run-codex.sh — Generic long-running batch execution of a unit's
# code-generation-plan.md by Codex CLI.
#
# Successor to scripts/run-codex-u7.sh (which remains for U7 historical
# compatibility). This generic version accepts a unit ID and optionally a
# phase range, so the same script powers every unit's Codex batch.
#
# Hand-off contract (also documented in scripts/README.md):
#   - Feeds the unit's code-generation-plan.md to `codex exec` non-interactively.
#   - Codex operates under workspace-write sandbox + network=enabled
#     (defined in .codex/config.toml) and auto-approves all in-workspace
#     operations (approval_policy=never, re-asserted via -c).
#   - Logs are tee'd to logs/codex-<unit>-<phases>-<timestamp>.log.
#   - Aborts before launch if working tree is dirty.
#
# Usage:
#   ./scripts/run-codex.sh <unit-id> [--phases <range>]
#   e.g.
#     ./scripts/run-codex.sh u1                  # all phases
#     ./scripts/run-codex.sh u1 --phases 0-12    # subset
#     ./scripts/run-codex.sh u1 --phases 3       # single phase
#     ./scripts/run-codex.sh u1 --dry-run        # preflight only
#
# Unit-id maps to the plan path:
#   aidlc-docs/construction/<unit-id>-*/code/code-generation-plan.md
# (the wildcard matches things like "u1-topology" or "u7-testutil").

set -euo pipefail

# ----------------------------------------------------------------------------
# Configuration
# ----------------------------------------------------------------------------
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly LOG_DIR="${REPO_ROOT}/logs"
readonly TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
readonly CODEX_TIMEOUT="${CODEX_TIMEOUT:-14400}"  # seconds, default 4h

UNIT_ID=""
PHASES="all"
DRY_RUN=false

# ----------------------------------------------------------------------------
# Helpers
# ----------------------------------------------------------------------------
log()  { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*"; }
die()  { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1 ($2)"
}

usage() {
  sed -n '2,30p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

# ----------------------------------------------------------------------------
# Argument parsing
# ----------------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=true; shift ;;
    --phases)  PHASES="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    -*)        die "unknown option: $1 (use --help)" ;;
    *)
      if [[ -z "${UNIT_ID}" ]]; then
        UNIT_ID="$1"
        shift
      else
        die "unexpected positional argument: $1 (use --help)"
      fi
      ;;
  esac
done

[[ -n "${UNIT_ID}" ]] || die "unit-id is required (e.g., u1, u4). Use --help."

# ----------------------------------------------------------------------------
# Resolve plan path
# ----------------------------------------------------------------------------
cd "${REPO_ROOT}"

PLAN_GLOB="aidlc-docs/construction/${UNIT_ID}-*/code/code-generation-plan.md"
shopt -s nullglob
PLAN_MATCHES=( ${PLAN_GLOB} )
shopt -u nullglob

if [[ ${#PLAN_MATCHES[@]} -eq 0 ]]; then
  die "no plan matching ${PLAN_GLOB}"
elif [[ ${#PLAN_MATCHES[@]} -gt 1 ]]; then
  die "multiple plans matched ${PLAN_GLOB}: ${PLAN_MATCHES[*]}"
fi
readonly PLAN_PATH="${PLAN_MATCHES[0]}"

readonly LOG_FILE="${LOG_DIR}/codex-${UNIT_ID}-${PHASES//\//_}-${TIMESTAMP}.log"

# ----------------------------------------------------------------------------
# Preflight checks
# ----------------------------------------------------------------------------
log "preflight checks…"
log "unit: ${UNIT_ID}"
log "plan: ${PLAN_PATH}"
log "phases: ${PHASES}"

require_cmd git   "install git"
require_cmd codex "install OpenAI Codex CLI: https://github.com/openai/codex"
require_cmd go    "install Go toolchain (Codex will need it during the run)"

[[ -f "${PLAN_PATH}" ]] || die "plan file missing: ${PLAN_PATH}"
[[ -f "AGENTS.md" ]]    || die "AGENTS.md missing"
[[ -f ".codex/config.toml" ]] || die ".codex/config.toml missing"

if [[ -n "$(git status --porcelain)" ]]; then
  die "working tree is dirty — commit or stash before running Codex"
fi
BASELINE_COMMIT="$(git rev-parse HEAD)"
BASELINE_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
log "baseline branch=${BASELINE_BRANCH} commit=${BASELINE_COMMIT:0:10}"
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
PHASE_INSTRUCTION=""
if [[ "${PHASES}" != "all" ]]; then
  PHASE_INSTRUCTION="

Phase scope for this run: ONLY phases [${PHASES}]. Skip checkboxes outside
this range. The other phases will be handled by separate runs (possibly
by a different agent). Do not touch their files."
fi

read -r -d '' PROMPT <<PROMPT_EOF || true
You are the implementation agent for the xk6-otel-gen project. Read
AGENTS.md first to understand your role boundaries (§2), Definition of
Done (§7), and the agent-selection guideline. Then execute the code
generation plan at:

  ${PLAN_PATH}

Authoritative source documents (read as needed):
  - The FD/NFR-R/NFR-D documents under aidlc-docs/construction/${UNIT_ID}-*/
  - aidlc-docs/inception/application-design/component-methods.md
  - .aidlc-rule-details/extensions/testing/property-based/property-based-testing.md
  - .agent-memory/MEMORY.md  (shared cross-agent memory)
  - CLAUDE.md (Conventional Commits policy at stage boundaries)
${PHASE_INSTRUCTION}

Execution rules:
  1. Work through the plan checkboxes sequentially. Mark each [ ]
     checkbox [x] in the plan immediately upon completion.
  2. After completing each Phase, create a Conventional Commits-style
     commit. Use canonical commit types per CLAUDE.md "MANDATORY:
     Conventional Commits at Stage Boundaries". Include a
     Co-Authored-By trailer identifying yourself: Codex (gpt-5.5 xhigh).
  3. Do NOT modify aidlc-docs/**, AGENTS.md, CLAUDE.md, .aidlc-rule-details/**,
     .codex/**, .cursor/**, or .agent-memory/** outside the explicit
     instructions in the plan.
  4. Do NOT add dependencies beyond what the plan specifies (typically
     yaml.v3, jsonschema/v5 for U1; pgregory.net/rapid for tests).
  5. Do NOT introduce package-level mutable state.
  6. If you encounter genuine ambiguity, STOP and append an
     "Implementation-time Question" entry to aidlc-docs/audit.md and
     exit.
  7. Final acceptance per AGENTS.md §7 and the plan's DoD section:
     go build, go test -race, coverage targets, no remaining
     TODO(agent) markers.

When you finish (success or ambiguity stop), print a one-paragraph
summary stating the final commit hash, total commits made, coverage
percentage, and any benchmark results, then exit cleanly.
PROMPT_EOF

# ----------------------------------------------------------------------------
# Launch Codex
# ----------------------------------------------------------------------------
log "launching codex exec (workspace-write sandbox, network=enabled, approval_policy=never)"
log "log file: ${LOG_FILE}"
log "hard timeout: ${CODEX_TIMEOUT}s"

{
  echo "=================================================================="
  echo "codex generic batch run"
  echo "  timestamp:       ${TIMESTAMP}"
  echo "  unit:            ${UNIT_ID}"
  echo "  phases:          ${PHASES}"
  echo "  plan:            ${PLAN_PATH}"
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
# Postflight
# ----------------------------------------------------------------------------
log "codex exited with status ${CODEX_EXIT}"

NEW_HEAD="$(git rev-parse HEAD)"
if [[ "${NEW_HEAD}" != "${BASELINE_COMMIT}" ]]; then
  COMMITS_MADE=$(git rev-list "${BASELINE_COMMIT}..HEAD" --count)
  log "${COMMITS_MADE} new commit(s) on ${BASELINE_BRANCH}:"
  git log --oneline "${BASELINE_COMMIT}..HEAD" | tee -a "${LOG_FILE}"
else
  log "no new commits (Codex may have stopped on ambiguity — check audit.md)"
fi

if [[ -n "$(git status --porcelain)" ]]; then
  log "WARNING: working tree is dirty after Codex run — review with 'git status' and 'git diff'"
fi

case ${CODEX_EXIT} in
  0)   log "SUCCESS — return to Claude Code for ${UNIT_ID} stage review" ;;
  124) log "TIMEOUT (${CODEX_TIMEOUT}s) — inspect ${LOG_FILE}" ;;
  *)   log "Codex exited non-zero (status ${CODEX_EXIT}) — inspect ${LOG_FILE}" ;;
esac

exit "${CODEX_EXIT}"
