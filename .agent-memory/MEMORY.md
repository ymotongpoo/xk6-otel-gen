# Shared Agent Memory — xk6-otel-gen

このディレクトリは **Claude Code / OpenAI Codex CLI / Cursor Composer** など、本リポジトリで作業する全エージェントが共有する永続メモリの **Single Source of Truth (SSOT)** です。

- Claude Code のローカルメモリ (`~/.claude/projects/.../memory/`) は本ディレクトリへの**ポインタのみ**を保持します。実体はこちらにあります。
- 新しい知見を記録するときは、まずこのディレクトリにファイルを追加/更新し、必要に応じて Claude 側のポインタも更新してください。
- 各エージェントはセッション開始時にこの `MEMORY.md` を読み、関連するエントリを参照してから作業を始めてください。

## Index

- [User tooling preferences](user-tooling-preferences.md) — Claude=planning, Codex CLI (gpt-5.5 xhigh)=autonomous batch impl, Cursor Composer 2.5=interactive editing
- [Conventional Commits at stage boundaries](feedback-conventional-commits.md) — propose canonical-type commits per AI-DLC stage; don't auto-commit; Co-Authored-By trailer required for Claude
- [Go 1.25 build flag](build-go125-buildvcs.md) — Go 1.25+ requires `GOFLAGS=-buildvcs=false` for `xk6 build` / `go build` in this repo
- [Branching & CI policy](project-branching-ci.md) — `dev` is default branch; CI runs only on `main`; `dev`->`main` merge is manual; `.githooks/` reproduce CI locally
