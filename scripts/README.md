# `scripts/`

Implementation-agent runner scripts. These exist to launch the
implementation agents (Codex CLI / Cursor Composer) against a unit's
`code-generation-plan.md` in long-running, mostly-unattended mode.

## `run-codex-u7.sh`

Runs Codex CLI in non-interactive batch mode against the U7
(testutil/generators) implementation plan. Codex follows
`aidlc-docs/construction/u7-testutil/code/code-generation-plan.md`
checkbox-by-checkbox, commits per phase, and stops on ambiguity.

### Prerequisites

- **Codex CLI installed** and on `PATH`. See <https://github.com/openai/codex>.
- **`go` toolchain** installed (Codex will invoke it during implementation).
- **Clean working tree** on a branch where you're OK letting Codex commit
  (don't run this on `main` if you want to review before merging — create
  a feature branch first).
- `.codex/config.toml` present (this repo provides one;
  `model=gpt-5.5`, reasoning=`xhigh`, `sandbox_mode=workspace-write`,
  `network=enabled` — Codex needs network to autonomously fetch Go
  modules during the run).
- `AGENTS.md` and the plan file present (this repo provides both).

### Usage

```bash
# Foreground (recommended the first time so you can watch Codex work)
./scripts/run-codex-u7.sh

# Validate prerequisites only, don't actually launch Codex
./scripts/run-codex-u7.sh --dry-run

# Background (after you trust the setup). Watch the log from another shell:
nohup ./scripts/run-codex-u7.sh > /dev/null 2>&1 &
tail -f logs/codex-u7-*.log
```

### Environment overrides

| Variable | Default | Description |
|---|---|---|
| `CODEX_TIMEOUT` | `14400` (= 4 hours) | Hard wall-clock cap for the entire Codex run (in seconds). On timeout, Codex receives SIGTERM, then SIGKILL after 30s. |

### Safety guarantees

- **Sandbox**: writes confined to the workspace (`sandbox_mode=workspace-write` in `.codex/config.toml`).
- **Network**: enabled — required for Codex to autonomously fetch Go modules (`go get`, `go mod tidy`). The user has explicitly granted this permission for long-running unattended execution. Codex still cannot escape the workspace for writes.
- **Read-only paths**: `aidlc-docs/`, `.aidlc-rule-details/`, `CLAUDE.md`, `AGENTS.md` declared read-only in the config (best-effort enforcement; in addition, the plan itself instructs Codex not to modify these except for documented exceptions like appending to `audit.md`).
- **Pre-launch git check**: aborts if the working tree is dirty so all Codex commits attribute cleanly to a known baseline.
- **Logging**: full transcript saved to `logs/codex-u7-<UTC-timestamp>.log`.

### What the script does NOT do

- It does NOT push to a remote. All commits stay local; you decide when to push.
- It does NOT skip the approval prompt for destructive Git operations
  if Codex tries them — but the prompt instructs Codex to only do
  normal commits, no force-push / reset / etc.
- It does NOT modify `aidlc-docs/**` outside the planned audit.md
  appends — that's a hard rule in the prompt and AGENTS.md §2.

### Stop / resume

- **To stop**: Ctrl+C in the foreground terminal, or `kill <pid>` for backgrounded runs. Codex receives SIGTERM and exits within a few seconds.
- **To resume**: re-run the script. Codex will read the plan, see the checked boxes (`[x]`) for completed steps, and pick up where the previous run left off.
- **On ambiguity stop**: Codex appends an "Implementation-time Question" entry to `aidlc-docs/audit.md` and exits. Open a Claude Code session, answer the question (typically by updating the plan or a referenced design doc), commit the answer, then re-run this script.

### Postmortem after a run

```bash
# What did Codex do?
git log --oneline HEAD ^<baseline-commit-from-log-header>

# Run the same DoD checks Codex was meant to satisfy:
go build ./...
go test -race -count=1 ./...
go test -cover ./testutil/generators/...    # expect >= 80%
go test -bench=BenchmarkValidSchemaDraw -benchmem ./testutil/generators/...

# Inspect the summary Codex wrote:
cat aidlc-docs/construction/u7-testutil/code/code-generation-summary.md

# Inspect any ambiguity questions Codex raised:
tail -100 aidlc-docs/audit.md
```

If everything looks good, return to a Claude Code session and **Continue to Next Stage** (Code Generation final approval → U1 Functional Design).

### Codex CLI flag compatibility

The script invokes:

```bash
codex exec \
  --sandbox workspace-write \
  --ask-for-approval never \
  --skip-git-repo-check \
  -
```

If your Codex CLI version has different flag names (the CLI is under
active development), `codex --help` and `codex exec --help` will show
the current spelling. Adjust the script accordingly; the contract
intended is "run non-interactively, write within workspace only,
never prompt for approvals, ignore the git-clean-tree pre-check
(we already did it ourselves)".

### Related

- `AGENTS.md` — universal contract for implementation agents (Codex / Cursor).
- `aidlc-docs/construction/u7-testutil/code/code-generation-plan.md` — the SSOT plan that this script feeds to Codex.
- `.codex/config.toml` — Codex CLI configuration for this repo.
- `.cursor/rules/00-project-handoff.mdc` — equivalent contract for Cursor Composer interactive sessions.
