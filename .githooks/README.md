# Git hooks

Repository-managed git hooks that reproduce the CI checks locally so that
day-to-day work on the `dev` branch does not break `main`'s CI.

## Why

CI (`.github/workflows/ci.yml`, CodeQL, security scanners) runs **only on
`main`** — on pushes to `main` and on pull requests targeting `main`. The
default development branch is `dev`, and pushing to `dev` does not consume CI
minutes. The `dev` -> `main` merge is performed manually. These hooks are the
local safety net that keeps `main`'s CI green.

## Setup (once per clone)

```sh
git config core.hooksPath .githooks
```

Verify with `git config --get core.hooksPath` (should print `.githooks`).

## What runs

| Hook | Checks | Mirrors |
| --- | --- | --- |
| `pre-commit` | `go build ./...`, `golangci-lint run`, `markdownlint-cli2`, the issue #246 blank-line check | the Go Lint and Markdown Lint jobs (fast) |
| `pre-push` | `go test -race -count=1 ./...` | the Go Build & Test job (slow) |

The heavy race test suite is deferred to `pre-push` so commits stay fast.

## Requirements

- `golangci-lint` **v2.12.2** on `PATH` (matches the version pinned in CI).
- `node`/`npx` available — `markdownlint-cli2` is fetched on demand via `npx`.

## Bypass

Use sparingly, only when you know the check does not apply:

```sh
git commit --no-verify
git push --no-verify
```
