# U8 (Samples & Distribution) — NFR Requirements Plan

## ユニットコンテキスト

- **Unit ID**: U8
- **パッケージ / ディレクトリ**: `examples/`, `cmd/xk6-otel-gen-schema/`, project root README, LICENSE
- **FD**: `aidlc-docs/construction/u8-samples/functional-design/` (committed caa82a7)
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → U2 ✓ → U5 ✓ → U6 ✓ → **U8 (this — NFR-R)** — **final unit**

## NFR-R で確定する事項

FD で「何をする」を確定済 (deliverable 一覧 + Kubernetes + LGTM-lite stack 採用)。NFR-R は **deliverable の品質基準** を確定する。

中心テーマ:
- **cmd/xk6-otel-gen-schema/** の performance / reliability
- **examples の validation** — topology.yaml が U1.Validate を passes、k8s manifest が kubectl apply --dry-run を passes
- **README link integrity** — broken link / dead URL の検出
- **Image tag pinning** — Collector / Tempo / Loki / Prometheus / Grafana の specific version
- **Documentation completeness** — JS API ref / YAML schema ref / Building / Security の網羅
- **License & SPDX header consistency** — Apache-2.0 統一
- **CI build check** — `go build ./cmd/...` + xk6 build success
- **Security / SBOM** — supply chain hint (prebuilt なし方針の reinforcement)
- **Maintainability** — astroshop の OTel Demo upstream 変更追随性
- **Tech stack** — kind / kubectl / xk6 等の prerequisite version

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u8-samples/nfr-requirements/nfr-requirements.md`
- [ ] `aidlc-docs/construction/u8-samples/nfr-requirements/tech-stack-decisions.md`

---

## 設計確定のための質問

### Question 1: cmd/xk6-otel-gen-schema/ performance budget

`xk6-otel-gen-schema` 実行時間の target:

A) **< 100 ms (one-shot CLI)** (推奨) — JSON Schema 出力 + file write のみ、user の体感即時

B) **< 50 ms** — より厳しく

C) **No target** — CLI は 1 回実行のみ、performance を測らない

X) Other

[Answer]: C - 数十秒とかかかるなら考えものだけど、1秒とかでも全然問題ないし、このあたりはベストエフォートでいいです

---

### Question 2: examples の validation 範囲

CI で examples をどこまで validate?

A) **topology.yaml の Validate + k8s manifest の dry-run + script.js の構文 check** (推奨)
   - `go test` 内で `topology.Parse(yaml)` + `Schema.Validate()` を各 example に適用
   - `kubectl apply --dry-run=server -k examples/*/k8s/` を CI (cluster あり) で
   - script.js は k6 binary が build できれば構文 OK

B) **A + 実 k6 run smoke test** — Docker Collector + xk6 + k6 run まで実行
   - U5/U6 integration test に重複だが U8 でも追加 (safety net)

C) **基本 syntax check のみ** — Validate / dry-run なし、minimal

X) Other

[Answer]: A

---

### Question 3: README link integrity

`README.md` 内 URL の broken link 検出:

A) **CI で broken link check (markdown-link-check 等)** (推奨)
   - `lychee` or `markdown-link-check` を CI で run
   - external link は HEAD request、internal link は file existence check

B) **手動 review のみ** — link breakage は別途検出

C) **internal link のみ check、external は skip** — external は flaky

X) Other

[Answer]: A

---

### Question 4: Image tag pinning policy

Collector / Tempo / Loki / Prometheus / Grafana の image version:

A) **specific tag pin + 半年ごと dependabot で bump 検討** (推奨)
   - 各 image を **major.minor.patch** で pin (e.g., `grafana/tempo:2.6.0`)
   - dependabot で半年ごとに PR、breaking change を check しながら手動 merge

B) **latest tag を使う** — 常に最新、ただし re-deploy で挙動変化リスク

C) **major のみ pin** (e.g., `grafana/tempo:2`) — 中間策

X) Other

[Answer]: A - 半年は遅いので1ヶ月ごとくらいにbumpしてもらいたいです

---

### Question 5: Documentation completeness

README が必ず含むべき内容:

A) **JS API ref + YAML schema ref + xk6 Build + `--out args` 全 keys + env vars + Security + Configuration priority + Examples + License** (推奨、Q5=A の 12 sections と整合)
   - 各 section に **具体例**を最低 1 つ
   - `cmd/xk6-otel-gen-schema` の使い方も含める

B) **Brief overview + link to godoc** — godoc が API ref を持つので README は薄く

C) **Tutorial-style** — single end-to-end walkthrough

X) Other

[Answer]: A

---

### Question 6: License & SPDX header consistency

`.go` ファイルの SPDX header:

A) **全 `.go` ファイルに SPDX header 必須** (推奨、U1-U6 と一貫)
   - `// SPDX-License-Identifier: Apache-2.0`
   - examples 内 `.js` ファイルにも optional で付与

