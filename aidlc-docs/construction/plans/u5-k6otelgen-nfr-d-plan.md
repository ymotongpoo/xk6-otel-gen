# U5 (k6otelgen) — NFR Design Plan

## ユニットコンテキスト

- **Unit ID**: U5
- **パッケージ**: `k6otelgen/`
- **FD**: `aidlc-docs/construction/u5-k6otelgen/functional-design/` (committed 6c9743b)
- **NFR-R**: `aidlc-docs/construction/u5-k6otelgen/nfr-requirements/` (committed 237445b)
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → U2 ✓ → **U5 (this — NFR-D)** → U6 → U8

## NFR Design の焦点

FD で「何をする」、NFR-R で「何を達成するか」を確定済。NFR Design は **「どう実装するか」のパターン** を確定する:

- **RootModule struct field 配置** — sync.Once / sync.Mutex / err state
- **ModuleInstance / TopologyHandle の参照グラフ** — instance ↔ module ↔ handle
- **Exports() の dispatch パターン** — sobek FunctionCall wrapper、引数解析
- **sobek value 変換戦略** — JS object → Go `map[string]any` / Config / Stats
- **Per-VU random seed の物理戦略** — vu.VUID 取得 + seed 合成
- **JS exception 化のヘルパー** — Go error → sobek runtime exception
- **Configure / Load の sync.Once + state caching の正確な実装**
- **Pipeline lazy build の trigger location**
- **`--out` 警告の物理実装** — doc.go コメント / runtime warning log
- **Test helper の構造** — modulestest.NewRuntime + mock synth/engine
- **Integration test の xk6 build flow**
- **Anti-patterns**

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u5-k6otelgen/nfr-design/nfr-design-patterns.md`
- [ ] `aidlc-docs/construction/u5-k6otelgen/nfr-design/logical-components.md`

---

## 設計確定のための質問

### Question 1: RootModule struct layout

`*RootModule` 内 field の物理配置:

A) **3 group: load state + configure state + cached handle** (推奨):
```go
type RootModule struct {
    // Load state (sync.Once guarded)
    schemaOnce sync.Once
    schemaErr  error
    schema     *topology.Schema
    overlay    *topology.FaultOverlay
    loadedPath string

    // Configure state (sync.Once guarded)
    configureOnce sync.Once
    configureErr  error
    config        exporter.Config
    configured    bool

    // Cached handle
    handle *TopologyHandle
}
```

B) **1 nested struct で grouping**:
```go
type RootModule struct {
    loadState      loadStateImpl
    configureState configureStateImpl
}
```

C) **Field per concern を分離せず flat に並べる**

X) Other

[Answer]: A

---

### Question 2: ModuleInstance ↔ RootModule reference

`*ModuleInstance` から `*RootModule` への back-reference:

A) **直接 pointer field** (推奨、シンプル):
```go
type ModuleInstance struct {
    root *RootModule
    vu   modules.VU
    // ...
}
```

B) **interface 経由** — testability のため `rootProvider` interface

C) **VU context 経由で取得** — k6 SDK が module を context に詰めていれば

X) Other

[Answer]: A

---

### Question 3: Exports() dispatch pattern

JS から見える function を sobek にどう登録する?

A) **`Exports.Named` map に `func(call sobek.FunctionCall) sobek.Value` 形式の wrapper を直接** (推奨):
```go
func (i *ModuleInstance) Exports() modules.Exports {
    return modules.Exports{
        Named: map[string]any{
            "configure": i.jsConfigure,
            "load":      i.jsLoad,
            "stats":     i.jsStats,
            "journeys":  i.jsJourneys,
        },
    }
}

