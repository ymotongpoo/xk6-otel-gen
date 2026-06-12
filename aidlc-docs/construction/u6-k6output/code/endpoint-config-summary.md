# U6 k6output — Code Summary: Per-Signal Endpoint Support

**Change Request**: Per-Signal Endpoint Support(2026-06-12)
**Plan**: `aidlc-docs/construction/plans/endpoint-config-code-generation-plan.md` Steps 11-12

## Modified Files

- **`k6output/params.go`** (modified)
  - `Params` に `MetricsEndpoint string` フィールド追加
  - `applyKV` に `--out otel-gen=metricsEndpoint=...` キー(`validEndpointArg` 検証、`markProvided`)
  - `exporterConfig`: `wasProvided("metricsEndpoint")` なら `cfg.MetricsEndpoint` へ反映

- **`k6output/output.go`** (modified)
  - `buildPipelineConfig`: `metricsEndpoint` を env マージへ追加
  - `Description`: `ResolveEndpoints()` の解決後 metrics エンドポイントを表示
    (params 由来の最小 Config から直接解決、`provided` マップ非依存)

## Tests

- **`k6output/params_test.go`** (modified): `TestParseOutArgs_MetricsEndpoint`(パース + exporterConfig マッピング)、
  `TestParseOutArgs_MetricsEndpoint_InvalidURL`(不正 URL 拒否)

## Requirement Traceability
- FR-5(k6 output の metricsEndpoint キー、metrics シグナル送信)

## Verification
- `go build ./...` 成功 / `go test ./k6output/` 成功
- 既存 `TestDescription_ContainsEndpoint`(`provided` 未設定の直接構築 Params)も解決ロジックで通過
