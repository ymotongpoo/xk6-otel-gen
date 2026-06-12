# Per-Signal Endpoint Support — Requirement Verification Questions

エンドポイント設定の 2 パターン対応(ベースエンドポイント + パス補完 / シグナル別エンドポイント)について、
設計を確定するための確認質問です。各質問の `[Answer]:` タグの後に選択肢の文字を記入してください。

**背景**: 現在の実装は単一の `endpoint` 設定を 3 シグナル(traces/metrics/logs)で共有し、
URL 形式の場合は `WithEndpointURL`(パスを **そのまま** 使う per-signal セマンティクス)で
エクスポーターに渡しています。このため Grafana Cloud のようにベースパス(`/otlp`)を持つ
エンドポイントでは `/v1/{signal}` が補完されず 404 になります。

**設定サーフェスは 3 箇所**: JS `configure()` (k6otelgen) / `--out otel-gen=key=value` (k6output) /
`OTEL_EXPORTER_OTLP_*` 環境変数 (exporter.ConfigFromEnv)。

## Question 1
JS `configure()` でのシグナル別エンドポイントの指定形式はどうしますか?

A) フラットキー方式: `tracesEndpoint` / `metricsEndpoint` / `logsEndpoint` を既存の `endpoint` と並べる
   (OTel 環境変数 `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` 等との対応が直感的。既存 config のフラット構造と整合)

B) ネストオブジェクト方式: `endpoints: { traces: "...", metrics: "...", logs: "..." }` を追加する
   (グルーピングは明確だが、既存のフラットなキー構造から逸脱)

C) Other (please describe after [Answer]: tag below)

[Answer]: A

## Question 2
ベースエンドポイント(共通 `endpoint`)の HTTP パス補完セマンティクスはどうしますか?
OTLP 仕様では `OTEL_EXPORTER_OTLP_ENDPOINT` のパスに `v1/{signal}` を追記します
(例: `https://host/otlp` → `https://host/otlp/v1/traces`)。

A) OTLP 仕様に厳格準拠: 常にパス末尾へ `v1/{signal}` を追記する。
   既に `/v1/traces` 等で終わっていても追記する(仕様どおり。誤設定はユーザー責任、ドキュメントで案内)

B) 仕様準拠 + 重複ガード: パスが既に `/v1/{signal}` で終わっている場合は追記しない
   (現行動作からの移行で `https://host:4318/v1/traces` のような既存設定を壊さない救済になるが、暗黙的な挙動)

C) Other (please describe after [Answer]: tag below)

[Answer]: A - このルールに従う https://opentelemetry.io/docs/specs/otel/protocol/exporter/#endpoint-urls-for-otlphttp

## Question 3
シグナル別エンドポイント(パターン 2)のパスセマンティクスはどうしますか?
OTLP 仕様では per-signal 環境変数(`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` 等)の URL は
**そのまま**(as-is)使用され、パス補完は行われません。

A) OTLP 仕様準拠: 指定された URL をそのまま使用する(パス補完なし)。
   フルパス(`https://host/otlp/v1/traces` など)をユーザーが明示する

B) シグナル別でも `v1/{signal}` を補完する(仕様から逸脱するが、入力は短くなる)

C) Other (please describe after [Answer]: tag below)

[Answer]: A

## Question 4
環境変数サポートの修正範囲はどうしますか?
現在の `ConfigFromEnv` は `OTEL_EXPORTER_OTLP_{TRACES|METRICS|LOGS}_ENDPOINT` を
「最初に見つかった値」として共有 Config に適用しており、シグナル別に効きません。

A) OTLP 仕様準拠に修正: `OTEL_EXPORTER_OTLP_{SIGNAL}_ENDPOINT` は該当シグナルのみに適用(as-is)、
   `OTEL_EXPORTER_OTLP_ENDPOINT` はベースとして全シグナルに適用(パス補完あり)

B) 環境変数は今回のスコープ外とし、JS config と --out args のみ対応する
   (ConfigFromEnv は現状のまま)

C) Other (please describe after [Answer]: tag below)

[Answer]: A

## Question 5
k6 output (`--out otel-gen=...`) の対応はどうしますか?
この output が送信するのは k6 ネイティブメトリクス(metrics シグナルのみ)です。

A) `endpoint`(ベース、パス補完あり)に加えて `metricsEndpoint`(as-is)もサポートする
   (JS config 側とキー体系を一貫させる)

B) `endpoint` のみ対応(ベースエンドポイントのパス補完だけ修正)。
   metrics 専用エンドポイントが必要なら endpoint にフルパス指定はできないため非対応とする

C) Other (please describe after [Answer]: tag below)

[Answer]: A

## Question 6
後方互換性の扱いはどうしますか?
現行の URL 形式 `endpoint` は「パス as-is」なので、`https://host:4318/v1/traces` のような
フルパス指定で traces だけ動かしていた利用者がいる場合、ベース補完への変更で挙動が変わります。

A) 破壊的変更として許容する(v0.x であり OTLP 仕様準拠を最優先。CHANGELOG / README に明記)

B) 重複ガード(Q2=B)で既存フルパス設定を実質的に救済する

C) Other (please describe after [Answer]: tag below)

[Answer]: A
