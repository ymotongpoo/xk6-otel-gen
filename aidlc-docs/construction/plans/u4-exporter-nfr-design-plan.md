# U4 (exporter) — NFR Design Plan

## ユニットコンテキスト

- **Unit ID**: U4
- **パッケージ**: `exporter/`
- **FD**: `aidlc-docs/construction/u4-exporter/functional-design/`
- **NFR-R**: `aidlc-docs/construction/u4-exporter/nfr-requirements/` (NFR-U4-1〜12)

## NFR Design の焦点

FD で「何をする」、NFR-R で「何を達成する」を確定済み。NFR Design は **「どう実装するか」のパターン** を確定する。中心となる事項:

- **Config / Pipeline / Stats の物理的実装パターン** — exported struct vs builder、atomic vs mutex の最終決定 (NFR-R は方針、ここで具体)
- **Instrumented exporter wrapper** — Stats を更新する OTel SDK Exporter のラッパー (3 信号 × 2 protocol = 6 種)
- **Mock exporter** for unit test — real Collector を起動せずに送信パスをテストする方法
- **Integration test harness** — Docker compose 起動と Collector 出力 JSON 読み取りの自動化
- **shared.go の初期化パターンとテスト分離** — `sync.Once` の test 間リセット
- **bench fixture** — `BenchmarkNew` の Config 入力をどう与えるか

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u4-exporter/nfr-design/nfr-design-patterns.md` — Performance / Concurrency / Error / API / Documentation / Test の各パターン群
- [ ] `aidlc-docs/construction/u4-exporter/nfr-design/logical-components.md` — `exporter/` 内の論理コンポーネント (LC-0..LC-N) とそれぞれの責務・公開 API・実装スケッチ

---

## 設計確定のための質問

### Question 1: Instrumented Exporter Wrapper の実装スタイル

Stats 更新のため、OTel SDK の Exporter interface (`ExportSpans` / `Export` / `ExportLog`) をラップする。実装スタイル:

A) **3 信号 × 1 wrapper = 3 wrapper 型** (`tracingExporter`, `metricExporter`, `loggingExporter`)。各 wrapper が `*pipelineStats` への参照と `inner SpanExporter`/`Reader`/`Processor` を持つ (推奨、3 信号の型が異なるため自然)

B) **interface ベースの汎用 wrapper** — `type instrumentedExporter[T any] struct { inner T; ... }`。Generics で抽象化、コード重複削減

C) **decorator なし、Stats は別途 monitoring channel で更新** — Exporter は SDK 標準のまま、別 goroutine が定期的に Stats を観測

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 2: Stats 更新タイミングの詳細

`*Exported` カウンタ更新は具体的にどのタイミング?

A) **`Export(ctx, batch)` が `nil` を返した時点で `len(batch)` 加算、`!= nil` なら `*Failed` を 1 加算** — 1 batch = 1 success/fail カウント、Exported counter は item 数 (推奨)

B) **batch サイズに関係なく成功時 1 加算、失敗時 1 加算** — シンプル、ただし `*Exported` はバッチ数の意味に変わる

C) A + retry を考慮 — OTel SDK が内部 retry した場合の挙動も追跡

X) Other (please describe after [Answer]: tag below)

[Answer]: A 

---

### Question 3: QueueLen の取得方法

`Stats.TracesQueueLen` 等のキュー長は OTel SDK の内部値だが、SDK が直接露出する公式 API がない可能性が高い。

A) **本 unit では `QueueLen = 0` 固定 (取得しない)** — SDK API が無いので諦め、`Stats` の 3 フィールドは将来再評価 (推奨、最小スコープ、Stats の他 6 フィールドは取れる)

B) **wrapper で並列カウンタを持つ** — `Export` 呼び出し時点での item 数を `QueueLen` として一時的に保持 (実態とは少しズレるが目安値)

C) **OTel SDK の内部 metric を読む** — SDK が `metrics_dropped_total` 等を露出していれば取り込む (SDK バージョン依存)

X) Other (please describe after [Answer]: tag below)

[Answer]: 質問では「SDKが直接露出する公式APIがない可能性が高い」と言っているが、これは推測なので確認してから決定する必要がある。APIがあればQueueLenは取得する。APIがなければ現時点でQueueLenをStatsで取る必要はなく、仕様にも加える必要はない。TODOとして、将来の可能性として記載しておけばよい。「将来の互換性のため」という理由でGo構造体にフィールドを残す必要もない。

---

### Question 4: Mock Exporter for Unit Test

`exporter/pipeline_test.go` などで `New(Config)` を呼ぶと実 Collector が必要になる。Unit test での回避策:

A) **テスト専用 `mockExporter` を `exporter_test` パッケージ内に定義** — SDK Exporter interface を満たし、in-memory に受信内容を保存。`SetShared(buildMockPipeline())` でテスト時に差し替え (推奨)

B) **`exporter.NewWithFactory(cfg, factory)` のような hook を公開** — production code に test 用 hook を入れる (API 汚染リスク)

C) **Real Collector を unit test でも起動 (Docker)** — Integration test との境界が曖昧

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 5: Integration Test の Collector 出力読み取り

Collector が `/var/log/otel/*.json` に書き出す JSON を test 内で読む方法:

A) **テスト host のディスク (volume mount) を直接 `os.ReadFile`** — 最もシンプル、CI / 開発者ローカル共通 (推奨)

B) **Collector の OTLP/HTTP receiver を別 port で立て、test が中身を取りに行く** — file_exporter なし、メモリ完結

C) **OTLP/gRPC server を test 内で立てて Collector の代わりに受信** — Collector の起動なし

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 6: Shared holder の test 分離

`exporter/shared.go` は package-level `sync.Once` を持つ。test 間で初期化状態をリセットするには?

A) **`ResetShared()` を export し、各 test の冒頭で呼ぶ** — explicit、テスト間の独立性確保 (推奨)

B) **`testing.T.Cleanup` でリセットを自動登録するヘルパー** — `shared_testhelp.go` で `ResetSharedForTest(t *testing.T)` を公開、t.Cleanup で復元

C) **テストは別パッケージ (`exporter_test`) に置き、リセット不要なケースだけ書く** — 表面的だがテストカバレッジが下がる

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 7: Config の builder vs struct literal

Config 構築のスタイル:

A) **plain struct literal を主流に** — `cfg := exporter.Config{Endpoint: "...", Timeout: 5*time.Second, ...}`。Go の慣例的、シンプル (推奨)

B) **functional options + `NewConfig(opts ...Option)`** — `exporter.NewConfig(exporter.WithEndpoint("..."), ...)`。U7 generator と整合

C) **両方サポート** — struct literal でも options でも作れる

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 8: BenchmarkNew の入力

`BenchmarkNew` の Config 入力をどう与えるか?

A) **fixed Config in test code** — テスト先頭で `var benchConfig = Config{...}` を定義、毎 iteration 同じものを使う (推奨)

B) **`generators.ValidConfig()` から draw** — rapid を bench で使う (Code generation 時の overhead が bias)

C) **3 種類のサブ bench** — `BenchmarkNew/grpc_typical`, `BenchmarkNew/http_typical`, `BenchmarkNew/grpc_large_headers`

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 9: Pipeline インスタンスの内部表現

`Pipeline` 構造体の内部フィールド管理:

A) **直接 SDK Provider 型を保持** — `Pipeline{tp *sdktrace.TracerProvider, mp *sdkmetric.MeterProvider, ...}`。シンプル (推奨)

B) **interface ベース** — `Pipeline{tp trace.TracerProvider, mp metric.MeterProvider, ...}`。テストで mock を差し込みやすいが、SDK の独自メソッド (Shutdown 等) にアクセスする際に type assertion 必要

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 10: SDK exporter cleanup on partial failure

NFR-U4 / FD で「all-or-nothing、partial failure 時は cleanup」と決めたが、cleanup 中の error をどう扱う?

A) **cleanup error は捨てる (log なし、戻り値の Inner にも含めない)** — 主要エラーをマスクしない (推奨、cleanup は best-effort)

B) **cleanup error を `errors.Join` で集約して `New` の error 戻り値に含める** — diagnostic 情報が増えるが、エラーが分かりにくくなる

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 11: ファイル分割の粒度確認

FD `domain-entities.md` §3 で提案した 8 production files + 5 test files をそのまま採用?

A) **そのまま採用** — `doc.go`, `config.go`, `pipeline.go`, `shared.go`, `resource.go`, `exporters.go`, `stats.go`, `errors.go` (推奨)

B) **`pipeline.go` を分割** — `pipeline.go` (struct + accessor) と `pipeline_build.go` (New) に分ける

C) **`exporters.go` を信号別に分割** — `trace_exporter.go`, `metric_exporter.go`, `log_exporter.go`

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 12: ドキュメント (Example function)

`exporter/doc_test.go` の Example function:

A) **3 件のみ** — `ExampleNew`, `ExampleConfig_MergeWith`, `ExampleGetShared` (推奨、top-level の主要 API)

B) A + `ExamplePipeline_Shutdown`, `ExamplePipeline_Stats` — 5 件

C) **すべての public 関数に Example** — 過剰

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR Design アーティファクトを生成して承認ゲートへ進みます。
