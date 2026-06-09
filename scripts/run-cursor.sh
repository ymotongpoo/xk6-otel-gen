#!/usr/bin/env bash
# run-cursor.sh — One-shot batch execution of a unit's
# code-generation-plan.md by Cursor Composer (Agent mode).
#
# Counterpart to scripts/run-codex.sh, but for Cursor. Cursor's batch
# capability is invoked via `cursor agent -p "<prompt>"` (one-shot,
# non-interactive). This script wraps that call with the same hand-off
# contract: plan reference, role boundaries, Conventional Commits per
# Phase, audit append on ambiguity.
#
# Cursor's strength is codebase-aware additive work — phases that
# match existing code patterns (e.g., adding more generators in the
# established U7 style). Use `scripts/run-codex.sh` for phases that
# require deep algorithmic reasoning.
#
# Usage:
#   ./scripts/run-cursor.sh <unit-id> [--phases <range>]
#   e.g.
#     ./scripts/run-cursor.sh u1 --phases 13      # only Phase 13
#     ./scripts/run-cursor.sh u1                  # all phases (rarely used)
#     ./scripts/run-cursor.sh u1 --dry-run        # preflight only
#
# Cursor CLI binary detection:
#   This script tries the following invocations in order until one
#   succeeds with --help (or --version):
#     1. cursor agent -p "..."
#     2. cursor-agent -p "..."
#   If your installation differs, set CURSOR_BIN="<binary>" and
#   CURSOR_SUBCMD="<sub-command>" env vars. Examples:
#     CURSOR_BIN=cursor-agent CURSOR_SUBCMD="" ./scripts/run-cursor.sh u1 --phases 13

set -euo pipefail

# ----------------------------------------------------------------------------
# Configuration
# ----------------------------------------------------------------------------
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly LOG_DIR="${REPO_ROOT}/logs"
readonly TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
readonly CURSOR_TIMEOUT="${CURSOR_TIMEOUT:-7200}"  # seconds, default 2h

UNIT_ID=""
PHASES="all"
DRY_RUN=false

