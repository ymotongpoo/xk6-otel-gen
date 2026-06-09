# U3 (synth) — NFR Requirements Plan

## ユニットコンテキスト

- **Unit ID**: U3
- **パッケージ**: `synth/`
- **FD**: `aidlc-docs/construction/u3-synth/functional-design/{business-logic-model,business-rules,domain-entities}.md` (committed 16c915f)
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → **U3 (this — NFR-R)** → U2 → U5 → U6 → U8

## NFR-R で確定する事項

FD で「何をする」を確定済。NFR-R は **「何を達成するか」** を測定可能な数値・テスト可能な不変条件として確定する。中心テーマ:

- **Performance**: BeginSpan / RecordMetric / EmitLog / BuildResource の各 latency target、メモリ・アロケーション、スループット
- **Reliability**: nil provider / 不正 input の扱い、finishFunc 多重呼び出し保護
- **Observability**: synth 自身の self-metric (U4 の Stats のような) を持つか
- **API Stability**: SemVer commitment、public API surface の凍結範囲
- **Documentation**: GoDoc coverage、Example function 数
- **Testability**: Mock provider strategy、Coverage target、Integration test 範囲
- **Compatibility**: OTel SDK 版 / semconv v1.27.0 / Go 1.25 (U4 と整合)
- **Tech stack**: 採用依存と却下した代替案

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u3-synth/nfr-requirements/nfr-requirements.md` (NFR-U3-1〜N + N/A 列挙 + PBT 適合性 + DoD checklist)
- [ ] `aidlc-docs/construction/u3-synth/nfr-requirements/tech-stack-decisions.md` (Module 一覧 + version 戦略 + 代替案 rejected list)

---

## 設計確定のための質問

### Question 1: Per-call latency budget

`BeginSpan` / `RecordMetric` / `EmitLog` / `BuildResource` の latency target:

A) **FD §9 の目安をそのまま採用** (推奨):
   - `BeginSpan`: < 10 µs (p99)
   - `RecordMetric`: < 5 µs (p99)
   - `EmitLog`: < 10 µs (p99)
   - `BuildResource`: < 50 µs (UUID v5 計算込み)
   ベンチマークで `go test -bench` 計測、CI で regression detection

B) より厳しく (e.g., `BeginSpan < 5 µs`) — k6 高並列 (VU=1000+) でも目立たないように

C) より緩く (`BeginSpan < 50 µs` 等) — semconv attribute build のコストを considering

X) Other

[Answer]: A

---

### Question 2: Throughput target

k6 load test の典型ワークロードで持続可能な signal 生成 throughput:

A) **明示しない** — k6 自身のスループットに任せる、U3 は per-call latency だけ守る (推奨、k6 ワークロードに依存)

B) **数値で固定** — 例: 50k spans/sec, 100k metrics/sec, 50k logs/sec per VU (1 core)

C) **比率で指定** — synth は k6 が emit する http_req_total の 5x までは追従できる、等

X) Other

[Answer]: A

---

### Question 3: Self-metric (synth 自身の Stats)

U4 の `Stats` 構造体のような自己観測 metric を持つか:

A) **持たない** (推奨) — U3 は thin wrapper、Provider 経由で生成された span/metric/log は U4 の Stats で観測される。重複計装は冗長

B) **持つ**: `synth.Stats{SpansBegun, SpansFinished, MetricsRecorded, LogsEmitted}` の atomic counter — debug 用途

C) **OTel instrumentation の self-observability metric を有効化** (SDK 内部 metric の expose) — SDK バージョン依存

X) Other

[Answer]: A

---

### Question 4: Nil provider handling (NewDefault)

`NewDefault(tp, mp, lp)` で nil 引数が渡された時:

A) **panic** (推奨、FD §7.2 で既に提案) — programmer error として fail-fast、production code が nil を渡すバグを早期検出

B) **`(Synthesizer, error)` に戻り値変更** — graceful failure、ただし caller の boilerplate 増 (`if err != nil`)

C) **no-op synthesizer を返す** — silently ignore、debug 困難

X) Other

[Answer]: A

---

### Question 5: 不正 SpanInput / MetricInput / LogInput 入力

`BeginSpan` 等で不正入力 (e.g., `in.Service == nil`、`in.InstanceIdx < 0`、`in.InstanceIdx >= svc.Replicas`) の扱い:

A) **panic** (推奨、FD §2.3-2.5 で既に提案) — programmer error、Engine が責任を持つべき

B) **no-op で log warning** — production 耐性、ただし debug 困難 + log spam リスク

C) **戻り値 error 追加** — interface signature 変更、Engine が boilerplate 必要

X) Other

[Answer]: A

---

### Question 6: finishFunc 多重呼び出し保護

`FinishSpanFunc` を 2 回以上呼んだ時:

A) **no-op (silently ignore)** + race detection build で panic (推奨) — production は安全、test では bug 検出

B) **常に panic** — 強制的に bug 検出

C) **何もしない (responsibility on caller)** — 最もシンプル、SDK の `span.End` が冪等 (OTel 標準: 2 回目以降 silent ignore) なのでこれで十分かも

X) Other

[Answer]: A

---

### Question 7: Instrument の lazy vs eager 生成

`NewDefault` 内で 9 個の Histogram / UpDownCounter (4 namespace × {client,server} + active gauge) を作るタイミング:

A) **eager (NewDefault 内で全部作る)** (推奨) — 構築時に全 instrument 確保、Hot path で生成コスト無し。9 個程度ならメモリも誤差

B) **lazy (RecordMetric 初回で必要なものだけ作る)** — 起動時メモリ削減、ただし `sync.Once` 必要、初回呼び出し遅延

C) **sync.Map cache + Get-or-Create** — namespace 拡張時 (将来 `database/redis` 等) の柔軟性

X) Other

[Answer]: A

---

### Question 8: Mock provider strategy

Unit test での Provider mock:

A) **OTel SDK の test utility (`tracetest`, `metricdata.ResourceMetrics` 等) を使う** — 公式 in-memory exporter、SDK Provider をテスト構築 (推奨、SDK 標準)

B) **自前 mock interface** — OTel `trace.TracerProvider` 等を手作り mock 化

C) **U4 mockExporter を再利用** — U4 で既に作った mockSpanExporter 等を U3 のテストでも使う、ただし U4 のテストアセットを別 unit から import するアーキテクチャ問題あり

X) Other

[Answer]: A

---

### Question 9: Coverage target

`go test -cover ./synth/...` の目標:

A) **80% (U4 と同じ)** (推奨) — 業界標準、PBT で構造的カバレッジを補強

B) **85%+** — attribute mapping table は decision branch が多いので高めに

C) **75%** — interface-heavy で test しにくい部分を考慮

X) Other

[Answer]: A

---

### Question 10: Example function 数

`synth/doc_test.go` に書く Example の数:

A) **3 件**: `ExampleNewDefault`, `ExampleBuildResource`, `ExampleSynthesizer_BeginSpan` (推奨、主要 API のみ)

B) **5 件**: A + `ExampleSynthesizer_RecordMetric`, `ExampleSynthesizer_EmitLog` — 全 method

C) **1 件**: `ExampleNewDefault` のみ + doc.go の overview に統合

X) Other

[Answer]: A

---

### Question 11: Integration test 範囲

U3 の integration test (U4 と連動する end-to-end):

A) **`synth/integration/` ディレクトリ + `-tags=integration`** — U4 の Pipeline を実構築して synth 経由で signal を流し、Collector の出力 JSON で trace_id correlation 確認 (推奨、U4 と同パターン)

B) **U4 の `exporter/integration/` を拡張** — synth 側のテストを U4 integration test に併載

C) **Integration test なし、unit test (mock provider) のみ** — synth 単体機能の verify には不要との判断もありうる

X) Other

[Answer]: A

---

### Question 12: SemVer commitment

API stability 約束:

A) **post-v1 公開後 SemVer 厳守** (推奨) — 公開 API 変更は major bump、addition は minor、bug fix は patch

B) **v0 のうちは breaking change 自由** — Go 慣例 (v0 = experimental)

C) **post-v1 でも experimental tag 付き API は変更可** — 段階的 stability

X) Other

[Answer]: A

---

### Question 13: 「これは N/A」の明示

U3 では扱わない NFR カテゴリを明示:

A) **NFR-R アーティファクトで N/A セクション作成** (推奨、U4 と同パターン):
   - Persistence (no DB / file storage)
   - Authn/Authz (library, no auth boundary)
   - Encryption at rest / in transit (handled by U4 Pipeline)
   - i18n / a11y (Go library, no UI)
   - GDPR/SOC2 (no PII on synthetic data)
   - Production monitoring SLO (not a service)
   - Disaster recovery (no state)
   - Internationalization (no localized strings)

B) A + 他追加カテゴリ — security baseline 関連を厳密に

C) N/A 列挙省略 — 必要な NFR だけ書く

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR-R アーティファクト (`nfr-requirements.md`, `tech-stack-decisions.md`) を生成して承認ゲートへ進みます。
