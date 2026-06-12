# Build and Test — Per-Signal Endpoint Support

**Change Request**: Per-Signal Endpoint Support(2026-06-12)
**Requirements**: `aidlc-docs/inception/requirements/endpoint-config-requirements.md`

## Build

- `go build ./...` — 成功
- `gofmt` — 全変更ファイルで差分なし
- k6 バイナリビルド: `GOFLAGS=-buildvcs=false xk6 build v1.8.0 --with github.com/ymotongpoo/xk6-otel-gen=.`
  — 成功(linux/amd64、ローカル拡張をバンドル)

## Unit & Property-Based Tests

- `go test ./...` — 全パッケージ成功
- PBT(blocking、Property-Based Testing 拡張 Full):
  - TP-U4-5 パス補完の構造保存(`/v1/{signal}` 終端 / scheme・host・query・fragment 保存 / 接頭辞 / 単回追記)— 成功
  - TP-U4-6 解決の優先順位 + per-signal 独立性 — 成功
  - TP-U4-7 ConfigFromEnv の per-signal 適用(非 ENDPOINT キー回帰)— 成功
  - 既存 TP-U4-1/2(MergeWith)は per-signal フィールド込みで成功
- 例示テスト `TestResolveEndpoints_Examples`(Grafana Cloud `/otlp`、末尾 `/`、パスなし、`/v1/traces` 重複、
  query+fragment、host:port、gRPC、per-signal override、デフォルト)— 成功

## End-to-End Verification(原障害の再現と解消)

ローカル OTLP/HTTP モックレシーバ(リクエストパスを記録、200 を返す)に対し、
**ベースエンドポイント** `http://127.0.0.1:14318/otlp`(`/v1/...` パスなし、原障害と同形)で
3 イテレーション実行:

- 起動ログ `exporter configured` の解決後エンドポイント:
  - `traces=http://127.0.0.1:14318/otlp/v1/traces`
  - `metrics=http://127.0.0.1:14318/otlp/v1/metrics`
  - `logs=http://127.0.0.1:14318/otlp/v1/logs`
- モックが受信したパス: `/otlp/v1/traces`、`/otlp/v1/metrics`(JS module + k6 native output)、`/otlp/v1/logs`
- `exporter failures observed` 警告なし、`404` なし、`level=error` なし、3 iterations complete

修正前は全シグナルが `/otlp` へ POST して 404(原障害)だったのに対し、修正後は OTLP 仕様どおり
`/otlp/v1/{signal}` へ到達することを実機で確認。

## Lint

- `markdownlint-cli2 README.md examples/saas-endpoints.md` — 0 errors

## Result

全 Quality Gate を満たした。原障害(Grafana Cloud ベースエンドポイントへの全シグナル 404)は解消。
