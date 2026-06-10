# U6 (k6output) — Functional Design Plan

## ユニットコンテキスト

- **Unit ID**: U6
- **パッケージ**: `k6output/`
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → U2 ✓ → U5 ✓ → **U6 (this)** → U8
- **Purpose** (Application Design より):
  - k6 Output SDK 経由で `--out otel-gen=...` を登録
  - **デュアル機能**:
    - (a) **合成シグナル egress** — U5 と同一の Pipeline (process singleton) を共有、Output lifecycle で flush + shutdown
    - (b) **k6 native metrics → OTLP/Metrics 変換** — `http_req_duration`, `vus`, `iterations` 等を Pipeline 経由で OTLP に送信、`service.name="xk6-otel-gen-runner"`
  - End-of-run summary は **責務外** (k6 標準機構)
  - Pipeline shutdown のトリガ (`Output.Stop()` で `exporter.GetShared().Shutdown(ctx)`)
- **Upstream artifacts**:
  - `aidlc-docs/inception/application-design/component-methods.md` §C6 (Output / Params 型)
  - U1 `topology`, U3 `synth`, U4 `exporter` (Pipeline / GetShared), U5 `k6otelgen` (singleton Pipeline 構築済 or U6 で初構築)
- **k6 SDK 依存**: `go.k6.io/k6/output` (Output / Params / Sample), `go.k6.io/k6/metrics` (Sample / Metric)

## FD で確定すべき事項

FD は「**何をする / どんなドメインルールに従う**」を確定する。U6 FD で扱う事項:

- **`--out otel-gen=...` の syntax** — argument 解析 (`endpoint=...,protocol=...`)
- **`output.Params` → `exporter.Config` の merge ルール** — `--out` arg vs env vs JS configure
- **Pipeline 取得戦略** — U5 がすでに Pipeline 構築している場合は共有、未構築なら U6 で初期化
- **k6 native metrics の OTLP/Metrics への mapping** — どの k6 metric を何の OTel instrument に
- **`AddMetricSamples` の batching 戦略**
- **Resource attributes for k6 metrics** — `service.name="xk6-otel-gen-runner"` + 他
- **`Start` / `Stop` lifecycle と error handling**
- **Pipeline shutdown timing と context**
- **k6 metric naming conventions の semantic 整合**
- **PBT properties** — k6 sample → OTel metric の変換不変条件
- **U7 への generator 追加リクエスト** — k6 SampleContainer の generator

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u6-k6output/functional-design/business-logic-model.md`
- [ ] `aidlc-docs/construction/u6-k6output/functional-design/business-rules.md`
- [ ] `aidlc-docs/construction/u6-k6output/functional-design/domain-entities.md`

---

## 設計確定のための質問

### Question 1: `--out otel-gen=...` の argument syntax

JS-side では `otelgen.configure(opts)` で詳細指定するが、`--out` arg からも config 受け取り可能?

A) **k6 standard k/v syntax**: `--out otel-gen=endpoint=https://...,protocol=grpc,insecure=false` (推奨)
   - argument を `,` で split → `=` で key/value 取得
   - keys: `endpoint`, `protocol`, `insecure`, `headers` (`headers=key1:val1;key2:val2`), `compression`, `timeout`
   - U5 `Configure()` opts と同じ field、ただし `--out` 経由は **JS API より低い** 優先順位

B) **endpoint only**: `--out otel-gen=https://otel.example.com:4317` で endpoint URL 単体指定、その他は env か JS のみ

C) **No args**: `--out otel-gen` のみ受け付け、すべて env or JS API

X) Other

[Answer]: A

---

### Question 2: Config 優先順位 (U6 追加レイヤー)

U5 と U6 が両方 Pipeline を構築する可能性がある場合の merge 順:

A) **JS API > `--out` args > env > built-in defaults** (推奨、`--out` を env より上に置く)
   - `exporter.GetShared(factory)` の factory で merge 実施
   - U5 が先に configure() を呼んでいれば JS が最優先
   - U6 が単独で動く場合 (JS configure なし) は `--out` args が一番優先

B) **`--out` > JS > env > defaults** — `--out` を最強に
   - rationale: `--out` は k6 CLI 直接指定、user intent が最も明確

