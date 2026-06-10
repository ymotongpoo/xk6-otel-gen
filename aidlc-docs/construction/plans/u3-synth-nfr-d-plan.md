# U3 (synth) — NFR Design Plan

## ユニットコンテキスト

- **Unit ID**: U3
- **パッケージ**: `synth/`
- **FD**: `aidlc-docs/construction/u3-synth/functional-design/` (committed 16c915f)
- **NFR-R**: `aidlc-docs/construction/u3-synth/nfr-requirements/` (committed 4895375)
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → **U3 (this — NFR-D)** → U2 → U5 → U6 → U8

## NFR Design の焦点

FD で「何をする」、NFR-R で「何を達成するか」を確定済。NFR Design は **「どう実装するか」のパターン** を確定する:

- **Instrument 構造化パターン** — 9 個の Histogram/UDC をどう物理配置 (map vs struct field)
- **Attribute build と allocation 戦略** — Hot path 上のアロケーション削減
- **Resource cache 戦略** — 同じ (svc, idx) で再計算? cache?
- **UUID v5 namespace 値の確定**
- **finishFunc 内部実装** — closure / struct / channel
- **active_requests の更新パターン** — BeginSpan / finishFunc の責務切り分け
- **Panic message のフォーマット標準**
- **Mock provider helper の構造**
- **Histogram bucket 設定** — SDK default vs explicit
- **Log Body 自動生成テンプレートの確定**
- **`process.runtime.name=svc.Language` の最終決定** (NFR-R Open Questions より)
- **Integration test directory/helper の構造**
- **ファイル分割と内部 helper の粒度** (FD §3 確認)

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u3-synth/nfr-design/nfr-design-patterns.md` (Performance / Concurrency / Error / API / Documentation / Test 各パターン群)
- [ ] `aidlc-docs/construction/u3-synth/nfr-design/logical-components.md` (`synth/` 内 LC-0..LC-N の責務 / 公開 API / 実装スケッチ)

---

## 設計確定のための質問

### Question 1: 9 Instrument の物理配置

`defaultSynthesizer` 構造体内の 9 個の Histogram/UDC をどう配置?

A) **named fields** (推奨、Q7=A eager に合う):
```go
type defaultSynthesizer struct {
    httpClientDur  metric.Float64Histogram
    httpServerDur  metric.Float64Histogram
    httpActiveReq  metric.Int64UpDownCounter
    rpcClientDur   metric.Float64Histogram
    rpcServerDur   metric.Float64Histogram
    rpcActiveReq   metric.Int64UpDownCounter
    dbClientDur    metric.Float64Histogram
    msgProducerDur metric.Float64Histogram
    msgConsumerDur metric.Float64Histogram
    // ... + tracer / logger
}
```
明示的、零コスト dispatch。

B) **map by namespace+kind**: `map[string]metric.Float64Histogram` — 動的、namespace 追加が容易。ただし lookup コスト + race-free 必須

C) **substruct grouping**: `httpInstruments struct{ ClientDur, ServerDur, Active }` × 4 — 構造化されているが冗長

X) Other

[Answer]: A

---

### Question 2: Attribute build / allocation 戦略

`BeginSpan` / `RecordMetric` での `[]attribute.KeyValue` 構築:

A) **per-call `attribute.KeyValue` slice、`attribute.NewSet` で SDK 側 cache 期待** (推奨) — SDK が内部で intern する可能性に賭ける、コードシンプル

B) **`sync.Pool[*[]attribute.KeyValue]` で slice 再利用** — GC 圧削減、ただし pool 操作のコスト + lifetime 管理リスク

C) **事前 build した `attribute.Set` を Service+Edge ごとに cache** — 同じ (svc, op, edge) では再利用、ただし cache 無効化条件が増える

X) Other

[Answer]: A - しかし intern する可能性に賭けるってどういうこと？確実にやってくれないと困るんだけど

---

### Question 3: Resource cache 戦略

`BuildResource(svc, idx)` を多数回呼ぶワークロード (k6 1000 VU × many ops):

A) **No cache (毎回新規構築)** — UUID v5 計算 + attribute slice 構築のみ、< 50 µs/call なら許容 (推奨、最小スコープ)

B) **`sync.Map[(svc.Name, idx)]*resource.Resource` で cache** — 第 2 回以降はゼロコスト、ただし lifecycle 管理 (svc が再定義された場合の invalidate) が必要

C) **caller (Engine) 側で cache、synth は不変関数を提供のみ** — 責務切り分け、Engine で per-VU cache

X) Other

[Answer]: A

---

### Question 4: UUID v5 namespace 固定値

`service.instance.id` 用の UUID v5 namespace UUID:

A) **`uuid.NewSHA1(uuid.NameSpaceDNS, []byte("xk6-otel-gen/synth"))` の結果を固定文字列としてコードに埋め込む** — 1 回計算して値を pin (推奨、決定論性 + 明示)

B) **DNS namespace を直接使う**: `uuid.NameSpaceDNS` で `synth.Name + "/" + strconv.Itoa(idx)` を name に — 簡素、衝突リスクは無視できるレベル

C) **Application-specific UUID を新規発行 (`uuid.New()` 1 回だけ手動、結果を pin)** — 完全 isolation

X) Other

[Answer]: A

---

### Question 5: finishFunc 内部実装

`FinishSpanFunc` の closure 中身:

A) **closure が `span trace.Span` と `*atomic.Bool` を capture** (推奨、Q6=A の double-call protection 用):
```go
finishOnce := atomic.Bool{}
return func(outcome Outcome) {
    if !finishOnce.CompareAndSwap(false, true) {
        // race detection build: panic; otherwise: silent ignore
        if raceEnabled { panic("FinishSpanFunc called twice") }
        return
    }
    // SetStatus / SetAttributes / End
}
```

B) **`sync.Once`-style** — 重い、closure 内で十分

C) **何もしない (caller 責任)** — race build でも panic させない、Q6=A と矛盾

X) Other

[Answer]: A

---

### Question 6: active_requests の更新責務

`*.server.active_requests` UpDownCounter の +1 / -1 をどこで?

A) **BeginSpan で +1、finishFunc 内で -1** (推奨) — span lifecycle と完全同期、forget しにくい

B) **RecordMetric で別途 active=true/false フラグを渡し、Engine 側が責任** — Synthesizer 外から制御、誤って unbalance しうる

C) **Engine が start/end 別の API を呼ぶ** (`MarkActive(in)` / `MarkInactive(in)`) — API surface 増大、複雑

X) Other

[Answer]: A

---

### Question 7: Panic message フォーマット

Programmer error の panic メッセージ標準:

A) **`fmt.Sprintf("synth: <method>: <field> = %v: <reason>", ...)`** (推奨、grep しやすい):
   - `panic("synth: NewDefault: tp must not be nil")`
   - `panic(fmt.Sprintf("synth: BeginSpan: InstanceIdx %d out of range [0, %d)", in.InstanceIdx, svc.Replicas))`

B) **error 型 (`*PanicReason{Method, Field, Reason}`) を panic** — recover 側で type assertion 容易

C) **error.New + panic — シンプルだが grep しにくい**

X) Other

[Answer]: A

---

### Question 8: Mock provider helper の構造

`synth/*_test.go` で使う test helper:

A) **`synth/internal_helpers_test.go` (or `helpers_test.go`)** に集約 (推奨):
```go
// Test helper: in-memory provider construction
func newTestProviders(t *testing.T) (
    trace.TracerProvider,
    metric.MeterProvider,
    log.LoggerProvider,
    *tracetest.InMemoryExporter,
    *sdkmetric.ManualReader,
    LogRecorder,
)
```
全 test ファイルから利用。

B) **各 test ファイルで重複定義** — independent だが DRY 違反

C) **外部 helper package (`synth/testsupport/`)** — 過剰

X) Other

[Answer]: A

---

### Question 9: Histogram bucket

`{namespace}.client.request.duration` / `.server.request.duration` の bucket:

A) **SDK default bucket boundaries を使う** (推奨) — explicit に指定せず、SDK が決める。NFR-R で「将来 bench 計測後に再評価」と明記済

B) **OTel semantic conventions の HTTP server duration 推奨 bucket** (e.g. `[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]` 秒) を explicit に — SDK 既定と同じ場合もあるが明示的

C) **k6 ワークロード fit ベース** — bench 後に確定、本 unit ではプレースホルダ

X) Other

[Answer]: A

---

### Question 10: Log Body 自動生成テンプレート

`EmitLog` で `LogInput.Body == ""` の場合の自動生成:

A) **success**: `"{Service.Name}.{Operation} succeeded"` / **failure**: `"{Service.Name}.{Operation} failed: {ErrorType}"` (推奨、`business-rules.md` §5.3 で既に提案)

B) **常に caller が Body を明示**、自動生成しない (空 Body は許容しない)

C) **JSON シリアライズで全 fields を Body に**

X) Other

[Answer]: A

---

### Question 11: `process.runtime.name` semantic の確定

NFR-R Open Question 「`process.runtime.name=svc.Language` の semantic 妥当性」:

A) **そのまま `process.runtime.name=svc.Language` で確定** (推奨、`business-logic-model.md` §3 で提案済) — semconv は本来 SDK が走る runtime を指すが、合成 service にとっては「シミュレートされる runtime」として再解釈

B) **`synth.simulated.runtime` という custom namespace で安全に置く** — semantic 衝突回避

C) **完全削除、Service.Language は attribute に含めない** — Q4=B (NFR-R) と矛盾するが Hardcoded "go" のみで運用

X) Other

[Answer]: A

---

### Question 12: Integration test directory layout

`synth/integration/` の構造:

A) **U4 と同型** (推奨):
```text
synth/integration/
├── docker-compose.yaml
├── collector-config.yaml
├── helpers.go         # StartCollector, ReadXxx 系
└── integration_test.go (//go:build integration)
```
U4 の `exporter/integration/` を参考にしつつ、Pipeline は U4 から構築。

B) **U4 の `exporter/integration/` 内に併載** — 別ディレクトリにしない

C) **`tests/integration/synth_test.go`** — project ルート直下

X) Other

[Answer]: A

---

### Question 13: ファイル分割の最終確認

FD §3 提案の `synth/` レイアウト (6 production + 5 test):

A) **そのまま採用** (推奨):
   - `doc.go`, `interface.go`, `synthesizer.go`, `resource.go`, `attributes.go`, `errors.go`
   - tests: `interface_test.go`, `synthesizer_test.go`, `resource_test.go`, `attributes_test.go`, `pbt_test.go`

B) **`errors.go` 省略** — synth-specific error 型を持たない (panic only) ので不要

C) **`synthesizer.go` を `synthesizer.go` (struct + NewDefault) + `synthesizer_span.go` (BeginSpan + finishFunc) + `synthesizer_metric.go` + `synthesizer_log.go` に分割** — file あたり責務が狭い

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR Design アーティファクトを生成して承認ゲートへ進みます。
