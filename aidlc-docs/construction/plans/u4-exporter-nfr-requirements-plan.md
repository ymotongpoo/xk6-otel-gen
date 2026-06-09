# U4 (exporter) — NFR Requirements Plan

## ユニットコンテキスト

- **Unit ID**: U4
- **パッケージ**: `exporter/`
- **Functional Design**: `aidlc-docs/construction/u4-exporter/functional-design/`
- **位置づけ**: Infrastructure layer。OTel Go SDK 直接 import の唯一の unit。Pipeline 構築 / Shared Holder / Stats / Shutdown を担う

## NFR スコープ

U4 は **OTel SDK 統合の境界**:
- 多数の OTel SDK 依存追加 (otlp{trace,metric,log}{grpc,http})
- 外部 OTLP endpoint への送信が成立する性能・信頼性が要求 (1k RPS、NFR-1)
- Pipeline は per-process singleton で複数 VU から同時参照
- Integration test (real Collector 起動) が U4 で初めて必要に

中心となる NFR:
- **OTel SDK 依存のバージョン方針** — 多数の otlp サブモジュール
- **性能** — `New < 100ms`, `Stats < 1µs`, `Shutdown` SLA、batch processor チューニング
- **並行 safety** — Pipeline は per-process singleton、Stats は per-field atomic
- **Integration test** — real OTel Collector を Docker compose で起動して送信検証

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u4-exporter/nfr-requirements/nfr-requirements.md`
- [ ] `aidlc-docs/construction/u4-exporter/nfr-requirements/tech-stack-decisions.md`

---

## 設計確定のための質問

### Question 1: OTel Go SDK のモジュール群選定

OTel Go SDK は多数の細分化モジュールに分かれています。U4 で import するもの:

A) **最小セット** — `otel/sdk/{trace,metric,log,resource}` + `otel/exporters/otlp/otlptrace{,grpc,http}` + `otel/exporters/otlp/otlpmetric{grpc,http}` + `otel/exporters/otlp/otlplog{grpc,http}` + `otel/attribute` (推奨、これだけで FD の要件すべて達成)

B) A + `otel/propagation` (将来 trace context 伝播が必要なら) — 本 unit では未使用、Out of Scope

C) A + `otel/exporters/stdout/*` (デバッグ用 stdout exporter) — Out of Scope、開発者は手動で別途 import

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 2: OTel SDK のバージョン方針

A) **最新 stable に追従** — `go get -u go.opentelemetry.io/otel/...`、dependabot で自動 PR (推奨、OTel SDK は active development)

B) **特定 minor で固定** — 例 `v1.30.x`、安定性最優先

X) Other (please describe after [Answer]: tag below)

[Answer]: A - 開発中も最新安定版に追従してください

---

### Question 3: Integration test の起動方針

`real OTel Collector` への送信検証 (NFR-4.1 = Unit + Integration tests) は U4 で初めて必要に。

A) **`testing.Short()` で skip 可能な build-tag-gated test** — `//go:build integration` 付きファイルに分離し、CI で実 Collector 起動、ローカル開発では skip 可 (推奨)

B) **常時実行、Docker compose は test 内で起動** — testcontainers-go 等で in-process に Collector を起動

C) **本 unit では integration test を書かない、Build and Test ステージで一括対応**

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 4: Integration test の検証内容

U4 の integration test で何を検証する?

A) **`New(Config)` → 3 信号 each で 1 サンプル送信 → Collector が `otelcol/file_exporter` で書き出した JSON ファイルを assert** — エンドツーエンドの最小確認 (推奨)

B) A + Collector の Prometheus metrics を読んで件数一致確認 — より精密

C) **TLS / gzip / headers 各種設定をすべてカバー** — comprehensive

X) Other (please describe after [Answer]: tag below)

[Answer]: A - ただ可能ならcorrelationできてほしい

---

### Question 5: Pipeline.New 性能目標

A) **`New(Config)` < 100 ms** (gRPC 接続確立込み、再試行なし) (推奨、FD §11 と一致)

B) **< 50 ms** — より厳密

C) **目標値なし** — k6 init time にあまり影響しない想定

X) Other (please describe after [Answer]: tag below)

[Answer]: A 

---

### Question 6: 大量 export 時の性能

`1k RPS の k6 ジャーニー` シナリオで、3 信号 × 1k RPS の export 負荷:

A) **`New` 後の steady-state で CPU < 10%` (4 vCPU)** + バックプレッシャー時に span drop はあっても OK (k6 ラン継続) — 現実的 (推奨)

B) **No drop の保証** — MaxQueueSize を大きくする、バックプレッシャーを `New` の段階で警告

C) **目標値なし** — 動けば良い

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 7: Stats 読み出しの性能

`(*Pipeline).Stats()` の性能目標:

A) **< 1 µs** — atomic.Load × 9 つで自然達成 (推奨)

B) **< 10 µs** — 余裕を持つ

X) Other (please describe after [Answer]: tag below)

[Answer]: 特に要件はないです。100msくらいかかったところでツールの性能には影響ないので。

---

### Question 8: U4 自身の coverage 目標

A) **80% 以上** — U7/U1 と統一 (推奨)

B) **70% 以上** — 一部の OTel SDK 統合パスはテスト困難

C) **90% 以上** — Infrastructure 層こそ厳密に

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 9: コードカバレッジ計測の test 種類

`go test -cover` で計測するテスト範囲:

A) **Unit test のみ** — Integration test (build tag) は coverage 計算外、`go test -cover` でも unit のみ走る (推奨、CI 簡素)

B) **Unit + Integration の合算** — `go test -tags=integration -cover ./...` で計算

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 10: 可観測性 (logging)

U1 と同様、U4 もライブラリ内ログ出力なし?

A) **ログ出力なし** — エラーは戻り値のみ、Stats は API 経由で公開 (推奨、U1 と統一)

B) **`slog.Default()` でデバッグログ出力可能** — `slog` 依存追加

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 11: 後方互換性ポリシー

A) **U7/U1 と同じ** — v1.0.0 リリース前 break OK、以降 SemVer 厳守、`// Deprecated:` で deprecation pattern (推奨)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 12: その他の NFR の N/A 一覧

A) **明示** — U7/U1 と同じく、適用外の NFR (i18n / a11y / 永続化 / 認証認可 等) を明示列挙 (推奨)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR Requirements アーティファクトを生成して承認ゲートへ進みます。