# ----------------------------------------------------------------------------
# Helpers
# ----------------------------------------------------------------------------
log()  { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*"; }
die()  { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

usage() {
  sed -n '2,25p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

# Detect Cursor CLI binary.
# Result variables set: CURSOR_BIN, CURSOR_SUBCMD
detect_cursor_cli() {
  # Honor env-var override first.
  if [[ -n "${CURSOR_BIN:-}" ]]; then
    command -v "${CURSOR_BIN}" >/dev/null 2>&1 \
      || die "CURSOR_BIN=${CURSOR_BIN} not on PATH"
    : "${CURSOR_SUBCMD:=}"
    log "using user-specified ${CURSOR_BIN} ${CURSOR_SUBCMD}"
    return 0
  fi
  # Try `cursor agent`.
  if command -v cursor >/dev/null 2>&1; then
    if cursor agent --help >/dev/null 2>&1; then
      CURSOR_BIN="cursor"
      CURSOR_SUBCMD="agent"
      return 0
    fi
  fi
  # Try `cursor-agent`.
  if command -v cursor-agent >/dev/null 2>&1; then
    CURSOR_BIN="cursor-agent"
    CURSOR_SUBCMD=""
    return 0
  fi
  die "Cursor CLI not found. Install Cursor (with CLI enabled) or set CURSOR_BIN. See scripts/README.md."
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

readonly LOG_FILE="${LOG_DIR}/cursor-${UNIT_ID}-${PHASES//\//_}-${TIMESTAMP}.log"

# ----------------------------------------------------------------------------
# Preflight checks
# ----------------------------------------------------------------------------
log "preflight checks…"
log "unit: ${UNIT_ID}"
log "plan: ${PLAN_PATH}"
log "phases: ${PHASES}"

command -v git >/dev/null 2>&1 || die "git is required"
command -v go  >/dev/null 2>&1 || die "go toolchain is required"
detect_cursor_cli

[[ -f "${PLAN_PATH}" ]] || die "plan file missing: ${PLAN_PATH}"
[[ -f "AGENTS.md" ]]    || die "AGENTS.md missing"
[[ -d ".cursor/rules" ]] || die ".cursor/rules missing — Cursor will operate without project rules"

if [[ -n "$(git status --porcelain)" ]]; then
  die "working tree is dirty — commit or stash before running Cursor"
fi
BASELINE_COMMIT="$(git rev-parse HEAD)"
BASELINE_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
log "baseline branch=${BASELINE_BRANCH} commit=${BASELINE_COMMIT:0:10}"
log "cursor cli: ${CURSOR_BIN} ${CURSOR_SUBCMD}"

if "${DRY_RUN}"; then
  log "--dry-run: preflight OK, exiting without launching Cursor"
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
this range. Other phases are or will be handled by separate runs
(possibly by Codex). Do not touch files outside the assigned phase scope."
fi

read -r -d '' PROMPT <<PROMPT_EOF || true
You are Cursor Composer 2.5 acting as the implementation agent for the
xk6-otel-gen project. Your project context (.cursor/rules/*.mdc) is
already loaded. Read AGENTS.md (§2 role boundaries, §7 DoD,
"Codex と Cursor の使い分けガイドライン") before starting. Then execute
the code generation plan at:

  ${PLAN_PATH}

Authoritative source documents (read as needed):
  - The FD/NFR-R/NFR-D documents under aidlc-docs/construction/${UNIT_ID}-*/
  - aidlc-docs/inception/application-design/component-methods.md
  - Existing code in topology/ and testutil/generators/ — match its style
    precisely (codebase-aware advantage).
  - .aidlc-rule-details/extensions/testing/property-based/property-based-testing.md
  - .agent-memory/MEMORY.md  (shared cross-agent memory)
  - CLAUDE.md (Conventional Commits policy at stage boundaries)
${PHASE_INSTRUCTION}

Execution rules:
  1. Work through the plan checkboxes sequentially within your assigned
     phases. Mark each [ ] -> [x] immediately upon completion.
  2. After completing each Phase (or logical sub-group within), create a
     Conventional Commits-style commit. Use canonical commit types per
     CLAUDE.md. Include a Co-Authored-By trailer identifying yourself:
     "Cursor Composer (2.5)".
  3. Do NOT modify aidlc-docs/**, AGENTS.md, CLAUDE.md,
     .aidlc-rule-details/**, .codex/**, .cursor/**, or .agent-memory/**
     outside the explicit instructions in the plan.
  4. Do NOT add dependencies beyond what the plan specifies.
  5. Do NOT introduce package-level mutable state.
  6. STYLE PARITY: This is the key Cursor advantage. When extending
     existing code (e.g., adding new generators alongside ValidSchema,
     ValidService, etc.), match the existing file structure, naming
     conventions, GoDoc style, options pattern, and invariant
     documentation exactly. Mimic, don't invent.
  7. If you encounter genuine ambiguity, STOP and append an
     "Implementation-time Question" entry to aidlc-docs/audit.md and
     exit.
  8. Final acceptance per AGENTS.md §7 and the plan's DoD section:
     go build, go test -race, coverage targets, no remaining
     TODO(agent) markers.

When you finish (success or ambiguity stop), print a one-paragraph
summary stating the final commit hash, total commits made, coverage
percentage, and any other relevant metrics, then exit cleanly.
PROMPT_EOF

# ----------------------------------------------------------------------------
# Launch Cursor
# ----------------------------------------------------------------------------
log "launching ${CURSOR_BIN} ${CURSOR_SUBCMD} -p '<prompt>'"
log "log file: ${LOG_FILE}"
log "hard timeout: ${CURSOR_TIMEOUT}s"

{
  echo "=================================================================="
  echo "cursor generic batch run"
  echo "  timestamp:       ${TIMESTAMP}"
  echo "  unit:            ${UNIT_ID}"
  echo "  phases:          ${PHASES}"
  echo "  plan:            ${PLAN_PATH}"
  echo "  baseline branch: ${BASELINE_BRANCH}"
  echo "  baseline commit: ${BASELINE_COMMIT}"
  echo "  cursor cli:      ${CURSOR_BIN} ${CURSOR_SUBCMD}"
  echo "  timeout:         ${CURSOR_TIMEOUT}s"
  echo "=================================================================="
  echo
  echo "--- PROMPT ---"
  echo "${PROMPT}"
  echo "--- END PROMPT ---"
  echo
  echo "--- CURSOR OUTPUT FOLLOWS ---"
  echo
} | tee -a "${LOG_FILE}"

# Build the argv. Cursor agent expects:
#   cursor agent -p "<prompt>"      (or)
#   cursor-agent -p "<prompt>"
# Note: some Cursor CLI versions support stdin instead of -p; if -p fails
# in your environment, swap to: `${CURSOR_BIN} ${CURSOR_SUBCMD} <<< "${PROMPT}"`
CURSOR_ARGV=("${CURSOR_BIN}")
[[ -n "${CURSOR_SUBCMD}" ]] && CURSOR_ARGV+=("${CURSOR_SUBCMD}")
CURSOR_ARGV+=("-p" "${PROMPT}")

set +e
timeout --kill-after=30s "${CURSOR_TIMEOUT}" "${CURSOR_ARGV[@]}" 2>&1 | tee -a "${LOG_FILE}"
CURSOR_EXIT=${PIPESTATUS[0]}
set -e

# ----------------------------------------------------------------------------
# Postflight
# ----------------------------------------------------------------------------
log "cursor exited with status ${CURSOR_EXIT}"

NEW_HEAD="$(git rev-parse HEAD)"
if [[ "${NEW_HEAD}" != "${BASELINE_COMMIT}" ]]; then
  COMMITS_MADE=$(git rev-list "${BASELINE_COMMIT}..HEAD" --count)
  log "${COMMITS_MADE} new commit(s) on ${BASELINE_BRANCH}:"
  git log --oneline "${BASELINE_COMMIT}..HEAD" | tee -a "${LOG_FILE}"
else
  log "no new commits (Cursor may have stopped on ambiguity — check audit.md)"
fi

if [[ -n "$(git status --porcelain)" ]]; then
  log "WARNING: working tree is dirty after Cursor run — review with 'git status' and 'git diff'"
fi

case ${CURSOR_EXIT} in
  0)   log "SUCCESS — return to Claude Code for ${UNIT_ID} stage review" ;;
  124) log "TIMEOUT (${CURSOR_TIMEOUT}s) — inspect ${LOG_FILE}" ;;
  *)   log "Cursor exited non-zero (status ${CURSOR_EXIT}) — inspect ${LOG_FILE}" ;;
esac

exit "${CURSOR_EXIT}"
