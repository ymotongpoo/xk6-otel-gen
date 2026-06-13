---
name: project-branching-ci
description: dev is the default branch; CI runs only on main; dev->main merge is manual; git hooks reproduce CI locally
metadata:
  type: project
---

ブランチ運用と CI のトリガ方針(2026-06-13 設定):

- **デフォルトブランチは `dev`**。日常の開発は `dev` で行い、`dev` への push は
  どの GitHub Actions ワークフローもトリガしない(= CI 時間を消費しない)。
- **CI は `main` でのみ走る**。`ci.yml` / `codeql.yml` / `security-scanners.yml`
  はいずれも `push`/`pull_request` のブランチを `main` に限定(加えて codeql/security
  はスケジュール実行)。
- **`dev` -> `main` のマージは手動**。`git switch main && git merge dev &&
  git push origin main` で main に反映したときに CI が1回だけ走る、という運用。
  むやみに `main` へ直接 push しない([[feedback-conventional-commits]] と同様、
  main への push は最終ゲート扱い)。
- **ローカルの CI 相当チェックは git hooks で担保**(`.githooks/`、`core.hooksPath`
  で有効化)。pre-commit = go build + `golangci-lint run` + markdownlint-cli2 +
  issue #246 の空行チェック。pre-push = `go test -race -count=1 ./...`。
  golangci-lint は CI と同じ **v2.12.2** をローカルに入れること(govet は既定 linter
  に含まれるので別途 `go vet` はしない)。

**Why:** main への push 毎に CI(build/test/lint + CodeQL + security scanners)が
走ってしまい GitHub Actions の実行時間を無駄に消費していたため。

**How to apply:** 作業は `dev` で。コミット/プッシュ前にフックが CI 相当チェックを
実行するので、main へマージする頃には CI が通る状態になっている。新しい clone では
`git config core.hooksPath .githooks` を一度実行する(`.githooks/README.md` 参照)。
