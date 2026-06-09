# U4 (exporter) — Functional Design Plan

## ユニットコンテキスト

- **Unit ID**: U4
- **パッケージ**: `exporter/`
- **位置づけ**: Infrastructure layer (Clean Architecture)。OTel Go SDK を直接ラップし、`*trace.TracerProvider` / `*metric.MeterProvider` / `*log.LoggerProvider` を一元的に組み立て・shutdown する。OTLP/gRPC + OTLP/HTTP の双方をサポート (FR-4)
- **依存上流**: なし (Domain/Application を import しない)
- **依存下流**: U3 (synth) が Provider を inject される / U5 (k6otelgen) が `*Pipeline` を構築 / U6 (k6output) が同 Pipeline を共有
- **Q5=A 統合**: shared Pipeline holder を **`exporter` パッケージ内部 API** として実装 (`registry/` 独立パッケージにしない)

## 今回の FD スコープ

**Config の業務ロジック**:
- 4 段階 (JS API > env > YAML defaults > built-in) の merge ルール
- `OTEL_EXPORTER_OTLP_*` 環境変数の解釈
- gRPC / HTTP / 共通設定の表現方法
- Resource 属性の override / override-not 区分
- TLS / headers / compression 等のセキュリティ・効率設定

**Provider 構築の業務ロジック**:
- gRPC / HTTP exporter の選択基準 (Config の `Protocol` 値)
- BatchProcessor / Resource / Sampler の組み立て順序
- 3 信号 (traces / metrics / logs) ごとの Provider 構築

**Shared Pipeline holder の業務ロジック**:
- プロセスシングルトン保証 (`sync.Once` ベース)
- 初期化エラー時の再試行可否
- 並行アクセス安全性
- Test での差し替え可能性 (リセット API の有無)

**Stats の業務ロジック**:
- 何をカウントするか (success/fail counts、queue depth、bytes sent 等)
- snapshot semantics (atomic read vs lock)
- Stats を内部メトリクスとして k6 output に流す責務分担

