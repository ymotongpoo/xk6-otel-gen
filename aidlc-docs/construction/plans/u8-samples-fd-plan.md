# U8 (Samples & Distribution) — Functional Design Plan

## ユニットコンテキスト

- **Unit ID**: U8
- **パッケージ / ディレクトリ**: `examples/`, `cmd/xk6-otel-gen-schema/`, project root README, LICENSE
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → U2 ✓ → U5 ✓ → U6 ✓ → **U8 (this) — final unit**
- **Purpose** (Application Design より):
  - `examples/minimal/` — 3 サービス minimal 例 + k6 スクリプト + Docker compose (Collector 起動)
  - `examples/astroshop/` — OTel Demo 由来 10+ サービス例
  - **README** — xk6 ビルド手順、JS API リファレンス、YAML スキーマリファレンス、セキュリティ告知 (プリビルドバイナリ提供だが自前ビルド推奨)
  - `cmd/xk6-otel-gen-schema/` — JSON Schema エクスポートヘルパー
  - **LICENSE** (Apache-2.0)
- **特徴**: U8 は他 unit と異なり **Go package public API を持たない** (distribution / docs / sample artifacts)。FD は「成果物の具体仕様」と「user-facing コンテンツ」を確定する

## FD で確定すべき事項

- **examples/minimal/ の topology 構成** — 何 service、どの edge、どの fault
- **examples/astroshop/ の構成方針** — OTel Demo の何部分を採用、サイズの目安
- **k6 script の典型 pattern** — setup/iteration/teardown の構造
- **Docker compose の構成** — Collector image、port、output destination
- **README の構造** — section 構成、xk6 build 手順、JS API ref、Schema ref、Security ref
- **cmd/xk6-otel-gen-schema の責務** — JSON Schema 出力フォーマット、CLI args
- **LICENSE** — Apache-2.0 確認
- **CI / release** は Build and Test stage の責務 (本 unit では deliverable のみ)
- **PBT properties** — U8 は code が極小なので PBT 最小限 (cmd の args parsing 程度)
- **U7 への generator 追加リクエスト** — 不要 (U8 は test 軽量)

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u8-samples/functional-design/business-logic-model.md`
- [ ] `aidlc-docs/construction/u8-samples/functional-design/business-rules.md`
- [ ] `aidlc-docs/construction/u8-samples/functional-design/domain-entities.md`

---

## 設計確定のための質問

### Question 1: examples/minimal/ topology 構成

3 サービス minimal 例の topology:

A) **古典 3-tier**: frontend → backend → database (推奨)
   - frontend: application, HTTP exposed
   - backend: application, HTTP receives + DB call
   - database: database service
   - 1 journey: `checkout` (frontend → backend → database)
   - 1 fault: backend に error_rate_override=0.1 (10% 失敗)

B) **microservice mesh**: 3 service が複雑に call し合う (cycle なし)

C) **single-service**: 1 service の operations 内訳のみ

X) Other

[Answer]: A

---

### Question 2: examples/astroshop/ の構成

OTel Demo 由来例 (10+ サービス):

A) **OTel Demo (astronomy shop) 構造を完全模倣**:
   - 14 services: frontend, ad, cart, checkout, currency, email, fraud-detection, payment, product-catalog, quote, recommendation, shipping, image-provider, accounting
   - 複数 journey: browse, search, add-to-cart, checkout, place-order
   - fault: 確率的 error / latency / cascade demonstration

B) **OTel Demo を簡略化** (5-7 services) — fully reproduce より maintainable

C) **OTel Demo 由来名前のみ、構造は本ツールに最適化** — 完全自由設計

X) Other

[Answer]: A

---

### Question 3: k6 script の典型 pattern

`examples/*/script.js` で示す pattern:

A) **3 phases**: setup() で load + configure → default function で journey loop → teardown() は空 (U6 が shutdown) (推奨)
   - `import otelgen from "k6/x/otel-gen";`
   - `options = { vus: 10, duration: '30s', }`
   - シンプルな k6 ramp-up + steady state

B) **Stages-based scenarios**: 複数 scenarios で別 journey を駆動
   - browse 70% + checkout 30% の weighted scenarios

C) **Thresholds 付き**: `thresholds: { http_req_failed: ['rate<0.05'], }` 等の k6 SLO 設定例

X) Other

[Answer]: A

---

### Question 4: Docker compose の構成

`examples/*/docker-compose.yaml` で起動する service:

A) **Collector + Jaeger + Prometheus (visualization stack)** (推奨):
   - `otel/opentelemetry-collector-contrib:<tag>` — OTLP 受信 + Jaeger / Prometheus exporter
   - `jaegertracing/all-in-one:latest` — trace 可視化
   - `prom/prometheus:latest` — metric 可視化
   - 利点: user が `docker compose up` → k6 走らせる → ブラウザで Jaeger/Prometheus 観察、フルスタック

B) **Collector のみ** — file_exporter で stdout / JSON file 出力、user が任意 backend を後付け

C) **Collector + Grafana (LGTM stack)** — Loki + Tempo + Mimir + Grafana で fully unified
   - 最も rich だが container 数多い

X) Other

[Answer]: X - これはDocker Composeがいいんですか？Kubernetesがいいです

---

### Question 5: README の構造

project root `README.md` の section:

A) **標準 OSS README** (推奨):
   1. Project description (1-2 paragraphs)
   2. Status / License badges
   3. Quick Start (xk6 build + minimal example の 5 行コマンド)
   4. Features
   5. Building (xk6 with extension)
   6. Usage (JS API reference summary + link to examples)
   7. Topology YAML Reference (link to detailed doc)
   8. Configuration (`--out args` + env vars + JS opts priority)
   9. Examples (link to examples/)
   10. Security (Q10 自前ビルド推奨)
   11. Contributing
   12. License (Apache-2.0)

B) **Tutorial-style** — step-by-step walkthrough with 1 example deeply explained

C) **API Reference 中心** — Quick Start + detailed JS API/YAML schema docs

X) Other

[Answer]: A

---

### Question 6: xk6 build 手順 in README

`Building` section の content:

A) **xk6 build コマンド + ldflags note** (推奨):
   ```bash
   xk6 build --with github.com/ymotongpoo/xk6-otel-gen
   ```
   - 結果として `k6` binary に xk6-otel-gen が link される
   - ldflags で `buildVersion` を埋め込む例 (将来必要なら)
   - 注意: pre-built binary は **配布しない** — security/supply-chain 観点で user に build を強く推奨 (Q10=自前ビルド推奨 と整合)

B) **Pre-built binary 提供 + 自前ビルド両論併記** — GitHub release で binary も配布
   - 利点: 即試せる
   - 欠点: supply chain risk

C) **Pre-built binary のみ + 自前ビルドは advanced** — 簡単指向、ただし供給リスク

X) Other

[Answer]: A

---

### Question 7: cmd/xk6-otel-gen-schema/ の責務

JSON Schema エクスポートヘルパー:

A) **`xk6-otel-gen-schema export [--output schema.json]` で JSON Schema を出力** (推奨)
   - `topology` パッケージの `ExportJSONSchema()` を呼ぶ (U1 で実装済)
   - default `stdout` または `--output` 指定でファイルへ
   - 用途: editor (VS Code 等) で YAML 補完を効かせるための schema 配布

B) **`schema` サブコマンド構成** — `xk6-otel-gen-schema export`, `xk6-otel-gen-schema validate <yaml>` の 2 サブコマンド
   - validate も提供 → CI で topology YAML lint 可能

C) **GUI / web app** — overkill

X) Other

[Answer]: A

---

### Question 8: cmd CLI library の選択

`cmd/xk6-otel-gen-schema/main.go` の CLI parsing:

A) **`flag` (stdlib)** (推奨、シンプル) — `flag.StringVar` で `--output`、外部依存なし

B) **`spf13/cobra`** — サブコマンド充実、Q7=B を選んだ場合に効果的

C) **`urfave/cli/v2`** — middle ground

X) Other

[Answer]: A

---

### Question 9: examples の test 戦略

`examples/*/` を CI でどう test する?

A) **build only test**: `go build` で k6 binary が xk6 経由で build できることを CI で確認 (推奨)
   - 実 k6 run は重い、`k6 run` までは Build and Test stage の責務
   - examples/ 自体は文書 + sample のみ、Go test は無し

B) **smoke test**: `k6 run` を CI で実行、exit code 0 を確認
   - U5/U6 integration test に既に類似 setup あるので重複

C) **examples ごとに `go test`** — sample に embedded test を入れる

X) Other

[Answer]: A

---

### Question 10: README の Security section 内容

Q10 (Application Design 言及) の "プリビルドバイナリ提供だが自前ビルド推奨":

A) **自前ビルドのみ提供、prebuilt 配布なし** (推奨、簡素 + 安全)
   - rationale: supply chain attack 防止、user が `xk6 build` を control
   - README Security section で明示

B) **prebuilt + signature verification 案内** — checksum / GPG / cosign
   - 利点: 即試せる + integrity 確保
   - 欠点: release pipeline 複雑、key 管理

C) **両方提供、文書で warn** — 妥協策

X) Other

[Answer]: A

---

### Question 11: examples の license

`examples/` 内 sample 用 license:

A) **project LICENSE (Apache-2.0) と一致** (推奨) — 統一、各 file に license header 不要

B) **examples は public domain (CC0)** — user が freely fork/modify

C) **examples は MIT** — Apache 2.0 より緩い

X) Other

[Answer]: A

---

### Question 12: PBT / test の有無

U8 で扱う PBT property:

A) **PBT なし、cmd の args parsing は example-based test** (推奨)
   - examples/ は static file のみ
   - `cmd/xk6-otel-gen-schema/` は flag parse → topology.ExportJSONSchema 呼び出すだけ、property 化メリット小

B) **cmd の output format に対する property** — JSON Schema が valid JSON Schema Draft 2020-12 仕様準拠

C) Other

X) Other

[Answer]: A

---

### Question 13: ファイル分割 / ディレクトリ構成

U8 の deliverable layout:

A) **以下の構成** (推奨):
   ```text
   examples/
   ├── minimal/
   │   ├── topology.yaml
   │   ├── script.js
   │   ├── docker-compose.yaml
   │   ├── otel-collector-config.yaml
   │   └── README.md
   └── astroshop/
       ├── topology.yaml
       ├── script.js
       ├── docker-compose.yaml
       ├── otel-collector-config.yaml
       └── README.md
   
   cmd/
   └── xk6-otel-gen-schema/
       ├── main.go
       └── main_test.go
   
   README.md  (project root, full project doc)
   LICENSE     (Apache-2.0 fulltext)
   ```

B) **examples/ を flat 化** — `examples/minimal.yaml`, `examples/minimal.js` 等

C) **docs/ separate directory** — README とは別に docs/ で詳細

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 3 つの FD アーティファクトを生成して承認ゲートへ進みます。
