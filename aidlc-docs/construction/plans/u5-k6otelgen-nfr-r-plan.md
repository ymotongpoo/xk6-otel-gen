# U5 (k6otelgen) — NFR Requirements Plan

## ユニットコンテキスト

- **Unit ID**: U5
- **パッケージ**: `k6otelgen/`
- **FD**: `aidlc-docs/construction/u5-k6otelgen/functional-design/` (committed 6c9743b)
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → U2 ✓ → **U5 (this — NFR-R)** → U6 → U8

## NFR-R で確定する事項

FD で「何をする」を確定済。NFR-R は **「何を達成するか」** を測定可能・テスト可能な閾値として確定する。

中心テーマ:
- **Performance**: init/load/configure/RunJourney 各 latency target、per-VU instance 構築コスト、メモリ overhead
- **Reliability**: VU 並列での state 共有 race-free、初期化失敗 (load 不正 YAML、configure 不正 opts) の挙動
- **Concurrency**: process singleton vs per-VU の境界保証
- **Observability**: U5 自身の self-metric (持つか持たないか)
- **API Stability**: JS API surface の SemVer
- **Documentation**: GoDoc + JS 利用 example + `--out` warning
- **Testability**: modulestest.NewRuntime + mock synth/engine、Coverage target
- **Compatibility**: k6 SDK version pinning、Go 1.25、grafana/sobek

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u5-k6otelgen/nfr-requirements/nfr-requirements.md`
- [ ] `aidlc-docs/construction/u5-k6otelgen/nfr-requirements/tech-stack-decisions.md`

---

## 設計確定のための質問

### Question 1: Init / Module construction latency

`k6 init` phase で発生する処理 (`modules.Register` + `New()`) の latency target:

A) **< 1 ms** (推奨) — `RootModule` の zero-init のみで heavy work は load() / configure() に遅延

B) **No target** — init は 1 回だけなので問題視しない

C) **< 100 µs** — k6 init を pure に保つ

X) Other

[Answer]:　B

---

### Question 2: Load(path) latency budget

`otelgen.load(path)` の所要時間:

A) **< 50 ms** (推奨) — typical YAML (services < 50, journeys < 10)、Parse + Validate + ApplyFaults 込み

B) **< 10 ms** — より厳しく

C) **< 200 ms** — 大規模 topology (100 services 等) 想定

X) Other

[Answer]: A - でも厳密にこの目標を目指さなくていいですよ

---

### Question 3: NewModuleInstance(vu) latency budget

各 VU で 1 回呼ばれる per-VU instance 構築の latency:

A) **< 5 ms** (推奨、k6 init phase で VU 数分繰り返される、典型 VU=100 で total 500ms)

B) **< 1 ms** — 厳しく、高 VU 数で k6 startup を遅延させない

C) **< 50 ms** — 余裕を持たせる

X) Other

[Answer]: A

---

### Question 4: RunJourney overhead budget

`handle.runJourney(name)` の **journey 実行時間を除いた overhead** (BuildPlan cache hit + Execute dispatch + JS exception handling):

A) **< 100 µs** (推奨、k6 iteration hot path)

B) **< 50 µs** — より厳しく

C) **No explicit target** — Engine 自体の overhead で律速

X) Other

[Answer]: C - 数ミリ秒くらいなら全然許容できるので、無理はしないで

---

### Question 5: Memory per VU

VU 1 個あたりの `*ModuleInstance` メモリ overhead:

A) **< 200 KB** (推奨) — Engine + Synthesizer + small map references。典型 VU=1000 で total < 200 MB

B) **< 100 KB** — より厳しく、高 VU で k6 メモリ制約

C) **< 500 KB** — 余裕を持たせる

X) Other

[Answer]: A

---

### Question 6: Self-metric for k6otelgen

`*RootModule` / `*ModuleInstance` の自己観測 metric:

A) **持たない** (推奨、U2/U3 と同方針) — Pipeline.Stats は U4 の責務、k6 native metrics は U6 の責務、U5 自体は薄い frontend

B) **debug counter** — `LoadCalls`, `ConfigureCalls`, `RunJourneyCalls` 等の atomic counter

C) **k6 metric として emit** — k6.metrics 経由で `k6otelgen.runjourney.total` 等を出す

X) Other

[Answer]: A

---

### Question 7: Error の Kind enum SemVer 扱い

`*ConfigError.Kind` の string enum を SemVer でどう扱う?

A) **post-v1 で Kind 値追加は minor bump、既存 Kind の意味変更 / 削除は major bump** (推奨)

B) **Kind 値は internal、外部に公開しない** — JS-side では error.message のみで判別

C) **Kind 値を JS error property に露出して post-v1 で immutable contract** — 厳しい

X) Other

[Answer]: A

---

### Question 8: load() の file path security

JS から `otelgen.load("../../../etc/passwd")` のような path traversal をどう扱う?

A) **k6 SDK の filesystem sandbox に従う** (推奨) — k6 が `--allow-list` 等で制限している前提、U5 内部で追加 check しない

B) **U5 内部で `filepath.Clean` + traversal 拒否** — 多層防御

C) **Working directory 配下のみ許可** — 一番厳しいが、k6 慣例と齟齬

X) Other

[Answer]: A

---

### Question 9: Coverage target

`go test -cover ./k6otelgen/...`:

A) **80%** (推奨、U2/U3/U4 と同じ)

B) **85%+** — JS layer は branch heavy

C) **70%** — sobek mock が必要な部分は test しにくい

X) Other

[Answer]: A

---

### Question 10: Example function 数

`k6otelgen/doc_test.go`:

A) **2 件**: `ExampleNew`, `ExampleRootModule_NewModuleInstance` (推奨、Go-side public surface に限定)

B) **5 件**: A + Load / Configure / Stats / Journeys / RunJourney — 全 method

C) **JS example を doc.go の説明文に含む、`doc_test.go` の Example は 1 件** — k6 user は JS API が主、Go Example は最小

X) Other

[Answer]: A

---

### Question 11: Integration test 範囲

U5 の integration test:

A) **`k6otelgen/integration/` + `-tags=integration`** (推奨、U3/U4/U2 と同型) — 実 k6 binary で `--out otel-gen=...` 込み実行、Collector に signal が流れることを確認

B) **`modulestest.NewRuntime` で JS execution を simulate** — k6 binary なしで unit test 拡張、ただし `--out` flow を test できない

C) **U6 の integration test に統合** — U5 + U6 の E2E を U6 側で行う

X) Other

[Answer]: A

---

### Question 12: SemVer commitment

API stability 約束:

A) **JS API は post-v1 で SemVer 厳守** (推奨)
   - top-level methods (configure / load / stats / journeys) の signature
   - TopologyHandle method の signature
   - opts decode rules
   - Stats field names

B) **Go-side API はすべて internal** — k6 が呼ぶ surface (`New`, `NewModuleInstance`, `Exports`) のみ public、その他 lowercase

C) **Go-side + JS-side 両方 SemVer** — Go test や別の Go caller が直接呼ぶ可能性も想定

X) Other

[Answer]: A

---

### Question 13: 「これは N/A」の明示

U5 で扱わない NFR カテゴリ:

A) **N/A セクション** (U2/U3/U4 と同パターン):
   - Persistence (no DB / file storage; load reads file once but state in memory)
   - Authn/Authz
   - Encryption at rest / in transit (U4 が担保)
   - i18n / a11y (no UI)
   - GDPR/SOC2 (合成データのみ)
   - Production monitoring SLO
   - Disaster recovery
   - Backup / Restore

B) A + k6 cloud 統合関連 (将来検討)

C) N/A 列挙省略

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR-R アーティファクトを生成して承認ゲートへ進みます。
