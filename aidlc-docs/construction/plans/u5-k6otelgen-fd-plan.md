# U5 (k6otelgen) — Functional Design Plan

## ユニットコンテキスト

- **Unit ID**: U5
- **パッケージ**: `k6otelgen/`
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → U2 ✓ → **U5 (this)** → U6 → U8
- **Purpose** (Application Design より):
  - k6 JS Module SDK 経由で `k6/x/otel-gen` を登録
  - JS 側 API: `load(path)`, `configure(opts)`, `topology.runJourney(name)`
  - **Process singleton** の Topology / FaultOverlay / Pipeline と **per-VU** Engine / Synthesizer の組み立て
- **Upstream artifacts**:
  - `aidlc-docs/inception/application-design/component-methods.md` §C5 (RootModule / ModuleInstance / TopologyHandle)
  - U1 `topology` (Parse / Schema / ApplyFaults), U3 `synth` (NewDefault / Synthesizer), U2 `journey` (NewEngine / BuildPlan / Execute), U4 `exporter` (GetShared / Pipeline)
- **k6 SDK 依存**: `go.k6.io/k6/js/modules` (RootModule, ModuleInstance, modules.VU, Exports)、`github.com/grafana/sobek` (JS value 操作 — k6 v0.x recent では sobek を採用)

## FD で確定すべき事項

FD は「**何をする / どんなドメインルールに従う**」を確定する。U5 FD で扱う事項:

- **`k6/x/otel-gen` の JS API surface** — JS 側で見えるメソッド・型・配置
- **Singleton vs per-VU 状態の境界** — Topology / FaultOverlay / Pipeline は process 単位、Engine / Synthesizer は VU 単位
- **`Load(path)` の semantics** — YAML 読み込み + parse + Validate + ApplyFaults
- **`Configure(opts)` の semantics** — exporter.Config の優先順位 merge (JS API > env > built-in)
- **`TopologyHandle.RunJourney(name)` の semantics** — Engine.Execute をどの ctx で呼ぶか、JS から見たエラー扱い
- **Lifecycle**: k6 init → setup → per-VU iteration → teardown のどこで何を初期化/破棄するか
- **エラー扱い**: panic vs JS exception vs return value
- **Concurrency**: per-VU state の race-free 保証
- **Stats / Journeys() の semantics** — Pipeline stats を JS 側で読む
- **PBT properties** — JS surface に対する property は限定的、unit test 中心
- **U7 への generator 追加リクエスト** — JS-side input の generator

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u5-k6otelgen/functional-design/business-logic-model.md`
- [ ] `aidlc-docs/construction/u5-k6otelgen/functional-design/business-rules.md`
- [ ] `aidlc-docs/construction/u5-k6otelgen/functional-design/domain-entities.md`

---

## 設計確定のための質問

### Question 1: JS API surface

k6 script から見える top-level API:

A) **`require('k6/x/otel-gen')` で:** (推奨)
```javascript
import otelgen from "k6/x/otel-gen";

export function setup() {
    otelgen.configure({ endpoint: "https://...", protocol: "grpc", insecure: false });
    const topology = otelgen.load("./topology.yaml");
    return { topology };  // ← returned to default function
}

export default function (data) {
    data.topology.runJourney("checkout-flow");
}