C) **U6 独自 Pipeline を構築 (U5 と別)** — Pipeline duplication、connection 2 つ
   - U5 と U6 の telemetry を完全分離、別 service.name で送信

X) Other

[Answer]: A

---

### Question 3: Pipeline 共有戦略

U6 が `exporter.GetShared` で Pipeline を取得するタイミング:

A) **`Start()` で `exporter.GetShared(factory)` を呼ぶ** (推奨、k6 lifecycle に合わせる)
   - factory は `--out` args + env + 既存 config で build
   - U5 が先に GetShared を呼んでいれば cache hit、U5 の config が使われる
   - U5 未使用なら U6 が factory に渡した config が使われる

B) **`New(params)` で即 build** — early construction、`Start()` までに connection 確立

C) **遅延 (`AddMetricSamples` 初回まで)** — Output が enabled でも samples が来るまで build しない

X) Other

[Answer]: A

---

### Question 4: k6 native metrics → OTLP mapping

k6 が emit する代表 metrics (`http_req_duration`, `vus`, `iterations`, `data_sent` 等) を OTel metric にどう mapping?

A) **k6 metric type を直接 mapping**:
   - k6 Trend (e.g., `http_req_duration`) → OTel **Histogram** (`k6.http.request.duration`)
   - k6 Counter (e.g., `iterations`) → OTel **Counter** (`k6.iterations`)
   - k6 Gauge (e.g., `vus`) → OTel **UpDownCounter** or **Gauge** (`k6.vus`)
   - k6 Rate (e.g., `http_req_failed`) → OTel **Counter** (rate は計算側に任せる)
   namespace `k6.*` で k6 native の区別を明示 (推奨)

B) **semconv 準拠 namespace に rewrite**:
   - `http_req_duration` → `http.client.request.duration` (semconv)
   - 利点: 既存 OTel dashboard でそのまま見える
   - 欠点: 「k6 がエージェントとして観測した」コンテキストが失われる

C) **k6 metric を素通し** (`http_req_duration` のまま emit) — 最もシンプル、OTel semconv と整合せず

X) Other

[Answer]: A

---

### Question 5: Resource attributes for k6 metrics

U6 が emit する k6 native metrics の Resource attributes:

A) **専用 Resource** (推奨):
   - `service.name="xk6-otel-gen-runner"` (Application Design 通り)
   - `service.version="<xk6-otel-gen のバージョン>"`
   - `telemetry.sdk.*` 一式
   - `k6.test.name="<script filename>"` (k6 が提供すれば)
   - Synth 側の Resource (per-service simulation) とは別扱い

B) **synth と同じ Resource を使い回し** — service.name はトポロジー service と統一

C) **k6 test runner Resource (`k6.*` 属性のみ) を minimal に** — service.name 等の semconv 標準を出さない

X) Other

[Answer]: A - これはk6に関わるメトリクスであって、拡張が生成する疑似テレメトリーではないですよね？疑似テレメトリーでないのであればAでよいです。

---

### Question 6: AddMetricSamples の batching 戦略

`AddMetricSamples(samples)` が大量に呼ばれる場合 (k6 high throughput):

A) **k6 SDK の AddMetricSamples は同期、内部で batch をためる + 定期 flush** (推奨)
   - `Start()` で flush goroutine 起動 (e.g., 1 sec ごと or queue length 閾値)
   - `Stop()` で残り flush + Pipeline.Shutdown
   - `AddMetricSamples` は queue に push のみ、ノンブロッキング

B) **同期 emit (毎 sample で OTel meter 呼び出し)** — 単純だが overhead 大

C) **k6 SDK の internal batching に依存** — k6 が batch_size 等で制御するなら、その間隔で batch 受領

X) Other

[Answer]: A

---

### Question 7: Stop() lifecycle

`Output.Stop()` の責務:

A) **(1) buffered samples を flush → (2) `Pipeline.Shutdown(ctx)` を呼ぶ** (推奨)
   - ctx は内部で timeout 設定 (e.g., 30s)
   - Shutdown 失敗時は warn log だが Stop() は nil を返す (k6 lifecycle を crashed させない)

B) **(1) のみ、Shutdown は別途 (k6 SDK が driving)** — U6 が Pipeline lifecycle を持たない