**Shutdown の業務ロジック**:
- pending batch の flush タイムアウト
- 部分失敗の許容範囲
- 多重 shutdown の冪等性

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u4-exporter/functional-design/business-logic-model.md` — Config merge / Provider 構築 / Shared holder / Stats / Shutdown のアルゴリズム
- [ ] `aidlc-docs/construction/u4-exporter/functional-design/business-rules.md` — Config 値の制約、merge 優先順位則、Stats 不変条件、Shutdown SLA、Testable Properties (PBT-01)
- [ ] `aidlc-docs/construction/u4-exporter/functional-design/domain-entities.md` — `Pipeline` / `Config` / `Stats` 型の contract、各メソッドの contract、エラー型階層
- [ ] U7 への generator 追加リクエスト (`ValidConfig` / `AnyConfig` / `ValidStats` 等)

---

## 設計確定のための質問

### Question 1: Config 構造体の粒度

`Config` 1 構造体に全設定を持つか、信号ごと (Traces / Metrics / Logs) に分離するか?

A) **単一 `Config`** — `Protocol` / `Endpoint` / `Headers` / `Insecure` / `Compression` / `BatchSize` / `BatchTimeout` / `ResourceOverrides` 等を 1 つの struct に統合。3 信号で同じ設定を共用 (推奨、典型的なシナリオ — 同じ Collector に 3 信号送る — に最適)

B) **信号ごと分離** — `Config.Traces`, `Config.Metrics`, `Config.Logs` の 3 サブ構造体。信号ごとに別 endpoint / 設定を持てる柔軟性

C) **ハイブリッド** — 共通設定 (`Endpoint`, `Headers`) は top-level、信号固有 (BatchSize 等) はサブ構造体

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 2: `OTEL_EXPORTER_OTLP_*` の対応範囲

OTel SDK は標準で `OTEL_EXPORTER_OTLP_ENDPOINT` 等を尊重するが、本 unit でどこまで明示サポートするか?

A) **標準 env 全部尊重** — `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_EXPORTER_OTLP_PROTOCOL` (`grpc` / `http/protobuf`), `OTEL_EXPORTER_OTLP_COMPRESSION`, `OTEL_EXPORTER_OTLP_TIMEOUT`, `OTEL_EXPORTER_OTLP_INSECURE`、加えて信号別 (`_TRACES_`, `_METRICS_`, `_LOGS_` prefix) も尊重 (推奨、OTel エコシステム互換性最大)

B) **基本のみ** — `OTEL_EXPORTER_OTLP_ENDPOINT` + `_HEADERS` + `_PROTOCOL` のみ、他は OTel SDK のデフォルト挙動に任せる

C) **完全に独自** — env を見ない、JS / Config から渡された値のみ使う

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 3: Shared Pipeline holder の実装パターン

Q5=A で「exporter 内部 API として実装」と確定済み。具体的にどう?

A) **`exporter` 内に unexported package-level `var sharedPipeline *Pipeline` + `sync.Once`、公開 API として `GetShared() (*Pipeline, error)` と `SetShared(*Pipeline) error` (テスト用)** — シンプル、k6otelgen と k6output 両方からアクセス可 (推奨)

B) **`exporter.Registry` 構造体を導入し、`NewRegistry()` で明示的にインスタンス化** — singleton を強制せず、複数 registry を作れる (テスト分離に有利だが、k6 統合系のコードが少し冗長)

C) **`context.Context` 経由で渡す** — `WithPipeline(ctx, p)` / `PipelineFromContext(ctx)`。k6 が context を持ち回るパスがあるかは要確認

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 4: BatchProcessor 設定のデフォルト値

OTel SDK の BatchSpanProcessor / BatchLogProcessor / PeriodicReader (metric) のチューニング値:

A) **OTel SDK デフォルトをそのまま採用** — `BatchSize=512`, `BatchTimeout=5s`, `MaxQueueSize=2048` 等。利用者が必要なら Config で override (推奨、最小判断)

B) **本拡張の用途 (k6 高頻度ジャーニー) 向けにチューニング** — `BatchSize=1024`, `BatchTimeout=1s`, `MaxQueueSize=8192` 等、高 throughput 向けに増強

C) **2 種類のプリセット** — `Default` (SDK 既定) と `HighThroughput` (本拡張向け) を提供、Config で選択

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 5: 3 信号の初期化失敗時の挙動

例えば Traces exporter は成功したが Metrics exporter が失敗した場合:

A) **全 or nothing** — どれか 1 つでも失敗したら `New(Config)` がエラーを返し、Pipeline は構築しない。シンプル (推奨、k6 ラン全体を fail fast)

B) **部分初期化** — 成功した信号のみ Provider を持ち、失敗した信号は no-op Provider に。`Stats` で失敗状態を確認可能

C) **エラー集約 + 利用者判断** — `New(Config)` が `*MultiSignalError` を返し、それでも `*Pipeline` も返す。利用者が「成功した信号だけ使う」を選べる

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 6: Stats の構成要素

`Stats` 構造体に含む値:

A) **送信成功/失敗カウント (信号別) + 内部キュー長 (信号別) のみ** — 最小、シンプル (推奨)

B) A に加えて: 送信バイト数 (信号別)、平均レイテンシ、リトライ回数、dropped sample count

C) A + B + OTel SDK の内部 stats も統合 (Provider が露出する metric を取り込み)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 7: Stats snapshot の atomic 性

`(*Pipeline).Stats() Stats` 呼び出し時の atomic 保証:

A) **per-field `atomic.Int64`** — 各カウンタを `atomic.Int64` で管理、`Stats()` は構造体に Load してコピーを返す (推奨、ロック不要)

B) **`sync.Mutex` でロック** — Stats 全体を一貫した瞬間で取れる、コードはシンプルだが contention

C) **`atomic.Pointer[Stats]`** — 内部で全 stats を 1 つの struct に詰め、`Pointer.Load()` で取得。書き込みは CoW

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 8: Graceful Shutdown の SLA

`(*Pipeline).Shutdown(ctx)` の振る舞い:

A) **`ctx` のデッドラインまで pending batch を flush、超過したら未送信を放棄してエラー返却** — OTel SDK の Shutdown 挙動に従う (推奨、SDK との整合)

B) A に加えて **2 段階タイムアウト** — 半分の時間で graceful flush、残り半分で forceful drain

C) **ノンブロッキング** — Shutdown は即座に返し、bg goroutine で flush 完了

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 9: 多重 Shutdown / 多重 New の挙動

A) **多重 New は最初の成功インスタンスを返す (idempotent)、多重 Shutdown は 2 回目以降は no-op** — k6 lifecycle で複数 VU init が同時に Pipeline を要求するシナリオに頑健 (推奨)

B) **多重 New はエラー (重複初期化検知)、多重 Shutdown もエラー** — 厳密、利用者がライフサイクルを正しく管理することを強制

C) **多重 New は最新を返す (上書き)、Shutdown はベストエフォート** — 柔軟だが意図せず置換のリスク

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 10: Resource 属性の override 規則

Config の `ResourceOverrides map[string]string` をどう Resource に反映するか?

A) **OTel SDK の自動検出 (env, host detector 等) → JS / Config の override で上書き、merge は SDK 標準** — `resource.New(ctx, resource.WithDetectors(...), resource.WithAttributes(...))` (推奨)

B) **完全な置換** — override が指定されたら detector を無視、override map だけで Resource を構成

C) **detector 結果と override を mid-way merge** — 特定の key (`service.name` 等) は override 優先、`os.type` 等は detector 優先

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 11: テスタブル特性 (PBT-01)

U4 で必要な testable properties:

A) **Config merge 優先順位則 (PBT-03 invariant)、`Config.MergeWith` の idempotency (PBT-04)、OTLP protobuf round-trip (PBT-02、test only)、Stats の monotonicity (counters は減らない) (PBT-03)** — コア 4 件 (推奨)

B) A + Provider 構築の決定論性 (同じ Config から同じ Provider が出る)

C) A + B + 統合テストでの Collector 受信検証 (real OTel Collector を Docker 起動して送信確認、integration test 寄り、NFR-4.1 で対応するので FD では別件)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 12: U7 への generator 追加リクエスト

U4 の FD で必要となる U7 ジェネレータ:

A) **`ValidConfig` / `AnyConfig` のみ** — Config merge / Pipeline 構築のテストに使う (推奨、最小)

B) A + `ValidStats` / `AnyStats` (Stats invariant のテスト用)

C) A + B + `ValidOTLPRequest` / `AnyOTLPRequest` (OTLP protobuf round-trip 用)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 3 つの設計アーティファクトを生成して承認ゲートへ進みます。
