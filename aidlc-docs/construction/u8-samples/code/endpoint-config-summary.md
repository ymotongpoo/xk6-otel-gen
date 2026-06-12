# U8 samples/docs — Code Summary: Per-Signal Endpoint Support

**Change Request**: Per-Signal Endpoint Support(2026-06-12)
**Plan**: `aidlc-docs/construction/plans/endpoint-config-code-generation-plan.md` Steps 13-14

## Modified Files

- **`README.md`** (modified)
  - `## Configuration` に `### Endpoint resolution` 小節を追加: ベースエンドポイントのパス補完
    (HTTP の `v1/{signal}`)と per-signal オーバーライド、サーフェス対応表(JS / `--out` / env)、
    per-signal キーを含む JS 例、**破壊的変更**の告知ブロック(NFR-2)
  - Configuration 優先順位表の Environment 行に per-signal env var の例を含意(既存記述を保持)

- **`k6otelgen/doc.go`** (modified)
  - パッケージ godoc に `Endpoint resolution:` 節を追加(base + per-signal の挙動)

- **`k6output/doc.go`** (modified)
  - `--out` args 表に `metricsEndpoint` 行を追加し、表の桁揃えを整形

- **`examples/saas-endpoints.md`** (modified)
  - Grafana Cloud トラブルシュート節を更新: パス補完は xk6-otel-gen が OTLP 仕様に従って行う旨を明確化、
    per-signal オーバーライド(`tracesEndpoint`/`metricsEndpoint`/`logsEndpoint` / 環境変数)を案内

## Notes

- `CHANGELOG.md` はリリース時に git-cliff がコミットから生成する(cliff.toml に breaking-change
  専用グルーピングはない)。破壊的変更のユーザー向け告知は README の Endpoint resolution 節に集約。
- `k6otelgen/doc.go` の `setup()` ライフサイクル例(別件の serialization 問題)は本 CR のスコープ外のため未変更。

## Requirement Traceability
- FR-6(ドキュメント・サンプル更新)/ NFR-2(破壊的変更告知)

## Verification
- `go build ./...` / `go test ./...` 成功
- `markdownlint-cli2 README.md examples/saas-endpoints.md` → 0 errors