B) **`cmd/xk6-otel-gen-schema/main.go` のみ必須、test file 等は optional**

C) **SPDX header なし、LICENSE ファイルで covers** — 慣例から外れる

X) Other

[Answer]: A

---

### Question 7: CI build check

`go build` + xk6 build の CI 検証:

A) **`go build ./cmd/...` + `xk6 build --with .` 両方を CI で実行** (推奨)
   - cmd binary が build できることを確認
   - xk6 build で k6 binary に linkable であることを確認 (U5/U6 integration test と類似だが U8 独自 phase で確認)

B) **`go build ./cmd/...` のみ** — xk6 build は U5/U6 integration で確認済

C) **xk6 build のみ** — k6 ecosystem の health check が main

X) Other

[Answer]: A

---

### Question 8: Security / SBOM

Security 関連:

A) **README に supply chain hint + (将来) SECURITY.md placeholder** (推奨)
   - prebuilt なし方針を README で明示 (Q10=A 通り、既決)
   - SECURITY.md は project が成熟したら追加 (現状 TODO)
   - SBOM は CI で `cyclonedx-gomod` 等で出す案あり (将来)

B) **A + 即時 SBOM 生成** — supply chain visibility 重視

C) **A + GPG signature for releases** — prebuilt 配布しない方針なので不要

X) Other

[Answer]: A

---

### Question 9: examples の maintainability (astroshop OTel Demo 追随)

OTel Demo (astronomy shop) は upstream で進化する。本 project の astroshop:

A) **作成時の OTel Demo snapshot を base に、upstream 追随は手動 (年 1 回程度 review)** (推奨)
   - upstream 完全同期は維持コスト大
   - example として "OTel Demo を topology 表現したらこうなる" を示す価値で十分

B) **automated sync** — script で OTel Demo の構造を pull、本 project の topology.yaml に反映
   - 実装複雑、メリット小

C) **astroshop を `examples/large/` 等に汎用化** — OTel Demo 依存を弱める

X) Other

[Answer]: A - 手動で追従と行ってもAIツールを使って対応することになるので、そのためのプロジェクトskillを作っておいてもらえると助かります

---

### Question 10: cmd/xk6-otel-gen-schema/ の coverage

`cmd/xk6-otel-gen-schema/` の test coverage target:

A) **70%** (推奨、CLI は flag parsing + stdlib I/O のみで test 簡素)
   - main() 自体は test 困難 (run function 分離が普通)

B) **80% (他 unit と同じ)** — 厳しく

C) **No target** — single binary、bug は immediate に発見される

X) Other

[Answer]: A

---

### Question 11: SemVer commitment

API stability 約束:

A) **`cmd/xk6-otel-gen-schema` の CLI args は post-v1 SemVer 厳守** (推奨)
   - `-output` flag は immutable
   - 新 flag 追加は minor、既存 flag の削除は major
   - examples の path / file 名も SemVer 対象 (user の手元 script が依存しうる)

B) **examples の path / file 名は freely 変更可** — 配置の安定性は guarantee しない

C) **CLI のみ SemVer、examples は free** — 中間

X) Other

[Answer]: A

---

### Question 12: 「これは N/A」の明示

U8 で扱わない NFR カテゴリ:

A) **N/A セクション** (U2-U6 と同パターン):
   - Persistence (deliverable は static)
   - Authentication / Authorization
   - Encryption at rest / in transit
   - Internationalization (i18n)
   - Accessibility (a11y) — 例外: Grafana dashboards に最低限の accessibility config
   - GDPR/SOC2 (合成データ)
   - Production monitoring SLO
   - Disaster recovery
   - Backup/Restore
   - Capacity planning

B) A + LGTM stack の operational tuning (Tempo retention 等)

C) N/A 列挙省略

X) Other

[Answer]: A

---

### Question 13: Compatibility constraints

prerequisite tools の version constraints:

A) **README に minimum version 明記** (推奨):
   - Go 1.25+
   - xk6 latest (k6 が pinning する version)
   - kubectl 1.27+
   - kind 0.20+ (任意、local cluster setup 用)
   - Docker (xk6 build 用、container backend)

B) **明記なし、user が自分の environment で工夫** — k6 慣例 (k6 自体が minimal な constraint 表記)

C) **CI で testした version のみ明記** — actual tested version を表に

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR-R アーティファクトを生成して承認ゲートへ進みます。
