# Requirements — Per-Signal Endpoint Support (Endpoint Configuration Enhancement)

## Intent Analysis

- **User Request**: ベースエンドポイント指定(OTel 規約に従いシグナル種別ごとにパスを補完)と、
  シグナル別エンドポイント指定の 2 パターンに対応する。
- **Request Type**: Enhancement(既存機能の拡張 + 仕様準拠バグ修正)
- **Scope Estimate**: Multiple Components — `exporter/`(U4)、`k6otelgen/`(U5)、`k6output/`(U6)、
  `examples/` + README(U8)
- **Complexity Estimate**: Moderate
- **Requirements Depth**: Standard
- **Trigger**: Grafana Cloud OTLP ゲートウェイ(`https://otlp-gateway-…/otlp`)への送信が全シグナル
  404 になる障害。原因は URL 形式エンドポイントを `WithEndpointURL`(パス as-is)で渡しており、
  OTLP 仕様のベースエンドポイント補完(`v1/{signal}` 追記)が行われないこと。

## Normative Reference

- [OTLP Exporter Specification — Endpoint URLs for OTLP/HTTP](https://opentelemetry.io/docs/specs/otel/protocol/exporter/#endpoint-urls-for-otlphttp)
  本変更のパス補完・優先順位の規範。(Q2 = A: 仕様厳格準拠、重複ガードなし)

## Functional Requirements

### FR-1: ベースエンドポイントのパス補完(パターン 1)
- 共通 `endpoint` が指定され、プロトコルが **HTTP** の場合、シグナルごとの送信先 URL を
  OTLP 仕様に従って構築する:
  - ベース URL のパス末尾に `v1/traces` / `v1/metrics` / `v1/logs` を追記する。
  - ベースパスが `/` で終わる場合は二重スラッシュにしない(仕様の追記規則どおり)。
  - 既存パスは保持する(例: `https://host/otlp` → `https://host/otlp/v1/traces`)。
  - パスが既に `/v1/{signal}` を含んでいても重複ガードは行わない(仕様厳格準拠)。
- プロトコルが **gRPC** の場合、パス概念はないため `endpoint`(host:port または URL)を
  そのまま全シグナルに使用する(現行どおり)。
- `host:port` 形式(スキームなし)の HTTP エンドポイントは従来どおり SDK デフォルトパス
  (`/v1/{signal}`)が適用される。

### FR-2: シグナル別エンドポイント(パターン 2)
- `exporter.Config` に traces / metrics / logs 個別のエンドポイント設定を追加する。
- シグナル別エンドポイントは OTLP 仕様の per-signal セマンティクスに従い、指定 URL を
  **そのまま**(as-is、パス補完なし)使用する。(Q3 = A)
- 優先順位: シグナル別エンドポイント > ベース `endpoint`(補完あり) > デフォルト。
- gRPC でもシグナル別指定を許容する(シグナルごとに異なる host:port / URL)。
- バリデーション: シグナル別エンドポイント値は既存 `endpoint` と同一の形式検証
  (host:port または scheme://host[:port])を適用する。

### FR-3: JS `configure()` サーフェス(k6otelgen)
- フラットキー `tracesEndpoint` / `metricsEndpoint` / `logsEndpoint` を追加する。(Q1 = A)
- 値は string。型不一致は既存の `ConfigError`(type_mismatch)と同様に報告する。
- 既存の `endpoint` キーはベースエンドポイントとして FR-1 の補完対象になる。

### FR-4: 環境変数サーフェス(exporter.ConfigFromEnv)
- `OTEL_EXPORTER_OTLP_{TRACES|METRICS|LOGS}_ENDPOINT` は **該当シグナルのみ** に適用する
  (as-is)。現行の「最初に見つかった per-signal 値を共有 Config に適用する」挙動を修正する。(Q4 = A)
- `OTEL_EXPORTER_OTLP_ENDPOINT` はベースエンドポイントとして全シグナルに適用する(FR-1 の補完あり)。
- 優先順位は OTLP 仕様どおり: per-signal 環境変数 > base 環境変数。
- エンドポイント以外のキー(HEADERS / PROTOCOL 等)の per-signal 環境変数取り扱いは
  本変更のスコープ外(現行の lookupSignalEnv 挙動を維持)。

### FR-5: k6 output サーフェス(k6output)
- `--out otel-gen=...` に `metricsEndpoint` キーを追加する(as-is セマンティクス)。(Q5 = A)
- 既存 `endpoint` キーはベースエンドポイントとして FR-1 の補完対象になる
  (この output は metrics のみ送信するため、実質 `v1/metrics` 補完)。
- 優先順位: `metricsEndpoint` > `endpoint`。

### FR-6: ドキュメント / サンプル更新
- README(設定キー一覧・Usage)、`k6otelgen/doc.go` / `k6output/doc.go`、
  `examples/saas-endpoints.md`(Grafana Cloud 手順がベース `/otlp` 指定で動作することを反映)、
  必要に応じて `examples/minimal` / `examples/astroshop` を更新する。
- 破壊的変更(FR-7)の告知を README に明記する。

## Non-Functional Requirements

### NFR-1: OTLP 仕様準拠
- パス構築・優先順位は OTLP Exporter 仕様(上記 Normative Reference)に厳格準拠する。

### NFR-2: 後方互換性(破壊的変更の許容と告知)
- URL 形式ベース `endpoint` の挙動が「パス as-is」→「`v1/{signal}` 補完」に変わる。
  v0.x として破壊的変更を許容し、CHANGELOG / README に明記する。(Q6 = A)
- `host:port` 形式(現行デフォルト `localhost:4317` を含む)の挙動は変わらない。

### NFR-3: テスト(Property-Based Testing 拡張 — Full / blocking)
- パス補完ロジックはプロパティベーステストで検証する(例: 任意の有効ベース URL に対し
  「構築結果は常に `/v1/{signal}` で終わる」「ベースのスキーム・ホスト・既存パス接頭辞が保持される」
  「補完は冪等に適用されない=常に 1 回だけ追記される」)。
- 優先順位解決(per-signal > base > default)も例示ベース + プロパティで検証する。

### NFR-4: 可観測性
- 起動時の `exporter configured` ログに、解決後のシグナル別エンドポイントが分かる情報を含める
  (トラブルシューティングで 404 の原因を特定しやすくするため)。

## Out of Scope

- エンドポイント以外の per-signal 設定(headers / protocol / compression / timeout 等)の分離。
- シグナル別プロトコル(traces=gRPC + metrics=HTTP のような混在)。プロトコルは全シグナル共通のまま。
- OTLP 以外のエクスポーター対応。

## Affected Units

| Unit | Package | 変更内容 |
|---|---|---|
| U4 | `exporter/` | Config 拡張、パス補完ロジック、ConfigFromEnv 修正、exporters.go の per-signal URL 解決 |
| U5 | `k6otelgen/` | `tracesEndpoint` / `metricsEndpoint` / `logsEndpoint` キー、configured ログ |
| U6 | `k6output/` | `metricsEndpoint` キー、exporterConfig マッピング |
| U8 | `examples/`, README | ドキュメント・サンプル更新、破壊的変更告知 |