export function teardown(data) {
    // exporter.Pipeline.Shutdown() called automatically by k6 lifecycle (U6)
}
```
top-level: `load`, `configure`, `stats`, `journeys`。TopologyHandle 上: `runJourney`, `journeys`

B) **すべて TopologyHandle 中心**: `otelgen.load(path)` のみが top-level、その他は handle method
   ```javascript
   const t = otelgen.load("./topology.yaml");
   t.configure({...});
   t.runJourney("checkout");
   ```

C) **Function-based (no handle)**: `otelgen.runJourney(name, opts?)` — singleton state 隠蔽
   ```javascript
   otelgen.load("./topology.yaml");
   otelgen.runJourney("checkout");
   ```

X) Other

[Answer]: A

---

### Question 2: Singleton state の保持

Topology / FaultOverlay / Pipeline は process singleton。物理的にどこに?

A) **`RootModule` 構造体内の field + sync.Once** (推奨、k6 SDK 慣例):
   - `RootModule` は k6 init phase で 1 回構築
   - load() を呼んだら schema/overlay/pipeline を `RootModule` に格納、sync.Once で 2 回目以降は no-op (or "already loaded" error)
   - configure() も同様、最初の呼び出しで config 固定

B) **Package-level vars** — global state、テスト困難

C) **k6 SDK の Process context (modules.VU 経由で取得?)** — k6 SDK が process state を提供する場合採用

X) Other

[Answer]: A

---

### Question 3: Per-VU state (Engine + Synthesizer)

`Engine` と `Synthesizer` は per-VU か process-shared か?

A) **per-VU** (推奨、Application Design 通り):
   - Engine は random source を持つ → per-VU instance で seed 独立
   - Synthesizer は thread-safe だが per-VU でも問題なし
   - `ModuleInstance` の field に格納、`NewModuleInstance(vu)` で構築
   - VU iteration ごとに iteration scope なし (Engine は VU lifetime で保持)

B) **process-shared** — race-free だが random source 共有
   - Engine 1 個、Synthesizer 1 個を RootModule に
   - 全 VU が同じ Engine.Execute を並行呼ぶ
   - U2 で per-Engine mutex 保護済なので race-free

C) **lazy build per-VU** — 最初の RunJourney 呼び出しで構築

X) Other

[Answer]: A

---

### Question 4: Load(path) の semantics

`otelgen.load(path)` の動作:

A) **idempotent + cache** (推奨):
   - 同じ path → 同じ TopologyHandle (RootModule 内 cache)
   - 異なる path → ConfigError "topology already loaded" (singleton 縛り)
   - Schema は `topology.Parse(yaml)` + `Validate()` + `ApplyFaults()` を実行
   - 失敗時 JS 側で throw (error 戻り値で JS exception 化)

B) **複数 Topology 許容** — path ごとに別 handle、複数 journey が並走可

C) **Reload 許容** — 同 path でも再 parse、上書き

X) Other

[Answer]: A

---

### Question 5: Configure(opts) の semantics

`otelgen.configure(opts)` の優先順位:

A) **JS API > env > built-in defaults** (推奨、U4 FD §10 で確認済優先順位)
   - `Configure({endpoint: "..."})` を呼ぶと exporter.Config を build、`exporter.GetShared(factory)` の factory で merge
   - 複数回呼び出しは "already configured" でエラー (singleton 縛り)
   - Configure せずに RunJourney しても OK (env のみで動く)

B) **Configure 必須** — 明示的に config を要求

C) **複数回 OK、merge** — 後から呼ぶと上書き

X) Other

[Answer]: A

---

### Question 6: RunJourney の ctx と error 扱い

`handle.runJourney(name)` の内部:

A) **per-VU iteration の ctx を Engine.Execute に渡す** (推奨)
   - k6 VU が iteration 用 ctx を `modules.VU.Context()` 経由で提供
   - Engine.Execute が ctx.Done() を尊重 (NFR-U2-4 で 10ms 以内中断保証済)
   - Engine.Execute が error 返したら JS exception として throw

B) **Background ctx + timeout** — k6 iteration の deadline と独立に動く

C) **goroutine 起動 + 非同期完了** — JS-side では Promise-like、ただし k6 JS は async/await が制約あり

X) Other

[Answer]: A

---

### Question 7: Stats() の意味

`otelgen.stats()` JS API:

A) **`exporter.Pipeline.Stats()` をそのまま JS object 化** (推奨)
   - `{TracesExported, TracesFailed, MetricsExported, MetricsFailed, LogsExported, LogsFailed}` を return
   - read-only snapshot
   - JS 側で `console.log(otelgen.stats())` で debug

B) **stats なし、JS 側に観測 API を提供しない** — Pipeline observability は外部 (Collector メトリクス) に任せる

C) **A + journey 実行統計 (RunJourney 呼び出し回数等)** — k6 native counter で十分なので冗長

X) Other

[Answer]: A

---

### Question 8: Pipeline Shutdown のトリガ

`exporter.Pipeline.Shutdown(ctx)` を呼ぶ責務:

A) **U6 (k6output) の `Output.Stop()` で呼ぶ** (推奨、Application Design 通り):
   - U5 は Pipeline 構築のみ、shutdown は U6 / k6 lifecycle に委譲
   - k6 が `--out otel-gen=...` 経由で U6 を有効化していない場合、Shutdown は呼ばれない可能性 → integration test で `--out` 使用前提を明記

B) **U5 で `teardown()` JS-side で明示呼び出し** — k6 JS の teardown 関数で `otelgen.shutdown()` を JS から呼ぶ

C) **U5 内で `runtime.SetFinalizer`** — GC 待ち、非決定的

X) Other

[Answer]: A

---

### Question 9: Error の JS 表現

Go side エラー → JS side 表現:

A) **`*ConfigError` / `*PipelineError` / `*PlanError` / `*ExecuteError` を JS Error として throw** (推奨)
   - sobek の `runtime.NewTypeError` 等で JS Error 化
   - error.message に Go error.Error() 文字列
   - 種別 (Kind) は JS exception name / property に

B) **すべて単一 generic Error** — シンプルだが種別判別不可

C) **JS object として return** — exception 化せず caller がチェック

X) Other

[Answer]: A

---

### Question 10: テスタビリティ — k6 SDK の mock

`k6otelgen` の unit test で k6 SDK (modules.VU, sobek.Runtime) をどう mock?

A) **k6 SDK の test utility (`modulestest.NewRuntime`) を使う** (推奨、k6 公式)
   - `modulestest.NewRuntime(t)` で sobek + modules.VU を組み立て可能
   - test 内で実際の JS execution を simulate

B) **自前 mock interface** — modules.VU を独自定義

C) **Integration test 中心 (実 k6 binary を build して動かす)** — slow、初期実装向け不要

X) Other

[Answer]: A

---

### Question 11: PBT properties

U5 で扱う property:

A) **3 properties** (推奨、JS-side surface は test しにくいので限定):
   - TP-U5-1: Load idempotency — 同じ path で同じ handle を返す
   - TP-U5-2: Configure merge — env + JS opts の merge 結果が U4 の MergeWith 結果と一致
   - TP-U5-3: RunJourney は Engine.Execute を 1 度だけ呼び、ctx を正しく渡す

B) A + JS exception の Kind 判別 PBT — Go error 各種が正しく JS error に mapping

C) PBT minimal (1 個のみ) — JS layer は example-based test で十分

X) Other

[Answer]: A

---

### Question 12: U7 への generator 追加リクエスト

PBT 用に U7 に追加する generator:

A) **2 generator pairs** (推奨):
   - `ValidConfigureOpts` / `AnyConfigureOpts` — JS opts (`map[string]any` を `exporter.Config` に逆変換できる形式) の generator
   - `ValidLoadPath` / `AnyLoadPath` — file path string (relative / absolute / with traversal)
   合計 4 関数

B) A + `ValidJSValue` — sobek.Value の generator

C) 不要 — JS-side test は example-based のみ

X) Other

[Answer]: A

---

### Question 13: ファイル分割

`k6otelgen/` のファイル構成:

A) **6 production + 4 test files** (推奨):
   - `doc.go`
   - `module.go` — RootModule + NewModuleInstance + modules.Register init()
   - `instance.go` — ModuleInstance + Exports() の構築
   - `handle.go` — TopologyHandle + RunJourney + Journeys
   - `config.go` — configure() の opts → exporter.Config 変換
   - `errors.go` — JS-friendly error wrapping
   - tests: `module_test.go`, `instance_test.go`, `handle_test.go`, `config_test.go`

B) **3 production**: `module.go` (RootModule + Instance + Handle), `config.go`, `doc.go` — 小さくまとめる

C) **JS-feature 別**: `load.go`, `configure.go`, `runjourney.go`, `stats.go` 等

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 3 つの FD アーティファクト (business-logic-model / business-rules / domain-entities) を生成して承認ゲートへ進みます。
