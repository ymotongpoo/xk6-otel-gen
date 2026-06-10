# U2 (journey) — NFR Requirements Plan

## ユニットコンテキスト

- **Unit ID**: U2
- **パッケージ**: `journey/`
- **FD**: `aidlc-docs/construction/u2-journey/functional-design/` (committed b585d7e)
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → **U2 (this — NFR-R)** → U5 → U6 → U8

## NFR-R で確定する事項

FD で「何をする」を確定済。NFR-R は **「何を達成するか」** を測定可能・テスト可能な閾値として確定する。

中心テーマ:
- **Performance**: BuildPlan / Execute (per-step dispatch) の latency / メモリ、time.Sleep の影響を除いた pure overhead 計測
- **Reliability**: ctx cancel 時の挙動保証、panic recovery、Plan invalidation 条件
- **Concurrency**: 複数 VU 並列 Execute の race-free 保証、random source の strategy
- **Observability**: Engine 自身の self-metric (U3/U4 と同じく "持たない" 方針か)
- **API Stability**: SemVer commitment、Outcome / Plan / Node の互換性
- **Documentation**: Example function 数、GoDoc 範囲
- **Testability**: PBT runner 設定、Mock synth strategy、Coverage target
- **Compatibility**: Go 1.25、math/rand/v2 (or math/rand)、topology / synth 依存版
- **Tech stack**: 採用依存と却下した代替案

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u2-journey/nfr-requirements/nfr-requirements.md`
- [ ] `aidlc-docs/construction/u2-journey/nfr-requirements/tech-stack-decisions.md`

---

## 設計確定のための質問

### Question 1: Per-step dispatch overhead budget

`Execute` で 1 step あたりの **pure overhead** (synth call と time.Sleep を除いた処理) の target:

A) **< 50 µs / step** (推奨) — fault lookup + replica draw + outcome assembly のみで 50µs 以内。typical journey (5-15 steps) で total overhead < 1 ms

B) **< 100 µs / step** — 余裕を持たせる、複雑な fault 重ね合わせも吸収

C) **< 20 µs / step** — k6 高並列で目立たないように厳しく

X) Other

[Answer]: A

---

### Question 2: BuildPlan latency budget

`BuildPlan(name)` の所要時間:

A) **< 1 ms** (推奨) — typical journey (深さ ≤ 5, ops ≤ 20)、init phase で吸収する用途

B) **< 100 µs** — VU init phase でなく hot path で呼ばれる場合の余裕

C) **No target** — init で 1 回だけなので遅くて良い、Plan cache が前提

X) Other

[Answer]: A

---

### Question 3: Self-metric for Engine

Engine 自身の Stats (実行 journey 数、Cascade 回数、fault hit 回数等):

A) **持たない** (推奨、U3 と同じ方針) — U3/U4 の Stats で送信側を観測、Engine の内部状態は debug 時に test で確認

B) **debug 用 atomic counter** — `journey.Stats{PlansBuilt, JourneysExecuted, CascadesTriggered, FaultsApplied}`

C) **OTel 自己観測** — Engine 自身が "engine.journeys.executed" 等の metric を synth 経由で emit

X) Other

[Answer]: A

---

### Question 4: Random source の strategy

`*rand.Rand` (fault probability / replica idx) の管理:

A) **per-Engine 単一 source + sync.Mutex** (推奨、シンプル) — 全 VU が同じ Engine instance を共有、rand へのアクセスは mutex で直列化。throughput 制約あれば NFR-R で再評価

B) **per-VU sync.Pool of *rand.Rand** — VU ごと独立 seed、mutex 不要、ただし pool 管理コスト + seed 戦略

C) **`math/rand/v2` の `rand.IntN()` (global, race-free in v2)** — Go 1.22+ の rand/v2 は global API が thread-safe、mutex 不要だが deterministic seed が困難

X) Other

[Answer]: A

---

### Question 5: ctx cancel の保証

`Execute(ctx, plan)` が ctx.Done() を受けた時の保証:

A) **即時 (< 10ms) 中断** (推奨) — sleep 中の step は time.After + select で即時抜ける、entry な child は cascade で全てスキップ。残りの span は close される (finishFn 呼び出し保証)

B) **graceful (sleep 完了まで wait、その後中断)** — 現在 step は完了させてから止める、より predictable timing

C) **best-effort** — 明示的 guarantee なし、k6 の iteration timeout に任せる

X) Other

[Answer]: A

---

### Question 6: Panic recovery

Engine 内部または synth 呼び出しから panic が伝播した場合:

A) **defer recover で span を close、Outcome に "internal_error" を入れる** (推奨) — production 耐性、k6 iteration を crash させない

B) **そのまま panic 伝播** — Go 慣例、k6 側で recover を期待

C) **panic → `*ExecuteError{Kind: "internal", Inner: ...}` 戻り値** — recover して error として返す

X) Other

[Answer]: A

---

### Question 7: BuildPlan cache strategy

`Engine` の `plans map[string]*Plan` の初期化:

A) **NewEngine 内で全 journey の Plan を事前構築** (推奨) — init phase で 1 回、Execute は cache hit のみ。失敗があれば NewEngine が panic / error 化

B) **Lazy build on first BuildPlan call** — 必要な journey だけ build、memory 節約

C) **Plan を cache しない、毎 BuildPlan 呼び出しで build** — Plan immutable なら結果は同じだが overhead 重複

X) Other

[Answer]: A

---

### Question 8: Mock synth strategy for tests

`journey/*_test.go` での Synthesizer mock:

A) **自前 mock struct** in `helpers_test.go` (推奨) — synth.Synthesizer interface を簡単に満たす test-local impl、span/metric/log 呼び出しを記録、assertion で確認

B) **OTel SDK tracetest を経由した Synthesizer** — U3 NewDefault で構築した本物 Synthesizer を使い、SDK の in-memory exporter で確認

C) **U3 のテストヘルパーを再利用** — U3 helpers_test.go を package 共有化 (アーキテクチャ違反)

X) Other

[Answer]: A

---

### Question 9: Coverage target

`go test -cover ./journey/...`:

A) **80%** (推奨、U3/U4 と同じ)

B) **85%+** — branch-heavy なロジック (cascade / fault / recovery) を保証

C) **75%** — context cancel / panic recovery 等の corner case が test しにくい

X) Other

[Answer]: A

---

### Question 10: Example function 数

`journey/doc_test.go`:

A) **3 件**: `ExampleNewEngine`, `ExampleEngine_BuildPlan`, `ExampleEngine_Execute` (推奨、主要 API)

B) **5 件**: A + `ExampleEngine_ListJourneys`, `ExamplePlan` — 全 method

C) **1 件**: `ExampleNewEngine` のみ + doc.go の overview に統合

X) Other

[Answer]: A

---

### Question 11: Integration test 範囲

U2 の integration test:

A) **`journey/integration/` + `-tags=integration`** (推奨、U3/U4 と同型) — 実 topology YAML + 実 U4 Pipeline で end-to-end、Collector に trace_id 相関 + cascade パターンを確認

B) **U3 の integration test を拡張** — synth 経由なので U3 の harness に journey の test を merge

C) **Integration test なし、unit test (mock synth) のみ** — synth-driven なので E2E は U3 で十分との判断

X) Other

[Answer]: A

---

### Question 12: SemVer commitment

API stability 約束:

A) **post-v1 公開後 SemVer 厳守** (推奨、U3/U4 と同じ)

B) **v0 のうちは breaking change 自由**

C) **Outcome 構造体 field を後から add するのは minor で OK と明示** — Outcome は post-v1 でも extensible にしておく

X) Other

[Answer]: A

---

### Question 13: 「これは N/A」の明示

U2 で扱わない NFR カテゴリ:

A) **N/A セクションを書く** (U3/U4 と同じパターン):
   - Persistence
   - Authn/Authz
   - Encryption at rest / in transit
   - i18n / a11y
   - GDPR/SOC2 (合成データのみ)
   - Production monitoring SLO
   - Disaster recovery
   - Multi-region
   - Backup / Restore

B) A + 他追加

C) N/A 列挙省略

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR-R アーティファクトを生成して承認ゲートへ進みます。
