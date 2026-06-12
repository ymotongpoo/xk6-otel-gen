# Code Generation Plan — Per-Signal Endpoint Support

**Change Request**: Per-Signal Endpoint Support(2026-06-12)
**Requirements**: `aidlc-docs/inception/requirements/endpoint-config-requirements.md`
**Functional Design**: `aidlc-docs/construction/u4-exporter/functional-design/` §9 / §11
**SSOT**: この計画が Code Generation の単一の真実。手順は番号順に厳密実行する。

## Scope & Approach

横断的だが単一機能(エンドポイント解決)のため、4 ユニットを 1 計画で逐次実行する
(U4 → U5 → U6 → U8)。各ユニット完了時にチェックボックスを更新する。
**Brownfield**: 既存ファイルは in-place 修正(コピー作成禁止)。

実装は Claude が行う(本セッションの先行作業と同様。バグ修正 + 機能拡張で密結合のため一括)。
本リポジトリの慣習(go build / go test 緑、PBT blocking)を遵守する。

---

## U4 — `exporter/` (中核ロジック)

### Step 1: Config フィールド追加 (`exporter/config.go`)
- [ ] `Config` に `TracesEndpoint` / `MetricsEndpoint` / `LogsEndpoint string` を追加
- [ ] `MergeWith`: 3 フィールドに「override 非空なら採用」分岐を追加
- [ ] `Validate`: 3 フィールドが非空のとき `validEndpoint` で検証、違反は `ConfigError{Field:"<Name>Endpoint"}`
- [ ] `fillDefaults`: per-signal にはデフォルトを与えない(変更なしを確認)

### Step 2: エンドポイント解決ロジック (`exporter/endpoints.go` 新規)
- [ ] `func (c Config) ResolveEndpoints() (traces, metrics, logs string)` を実装
      (FD §9.1 のアルゴリズム; fillDefaults 済み前提で内部的に base を確定)
- [ ] `func appendSignalPath(base, signalPath string) string`(FD §9.2; url.Parse、
      末尾 `/` 正規化、query/fragment 保持、失敗時は base を返す)
- [ ] `func resolveSignal(perSignal, base string, protocol Protocol, signalPath string) string` ヘルパー

### Step 3: ConfigFromEnv 修正 (`exporter/config.go`)
- [ ] `OTEL_EXPORTER_OTLP_ENDPOINT` → `Endpoint`、`OTEL_EXPORTER_OTLP_{TRACES|METRICS|LOGS}_ENDPOINT`
      → 各 per-signal フィールド(専用ルックアップ)。ENDPOINT 以外のキーは `lookupSignalEnv` 維持

### Step 4: exporters.go の配線 (`exporter/exporters.go`)
- [ ] `New`(または各 build*Exporter 呼び出し元)で `ResolveEndpoints()` を 1 回呼び、
      解決後 URL を各 build 関数へ渡す
- [ ] 各 build*Exporter は渡された解決済みエンドポイントで `endpointIsURL` 判定 →
      `WithEndpointURL` / `WithEndpoint` を選択(既存ロジック流用、cfg.Endpoint 直参照を置換)

### Step 5: U4 ユニットテスト
- [ ] `exporter/endpoints_test.go`: appendSignalPath / ResolveEndpoints の例示ベーステスト
      (Grafana Cloud `https://otlp-gateway-…/otlp` → `/otlp/v1/{signal}`、末尾 `/`、
      query 付き、host:port、gRPC、per-signal override、混在)
- [ ] `exporter/config_test.go`: per-signal Validate / MergeWith / ConfigFromEnv ケース追加

### Step 6: U4 PBT (testutil/generators + exporter/*_property_test.go)
- [ ] `testutil/generators/exporter_config.go`: ValidConfig に per-signal フィールド生成を追加
      (一部確率で set、URL/host:port 混在)。`ConfigOption` で on/off 可能に
- [ ] `exporter/endpoints_property_test.go` 新規: TP-U4-5(構造保存 P1-P4)、TP-U4-6(優先順位 P1-P4)、
      TP-U4-7(ConfigFromEnv per-signal P1-P3)を rapid で実装

### Step 7: U4 コードサマリ
- [ ] `aidlc-docs/construction/u4-exporter/code/endpoint-config-summary.md` 作成(変更ファイル一覧)

---

## U5 — `k6otelgen/` (JS configure キー)

### Step 8: JS config キー追加 (`k6otelgen/config.go`)
- [ ] `optsToConfig` に `tracesEndpoint` / `metricsEndpoint` / `logsEndpoint`(string)を追加

### Step 9: 起動ログに解決後エンドポイント (`k6otelgen/instance.go`)
- [ ] `exporter configured` ログに `traces=` / `metrics=` / `logs=` を `ResolveEndpoints()` から追加(FD-Q4=A)
- [ ] `configuredEndpoint` ヘルパーは維持または ResolveEndpoints 利用へ調整

### Step 10: U5 テスト & サマリ
- [ ] `k6otelgen/config_test.go`: per-signal キーのマッピングテスト追加
- [ ] `aidlc-docs/construction/u5-k6otelgen/code/endpoint-config-summary.md`

---

## U6 — `k6output/` (--out metricsEndpoint キー)

### Step 11: --out args キー追加 (`k6output/params.go`)
- [ ] `applyKV` に `metricsEndpoint` を追加(`validEndpointArg` で検証、`markProvided`)
- [ ] `Params` に `MetricsEndpoint string` フィールド追加
- [ ] `exporterConfig`: `wasProvided("metricsEndpoint")` なら `cfg.MetricsEndpoint` へ
- [ ] `Description`(output.go)に解決後 metrics エンドポイント反映を検討(任意)

### Step 12: U6 テスト & サマリ
- [ ] `k6output/params_test.go`: metricsEndpoint パース/マッピングテスト追加
- [ ] `aidlc-docs/construction/u6-k6output/code/endpoint-config-summary.md`

---

## U8 — ドキュメント / サンプル

### Step 13: ドキュメント更新
- [ ] `README.md`: 設定キー一覧に per-signal キー追加、ベースエンドポイントの `v1/{signal}` 補完説明、
      **破壊的変更**(URL 形式ベースエンドポイントの挙動変更)を CHANGELOG/Breaking note に明記
- [ ] `k6otelgen/doc.go` / `k6output/doc.go`: 新キーを doc コメントに追記
- [ ] `examples/saas-endpoints.md`: Grafana Cloud 手順がベース `/otlp` 指定で動作することを反映
      (パス手動付与の記述があれば修正)、必要なら per-signal 例を追記
- [ ] CHANGELOG があれば追記(なければ README の Breaking Changes セクション)

### Step 14: U8 サマリ
- [ ] `aidlc-docs/construction/u8-samples/code/endpoint-config-summary.md`

---

## Story / Requirement Traceability

| Step | 要件 |
|---|---|
| 1-4 | FR-1, FR-2, FR-4 |
| 5-6 | NFR-1, NFR-3(TP-U4-5/6/7) |
| 8-9 | FR-3, NFR-4 |
| 11 | FR-5 |
| 13 | FR-6, NFR-2(破壊的変更告知) |

## Quality Gates(Build and Test ステージで実行)
- `go build ./...` / `go test ./...` 全件成功
- `go test ./exporter/ -run Property` / PBT プロパティ成立
- xk6 ビルド + Grafana Cloud 形式エンドポイント(ベース `/otlp`)での実機 404 解消確認

## Commit 方針
ユニットごとに 1 コミット(`feat(exporter)` / `feat(k6otelgen)` / `feat(k6output)` / `docs`)、
末尾に Co-Authored-By トレーラ。
