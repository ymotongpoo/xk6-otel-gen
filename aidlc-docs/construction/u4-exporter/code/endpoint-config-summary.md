# U4 exporter — Code Summary: Per-Signal Endpoint Support

**Change Request**: Per-Signal Endpoint Support(2026-06-12)
**Plan**: `aidlc-docs/construction/plans/endpoint-config-code-generation-plan.md` Steps 1-7

## Modified Files

- **`exporter/config.go`** (modified)
  - `Config` に per-signal フィールド `TracesEndpoint` / `MetricsEndpoint` / `LogsEndpoint` を追加(ドキュメントコメント付き)
  - `Validate`: 3 フィールドが非空のとき `validEndpoint` で検証(違反は `ConfigError{Field:"<Name>Endpoint"}`)
  - `MergeWith`: 3 フィールドに「override 非空なら採用」分岐を追加
  - `ConfigFromEnv`: `OTEL_EXPORTER_OTLP_ENDPOINT` → `Endpoint`、`OTEL_EXPORTER_OTLP_{TRACES|METRICS|LOGS}_ENDPOINT`
    → 各 per-signal フィールド(専用 `os.LookupEnv`)。ENDPOINT 以外のキーは `lookupSignalEnv` のまま

- **`exporter/endpoints.go`** (新規)
  - `func (c Config) ResolveEndpoints() (traces, metrics, logs string)` — シグナルごとの実効送信先を解決する純関数
  - `resolveSignalEndpoint(perSignal, base, protocol, signalPath)` — per-signal > base(HTTP+URL なら補完)> base
  - `appendSignalPath(base, signalPath)` — OTLP 仕様のパス補完(末尾 `/` 正規化、query/fragment 保持、重複ガードなし)

- **`exporter/exporters.go`** (modified)
  - `buildTraceExporter` / `buildMetricExporter` / `buildLogExporter` が `cfg.Endpoint` 直参照を廃し、
    `ResolveEndpoints()` の対応シグナル値を `WithEndpointURL` / `WithEndpoint` に渡す

## Tests

- **`exporter/endpoints_test.go`** (新規): `ResolveEndpoints` の例示テスト(Grafana Cloud `/otlp`、末尾 `/`、
  パスなし、`/v1/traces` 重複、query+fragment、host:port、gRPC、per-signal override、デフォルト)、
  per-signal `Validate` ケース
- **`exporter/endpoints_property_test.go`** (新規): TP-U4-5(パス構造保存)、TP-U4-6(優先順位 + 独立性)、
  TP-U4-7(ConfigFromEnv per-signal 適用)
- **`exporter/config_property_test.go`** (modified): MergeWith override-wins プロパティに per-signal 3 フィールドを追加
- **`exporter/config_test.go`** (modified): `TestConfigFromEnv_SignalSpecificPriority` を
  `TestConfigFromEnv_SignalSpecificEndpoints` に置換(旧バグ挙動の検証 → OTLP 仕様準拠の検証)
- **`testutil/generators/exporter_config.go`** (modified): `ValidConfig` が per-signal エンドポイントを
  確率的に生成(`WithoutPerSignalEndpoints()` で無効化可能)

## Requirement Traceability
- FR-1(ベースパス補完)/ FR-2(per-signal as-is + 優先順位)/ FR-4(ConfigFromEnv 修正)
- NFR-1(OTLP 仕様準拠)/ NFR-3(TP-U4-5/6/7、blocking)

## Verification
- `go build ./...` 成功
- `go test ./exporter/ ./testutil/generators/` 成功(PBT 含む)
