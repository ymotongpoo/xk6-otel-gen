# U5 k6otelgen — Code Summary: Per-Signal Endpoint Support

**Change Request**: Per-Signal Endpoint Support(2026-06-12)
**Plan**: `aidlc-docs/construction/plans/endpoint-config-code-generation-plan.md` Steps 8-10

## Modified Files

- **`k6otelgen/config.go`** (modified)
  - `optsToConfig` に JS フラットキー `tracesEndpoint` / `metricsEndpoint` / `logsEndpoint`(string)を追加。
    型不一致は既存 `typeMismatch` で `ConfigError{Kind:"type_mismatch"}` を返す

- **`k6otelgen/instance.go`** (modified)
  - `Configure` の `exporter configured` INFO ログに `ResolveEndpoints()` の解決後 3 値
    (`traces` / `metrics` / `logs`)を追加(NFR-4)

## Tests

- **`k6otelgen/config_test.go`** (modified): `TestOptsToConfig_PerSignalEndpoints` 追加、
  type-mismatch テーブルに 3 キー追加
- **`k6otelgen/instance_test.go`** (modified): `exporter configured` ログの解決後エンドポイント
  フィールド検証を追加

## Requirement Traceability
- FR-3(JS per-signal キー)/ NFR-4(解決後エンドポイントのログ)

## Verification
- `go build ./...` 成功 / `go test ./k6otelgen/` 成功
