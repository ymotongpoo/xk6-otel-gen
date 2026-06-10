# U6 (k6output) — NFR Requirements Plan

## ユニットコンテキスト

- **Unit ID**: U6
- **パッケージ**: `k6output/`
- **FD**: `aidlc-docs/construction/u6-k6output/functional-design/` (committed 507c719)
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → U2 ✓ → U5 ✓ → **U6 (this — NFR-R)** → U8

## NFR-R で確定する事項

FD で「何をする」を確定済。NFR-R は **「何を達成するか」** を測定可能・テスト可能な閾値として確定する。

中心テーマ:
- **Performance**: AddMetricSamples per-sample overhead、flush loop ratio、Start/Stop latency
- **Reliability**: queue full backpressure、Pipeline shutdown failure、k6 lifecycle 保護
- **Concurrency**: queue channel safety、flush goroutine と Stop の race
- **Observability**: U6 自身の self-metric (持つか持たないか)
- **API Stability**: k6 SDK contract のみ public、`--out` args の SemVer
- **Documentation**: GoDoc + `--out` usage example
- **Testability**: k6 SDK mock、Coverage target、Integration test
- **Compatibility**: k6 output SDK version、Go 1.25

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u6-k6output/nfr-requirements/nfr-requirements.md`
- [ ] `aidlc-docs/construction/u6-k6output/nfr-requirements/tech-stack-decisions.md`

---

## 設計確定のための質問

### Question 1: Start() latency budget

`Output.Start()` の所要時間:

A) **< 100 ms** (推奨) — Pipeline 構築 (NFR-U4-6 と整合) + runner MeterProvider 構築 + instrument 構築 + flush goroutine 起動

B) **< 50 ms** — より厳しく

C) **No target** — k6 startup で 1 回だけ

X) Other

[Answer]:　C - 100ms前後はあくまで目安でよいです。

---

### Question 2: AddMetricSamples overhead per sample

`AddMetricSamples(samples)` の sample 1 個あたり overhead (queue push):

A) **< 1 µs / sample** (推奨、queue push のみで重い処理なし)

B) **< 100 ns / sample** — 厳しく、極めて high throughput k6 でも目立たない

C) **No target** — flush 経路で律速

X) Other

[Answer]: A

---

### Question 3: Flush loop overhead

flush goroutine の overhead (sample → OTel meter Record 1 個あたり):

A) **< 5 µs / sample** (推奨、instrument lookup + attribute set + Record)

B) **< 1 µs / sample** — sync.Map + cache hit でほぼ pure record

C) **No target** — backend が律速

X) Other

[Answer]: A

---

### Question 4: Stop() latency budget

`Output.Stop()` の所要時間:

A) **< 30 sec** (推奨、Pipeline.Shutdown timeout がここ + buffer flush) — k6 が graceful shutdown を待つ範囲

B) **< 10 sec** — 厳しく、large buffer flush も含む

C) **No target** — best-effort

X) Other

[Answer]: C - 30 secは目安で良いです

---

### Question 5: Memory overhead

U6 Output instance の memory footprint:

A) **< 10 MB** (推奨) — queue (100 entries × ~10KB) + instrument map + runner Resource、典型値

B) **< 1 MB** — queue を縮小、instrument lookup を minimal に

C) **< 100 MB** — 高 cardinality 想定、attribute set cache が大きい場合

X) Other

[Answer]: X - Aで行っているような計算でカーディナリティの高さによって都度計算で求める方が良いと思います

---

### Question 6: Self-metric for U6

`*Output` 自身の self-metric:

A) **持たない** (推奨、U2/U3/U5 と同方針) — Pipeline.Stats で送信側を観測可能

B) **debug counter** — `SamplesReceived`, `SamplesEmitted`, `QueueDrops` 等の atomic counter

C) **k6 metric として出す** — `k6.otelgen.output.samples_total` 等

X) Other

[Answer]: A

---

### Question 7: Queue full handling

queue が full (100 entries) の場合の挙動:

A) **drop oldest + warn log** (推奨、k6 throughput を阻害しない)
   - `drops_total` を internal counter で記録 (debug 時に確認)
   - 1 sec ごとの warn log で flooding 防止

B) **block until space** — sample loss なし、ただし k6 throughput を遅らせるリスク

C) **drop newest** — A の逆、最新は failure mode で argued

X) Other

[Answer]: A - これはqueueのサイズは大きくすると他のユニットに影響あるんでしたっけ？ないなら大きさは調整できるようにすると良いような気がしますが、どうでしょうか。

---

### Question 8: SemVer commitment

API stability:

A) **`--out otel-gen=<args>` の args syntax は post-v1 で SemVer 厳守** (推奨)
   - key 値の追加 = minor
   - 既存 key の意味変更 / 削除 = major
   - k6 user との contract

B) **Go-side `New` signature のみ public、k6 SDK contract に従う** — public API surface 最小

C) A + B 両方

X) Other

[Answer]: A

---

### Question 9: Coverage target

`go test -cover ./k6output/...`:

A) **80%** (推奨、U2/U3/U4/U5 と同じ)

B) **85%+** — convert.go の table-driven mapping を網羅

C) **75%** — flush goroutine タイミング系が test しにくい

X) Other

[Answer]: A

---

### Question 10: Example function 数

`k6output/doc_test.go`:

A) **1 件**: `ExampleNew` — Go API は薄い (k6 SDK が呼ぶ) (推奨)

B) **0 件** — doc.go の comment + integration test で十分

C) **3 件**: A + `ExampleOutput_Start`, `ExampleOutput_Stop` — 全 method

X) Other

[Answer]: A

---

### Question 11: Integration test 範囲

U6 の integration test:

A) **`k6output/integration/` + `-tags=integration`** (推奨、xk6 build + 実 k6 run + Collector):
   - 実 `--out otel-gen=...` で run
   - k6 native metrics が `k6.*` namespace で Collector に届くことを確認
   - U5 と同じ harness を流用 (xk6 build helper を共有)

B) **U5 integration test を拡張** — `--out` 経路は U5 から U6 を呼ぶので統合

C) **Integration test なし** — unit test (k6 SDK mock) のみ

X) Other

[Answer]: A

---

### Question 12: 「これは N/A」の明示

U6 で扱わない NFR カテゴリ:

A) **N/A セクション** (U2-U5 と同パターン):
   - Persistence
   - Authn/Authz
   - Encryption at rest / in transit (U4 が担保)
   - i18n / a11y
   - GDPR/SOC2
   - Production monitoring SLO
   - Disaster recovery
   - Backup / Restore

B) A + cloud-specific 想定追加

C) N/A 列挙省略

X) Other

[Answer]: A

---

### Question 13: Cardinality safeguard

k6 sample tags が無限に増える potential (e.g., `name` = URL with query string が無限):

A) **U6 内で safeguard なし、Collector 側で対応** (推奨、k6 user の責務)
   - k6 script 内で `tags` を制限する慣例
   - Collector の processors で cardinality limit を設定推奨

B) **U6 内 attribute set cache に upper bound** (e.g., 10,000 unique sets) — exceeded で warn + skip

C) **U6 で tag を whitelist 化** — known tag のみ通す

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR-R アーティファクトを生成して承認ゲートへ進みます。