func (i *ModuleInstance) jsLoad(call sobek.FunctionCall) sobek.Value {
    path := call.Argument(0).String()
    handle, err := i.Load(path)
    if err != nil { panic(i.vu.Runtime().NewTypeError(err.Error())) }
    return i.vu.Runtime().ToValue(handle)
}
```

B) **sobek の `Define` で property を 1 個ずつ登録** — fine-grained だが冗長

C) **reflection ベースの auto-dispatch** — magic 多すぎ

X) Other

[Answer]: A

---

### Question 4: JS value 変換 — `Configure(opts)` の opts decode

JS object (`{endpoint: "...", insecure: true, ...}`) を Go side で受け取る方法:

A) **sobek の `Runtime.ExportTo(value, &target)` で reflection-based decode** (推奨):
```go
var raw map[string]any
err := i.vu.Runtime().ExportTo(call.Argument(0), &raw)
// then convert raw to exporter.Config field-by-field
```

B) **手動で sobek.Object methods を呼んで 1 field ずつ get** — type-safe だが冗長

C) **JSON.stringify → Go encoding/json decode** — round-trip コストあり

X) Other

[Answer]: A

---

### Question 5: opts → exporter.Config converter の実装

`map[string]any` (decode 済) → `exporter.Config` の変換ロジックの配置:

A) **`k6otelgen/config.go` 内に `optsToConfig(opts map[string]any) (exporter.Config, error)`** (推奨):
   - 各 known field を type assert で取り出し
   - timeout / batchTimeout は number (ms) と string ("10s") 両方対応
   - 不明 field は warn log + ignore
   - 型不一致は `*ConfigError{Kind: "type_mismatch", Field, Value}`

B) **`exporter.Config` 側に `(*Config).LoadFromMap(map)` method を持たせる** — U4 修正必要

C) **sobek decode の callback で直接 *exporter.Config に書く** — 一段省略

X) Other

[Answer]: A

---

### Question 6: Per-VU random seed

`journey.Engine` の random seed 構築方法 (per-VU 独立):

A) **`time.Now().UnixNano() ^ uint64(vu.State().VUID)` で seed** (推奨):
   - vu.State().VUID は k6 が割り振る一意 ID
   - XOR で混ぜると VU=0 でも非ゼロ seed
   - Engine 構築時 1 回計算、以降 lifetime 不変

B) **deterministic seed (`vu.State().VUID` のみ)** — test 再現性高、production も決定的 (反復実験向け)

C) **`crypto/rand` で 64bit seed** — full random、k6 startup ごとに変わる

X) Other

[Answer]: A

---

### Question 7: Pipeline lazy build trigger

`exporter.GetShared(factory)` をいつ呼ぶ?

A) **初回 `RunJourney` or `Stats` 呼び出し時、`*ModuleInstance` が遅延 build** (推奨):
```go
func (i *ModuleInstance) getOrBuildPipeline() (*exporter.Pipeline, error) {
    return exporter.GetShared(func() (*exporter.Pipeline, error) {
        return exporter.New(i.root.config)
    })
}
```
GetShared 自体が sync.Once、複数 VU が同時に呼んでも 1 回しか build しない

B) **`NewModuleInstance` 時に eager build** — VU 構築コストが上がるが startup 後の hot path は cache hit

C) **`Load` の中で build** — schema load と同時、setup() で 1 回完結

X) Other

[Answer]: A

---

### Question 8: Error → JS exception 化ヘルパー

Go error を JS exception に変換するヘルパー:

A) **`func throwJSException(rt *sobek.Runtime, err error)` を共通ヘルパー化** (推奨):
```go
func throwJSException(rt *sobek.Runtime, err error) {
    // Wrap by Go error type, format message with "[Kind]" prefix
    msg := formatErrorMessage(err)
    panic(rt.NewTypeError(msg))  // sobek convention: panic with NewTypeError
}
```
各 jsXxx wrapper で `defer func() { if r := recover(); r != nil { ... } }()` で受けて呼ぶ pattern

B) **error 型ごとに別 helper** — `throwConfigError` / `throwPlanError` 等

C) **Go error を sobek.Value (object) として return** — JS-side が is-error check 必要、convention 違反

X) Other

[Answer]: A

---

### Question 9: TopologyHandle の Go-side 表現

JS-side から見える `handle.runJourney(name)` の物理実装:

A) **`TopologyHandle` 型の Go method を sobek が `handle.runJourney` として expose** (推奨):
```go
type TopologyHandle struct {
    runtime *sobek.Runtime  // for throwing exceptions
    engine  *journey.Engine
    module  *RootModule
}

func (h *TopologyHandle) RunJourney(name string) {
    plan, err := h.engine.BuildPlan(name)
    if err != nil { throwJSException(h.runtime, err) }
    // ...
}
```
sobek は `*TopologyHandle` を JS object として wrap、`RunJourney` method を `runJourney` (lower-case first) として expose

B) **Plain Go method、JS から sobek 経由で reflection 呼び出し** — A と同じ実装

C) **handle を JS-side で完全に独立 object 化** (`Object.create`-like) — sobek 慣例から離れる

X) Other

[Answer]: A

---

### Question 10: `--out` warning の物理表現

`--out otel-gen=...` 未指定時の警告をどう表示?

A) **`doc.go` の package comment に warning + JS-side example** (推奨、receiver-side で work):
   - Go-side で runtime warning は出さない (k6 process が `--out` 状態を知らない可能性)
   - JS user は documentation を読むことを期待

B) **`load()` 時に sobek の `console.log("[k6otelgen] WARNING: ...")` で warn 出力** — JS-side で見える、ただし spammy

C) **`runJourney()` 初回呼び出し時に 1 回だけ warn log** — sync.Once で抑制、useful but invasive

X) Other

[Answer]: A

---

### Question 11: Test helper layout

`k6otelgen/*_test.go` の helper:

A) **`module_test.go` の冒頭 (or `helpers_test.go`) に集約** (推奨):
   - `newTestRuntime(t) *modulestest.Runtime`
   - `newTestRootModule(t) *RootModule`
   - `loadTestSchema(t, runtime, yaml)`
   - `mockSynth{}` (synth.Synthesizer interface を満たす test mock)

B) **各 test file で fresh setup を inline** — DRY 違反だが test 意図露わ

C) **外部 `internal/k6otelgentest` package** — 過剰

X) Other

[Answer]: A

---

### Question 12: Integration test の xk6 build flow

`k6otelgen/integration/integration_test.go` で実 k6 binary をどう用意?

A) **Integration test 内で `xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.` を `exec.Command` 実行 → 一時ディレクトリに k6 binary 生成 → `k6 run --out otel-gen=... script.js`** (推奨、self-contained)

B) **CI で事前に k6 binary を build しておき、test は path を環境変数で受ける** — シンプルだが local test 困難

C) **Docker compose に k6 service を含み、container 内で xk6 build** — 完全 sandbox

X) Other

[Answer]: A

---

### Question 13: ファイル分割の最終確認

FD §3 の 6 production + 4 test files (Q13=A):

A) **そのまま採用** (推奨):
   - `doc.go`, `module.go`, `instance.go`, `handle.go`, `config.go`, `errors.go`
   - tests: `module_test.go`, `instance_test.go`, `handle_test.go`, `config_test.go`
   + `helpers_test.go` (Q11=A)、`pbt_test.go` (TP-U5-1〜3)、`doc_test.go`、`bench_test.go`
   + `integration/integration_test.go`, `integration/helpers.go`, `integration/script.js`, `integration/topology.yaml`

B) **`errors.go` 省略** — `*ConfigError` 1 型のみ、`config.go` に同居

C) **`module.go` + `instance.go` 統合** — 小規模

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR Design アーティファクトを生成して承認ゲートへ進みます。