C) **(1)+(2)+ (3) Pipeline.GetShared をリセット** — process 全体の cleanup

X) Other

[Answer]: A

---

### Question 8: Start() の error 処理

`Output.Start()` 失敗時 (e.g., Pipeline build 失敗、connection refused):

A) **error 返す → k6 が run を abort** (推奨、fail-fast)
   - typo / 不正 endpoint / 未起動 Collector を起動時点で検出
   - k6 process exit、user に明確 feedback

B) **error を warn log、Start は nil で続行** — k6 run が動くが telemetry が出ない (silent failure)

C) **partial Start** — Pipeline 構築失敗でも k6 native metrics conversion は試みる (ただし送信先なし)

X) Other

[Answer]: A

---

### Question 9: k6 metric `http_req_failed` のような Rate 型

k6 Rate metric (`Sample.Value` が 0/1 で aggregate される) の OTel 表現:

A) **`http_req_failed` の各 sample を Counter で +1** (失敗のみカウント、成功は emit しない) — rate は OTel side で `counter / total_iterations` で算出
   namespace: `k6.http.request.failed.total`

B) **Histogram with bucket [0, 1]** — 0/1 分布として記録

C) **専用 Rate metric は OTel 標準にないため、Counter (failed) + Counter (success) の 2 個**

X) Other

[Answer]: A

---

### Question 10: k6 tag → OTel attribute mapping

k6 sample が持つ tags (`group`, `name`, `method`, `status`, `tls_version` 等) を OTel attribute にどう?

A) **k6 tag をそのまま attribute key として渡す** (推奨、原始的だが情報を失わない)
   - prefix `k6.tag.*` で k6 native だと識別
   - 例: `k6.tag.method`, `k6.tag.status`, `k6.tag.group`

B) **semconv にマッピング** — `method` → `http.request.method`, `status` → `http.response.status_code`
   - 注意: k6 tag は HTTP 以外でも使われる場合 mapping が不可解

C) **A + (semconv マッピングを subset で同時付与)** — backward compat と semconv 両立

X) Other

[Answer]: A

---

### Question 11: PBT properties

U6 で扱う property:

A) **3 properties** (推奨):
   - TP-U6-1: AddMetricSamples no-op when nil pipeline — Start 失敗時に AddMetricSamples が panic しない
   - TP-U6-2: Counter monotonic — k6 Counter sample が正の値のみで OTel Counter も monotonic
   - TP-U6-3: Tag → Attribute round-trip — k6 sample.Tags が `k6.tag.*` で OTel attribute として attached

B) A + Sample batching invariant — flush 後に queue length が 0

C) PBT minimal (1 個) — k6 output integration は example-based test 中心

X) Other

[Answer]: A

---

### Question 12: U7 への generator 追加リクエスト

PBT 用に U7 に追加する generator:

A) **2 generator pairs** (推奨):
   - `ValidK6Sample` / `AnyK6Sample` — `metrics.Sample` (Time / Metric / Value / Tags) generator
   - `ValidOutputParams` / `AnyOutputParams` — `output.Params` + `k6output.Params` 構築用 (endpoint / protocol / args の組み合わせ)
   合計 4 関数

B) A + k6 metric type ごとの specialized generator (Trend / Counter / Gauge / Rate)

C) 不要 — k6 SDK の builder を test で直接使う

X) Other

[Answer]: A

---

### Question 13: ファイル分割

`k6output/` のファイル構成:

A) **5 production + 4 test files** (推奨):
   - `doc.go`
   - `output.go` — Output struct + New + Description + Start/Stop
   - `params.go` — Params struct + `--out` args parsing
   - `convert.go` — k6 Sample → OTel metric 変換 (mapping table、tag conversion)
   - `errors.go` — output-specific error wrapping
   - tests: `output_test.go`, `params_test.go`, `convert_test.go`, `pbt_test.go`

B) **`errors.go` 省略** — error type は U5 と同じくシンプルに、`params.go` / `output.go` 内 inline

C) **Feature-based**: `register.go` (init), `lifecycle.go` (Start/Stop), `metric_convert.go`, `args.go`, `params.go`

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 3 つの FD アーティファクト (business-logic-model / business-rules / domain-entities) を生成して承認ゲートへ進みます。
