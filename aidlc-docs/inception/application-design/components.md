# Components — xk6-otel-gen

本書は xk6-otel-gen のアプリケーションレベルのコンポーネント定義です。詳細な業務ロジックは Construction フェーズの Functional Design で扱います。

## コンポーネント一覧

| # | コンポーネント | Go パッケージ | レイヤ | 公開度 |
|---|---|---|---|---|
| C1 | Topology Schema & Parser | `topology` | Domain | public |
| C2 | Journey Engine | `journey` | Application | public |
| C3 | Signal Synthesizer | `synth` | Application | public |
| C4 | OTLP Exporter Pipeline | `exporter` | Infrastructure | public |
| C5 | k6 JS Module | `k6otelgen` | Boundary | public (k6 register) |
| C6 | k6 Output Module | `k6output` | Boundary | public (k6 register) |
| (T) | Test Utilities | `testutil/generators` | — | public (test 用ドメインジェネレータ) |
| (S) | Samples & Distribution | `examples/`, `cmd/`, build config | — | — |

**パッケージレイアウト方針**: トップレベルのみ (`internal/` も `pkg/` も使わない)。理由:
- 各パッケージにスタンドアロン用途がある (例: `topology` は YAML を CI で検証、`synth`/`exporter` は別ドライバから再利用)
- ライブラリとしての一般的な Go レイアウトに従い、import パスを簡潔にする (`github.com/ymotongpoo/xk6-otel-gen/topology` 等)
- 公開 API としての規律を保つことで、設計品質と GoDoc 整備を促す
- レイヤ規律 (Boundary→Application→Domain / Boundary→Infrastructure) は `internal/` による強制ではなく、レビューと依存マトリクスで担保 (`component-dependency.md` 参照)

---

## C1 — Topology Schema & Parser (`topology`)

### 責務
- トポロジー YAML スキーマの定義 (Go 構造体 + struct tag)
- YAML パース、スキーマバリデーション、デフォルト値適用
- 障害定義 (`faults:`) のロードとトポロジーモデルへの overlay 組み込み
- JSON Schema (`topology-schema.json`) のエクスポート (エディタ補完用)
- グラフ表現の保持 (サービスノード、依存エッジ、ジャーニー、faults)
- 不変条件のバリデーション (DAG 性、journey が参照するサービスの存在、faults が参照する node/edge の存在)

### 主な型 (Domain Model)

**参照モデル**: YAML 表面は名前文字列だが、Parse 後の Go 公開型は **解決済みポインタ** を保持する (`Edge.From`, `Edge.To`, `Step.Op` 等)。これは IDE 支援・タイポ早期検出・コンパイラによる nil 防御のため。詳細と `Parse` の 2 パスフローは `component-methods.md` の "2-pass parse with resolved pointer references"、YAML スキーマの全体像は [`topology-yaml-schema.md`](./topology-yaml-schema.md) を参照。

**Operation を第一級概念に**: サービスは複数の `Operation` を持ち、outgoing edge はすべて Operation 配下の `Calls` ツリーに住む。Journey は Operation のシーケンスを宣言するだけで、下流呼び出しは Operation tree の traversal で自動展開される。

- `ServiceID` (newtype, `type ServiceID string`) — サービス名識別子。`Schema.Services` のマップキー、エラーメッセージ、将来のネームスペース対応用
- `Schema` — 1 つの YAML ファイル全体を表すルート構造体
  - `Services map[ServiceID]*Service`
  - `Journeys map[string]*Journey`
  - `Faults []FaultSpec` (failures section — overlay として後段で適用)
- `Service` — `Name (ServiceID)`, `Kind` (application/database/external_api/cache/queue), `Replicas`, `Language`, `Framework`, `Version`, `Operations map[string]*Operation`
- `Operation` (NEW, first-class) — サービスの公開呼び出し単位
  - `Name string` — operation 名 (HTTP path / RPC method / message topic 等)
  - `Service *Service` — back-pointer
  - `Calls []*CallNode` — 順序付き呼び出し列 (sequential by default、`parallel:` グループで fan-out)
- `CallNode` (NEW) — `Calls` 配列の要素。`Edge *Edge` (単一呼び出し) または `Parallel []*CallNode` (fan-out グループ) のいずれかが populated
- `Edge` — operation 間の有向呼び出し
  - `From *Operation` (back-pointer, owning operation), `To *Operation` (解決済み)
  - `Protocol` (http/grpc/messaging), `LatencyDist`, `ErrorRate`, `Timeout`, `Retries`, `RetryBackoff`
  - **`OnFailure *RecoveryPolicy` (リカバリーフロー、後述)**
- `RecoveryPolicy` — エッジ失敗時の回復手順
  - `Fallback []*Edge` — 試行する代替エッジの順序付きチェーン (各々が独立した Edge として外部に span/metric を発行)
  - `OnExhausted ExhaustedAction` — `propagate` (カスケード発生) | `return_default` (デフォルト応答で成功扱い) | `succeed_silently` (例: ベストエフォート書き込み)
  - `DefaultResponse map[string]any` — `OnExhausted=return_default` 時のデフォルト値
- `Journey` — `Name`, `Steps []*Step`, `Weight` (for weighted journey selection)
- `Step` — `Op *Operation` (解決済み、entry operation)、optional `Parallel []*Step` (journey-level fan-out、稀)
- `FaultTarget` — `{Kind, Service *Service, Operation *Operation, Edge *Edge}` の variant 型。YAML の `node:<name>` / `operation:<svc>.<op>` / `edge:<from>-><to>` を Parse 時にいずれかのポインタへ解決
- `FaultSpec` — `Target FaultTarget`, `Kind FaultKind` (latency_inflation, error_rate_override, disconnect, crash), `Severity` (確率またはパラメータ)
  - **カスケード派生は事前計算しない** — Journey Engine が実行時にエッジの `OnFailure` チェーンを評価し、結果として step が失敗したときにのみ下流へカスケード

### 公開インターフェース
- `Parse(r io.Reader) (*Schema, error)` — io.Reader から読み込み + バリデーション
- `ParseFile(path string) (*Schema, error)` — ファイルパスから読み込み
- `Validate(s *Schema) error` — 構造的不変条件チェック
- `(s *Schema) FindServiceByName(name string) (*Service, bool)`
- `(s *Schema) JourneyNames() []string`
- `(s *Schema) ApplyFaults() *FaultOverlay` — faults を計算可能な形に変換し返す

### 外部依存
- `gopkg.in/yaml.v3` — YAML パース
- `pgregory.net/rapid` (テストのみ) — PBT

---

## C2 — Journey Engine (`journey`)

### 責務
- パース済み Topology Schema と FaultOverlay から、指定ジャーニーの実行計画 (Plan) を生成
- Plan の実行制御 (基本逐次、`parallel:` ブロックは別 goroutine で並列実行 → join)
- 各ステップで Failure Overlay を参照して結果 (Outcome) を確定し、Synthesizer を呼んで信号を発行
- **エッジ失敗時のリカバリーフロー実行** — `Edge.OnFailure.Fallback` チェーンを順次試行 (それぞれ独立した子 span を作る)
- **条件付きカスケード障害伝播** — リカバリー成功時はカスケードしない / `on_exhausted=propagate` のときのみ下流へ `upstream_unavailable` Outcome を伝播
- レイテンシ実時間シミュレーション (Q6=A — `time.Sleep` で実時間消費)

### 主な型
- `Engine` — Plan を実行するエントリーポイント
- `Plan` — 実行可能な木構造 (`Node` のシーケンス、`ParallelGroup` を含む)
- `Node` — 1 つのサービス呼び出しを表す `{Service, Operation, EdgeRef}`
- `Outcome` — `{Success bool, Latency time.Duration, StatusCode int, ErrorType string}`

### 公開インターフェース
- `NewEngine(schema *topology.Schema, overlay *topology.FaultOverlay, synth synth.Synthesizer) *Engine`
- `(e *Engine) BuildPlan(journeyName string) (*Plan, error)`
- `(e *Engine) Execute(ctx context.Context, plan *Plan) error`

### 外部依存
- `topology` (Schema, FaultOverlay, Service/Edge/Journey types)
- `synth` (Synthesizer interface)
- 標準ライブラリ `context`, `time`, `sync`

---

## C3 — Signal Synthesizer (`synth`)

### 責務
- Journey Engine から `(node, outcome)` を受け取り、OTel Semantic Conventions に準拠した spans / metrics / logs を構築
- Resource 属性 (`service.name`, `service.namespace`, `service.instance.id`, `telemetry.sdk.*`, `host.name` 等) の生成と付与
- HTTP / RPC 属性の付与 (Q7=B 相当の Semantic Conventions 主要部分のみ、FR-6)
- エラー時の `error.type` / `exception.*` 属性、Span Status=ERROR の付与
- 構築した signals を OTel SDK Provider (Tracer/Meter/Logger) に投入 (export は SDK の BatchProcessor が担当)

### サブ構造
- `synth/attributes` — Semantic Conventions マッピング (HTTP/RPC/Error 用属性付与ヘルパー)
- `synth/resources` — `Resource` 構築 (一度作って各 Provider に注入)
- `synth/trace`, `synth/metric`, `synth/log` — 信号別の合成ロジック (Synthesizer interface を実装)

### 主な型
- `Synthesizer` interface — `EmitSpan(...)`, `RecordMetric(...)`, `EmitLog(...)` の3メソッド (Journey Engine がこれを呼ぶ)
- `Resource` — OTel SDK `*resource.Resource` のラッパ + プロジェクト固有の合成ロジック
- `default Synthesizer` 実装 — OTel SDK を使う標準実装

### 公開インターフェース (Journey Engine が見るインターフェース)
- `Synthesizer` interface
- `NewDefault(tp trace.TracerProvider, mp metric.MeterProvider, lp log.LoggerProvider) Synthesizer`
- `BuildResource(svc *topology.Service, instanceIdx int) *resource.Resource`

### 外部依存
- `go.opentelemetry.io/otel/...` (trace, metric, log, sdk/resource, sdk/trace, sdk/metric)
- `go.opentelemetry.io/otel/semconv/v1.x.0` — Semantic Conventions 定数
- `topology` (read-only: Service/Edge 型を参照)

---

## C4 — OTLP Exporter Pipeline (`exporter`)

### 責務
- OTel Go SDK の Exporter (OTLP/gRPC, OTLP/HTTP) のインスタンス化と設定
- TracerProvider / MeterProvider / LoggerProvider の構築 (各種 BatchProcessor を含む)
- 設定マージ — JS API オプション > 環境変数 > YAML defaults の優先順位 (Q9=A)
- 自己観測メトリクス: 送信成功カウント、失敗カウント、内部キュー長など (NFR-5.2)
- Graceful shutdown: ペンディング batch の flush
- C5 (JS module) と C6 (output module) で **同一の Pipeline インスタンス** を共有 (シングルトン)

### 主な型
- `Pipeline` — 全 Provider と Exporter を束ねる構造体
- `Config` — `{Protocol: gRPC|HTTP, Endpoint, Headers, Insecure, Compression, Resource overrides, Batch sizes, ...}`
- `Stats` — Pipeline の内部メトリクス snapshot

### 公開インターフェース
- `New(cfg Config) (*Pipeline, error)`
- `(p *Pipeline) TracerProvider() trace.TracerProvider`
- `(p *Pipeline) MeterProvider() metric.MeterProvider`
- `(p *Pipeline) LoggerProvider() log.LoggerProvider`
- `(p *Pipeline) Shutdown(ctx context.Context) error`
- `(p *Pipeline) Stats() Stats`
- `ConfigFromEnv() Config` — 環境変数 (`OTEL_EXPORTER_OTLP_*`) からの組み立て
- `(c Config) MergeWith(override Config) Config` — 優先順位ベースのマージ

### 外部依存
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace`, `otlptracegrpc`, `otlptracehttp`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric`, `otlpmetricgrpc`, `otlpmetrichttp`
- `go.opentelemetry.io/otel/exporters/otlp/otlplog`, `otlploggrpc`, `otlploghttp` (log exporter は安定版が出次第)
- `go.opentelemetry.io/otel/sdk/trace`, `sdk/metric`, `sdk/log`

---

## C5 — k6 JS Module (`k6otelgen`)

### 責務
- k6 拡張として `k6/x/otel-gen` を登録
- JS 側 API (`load(path)`, `runJourney(name)`, `configure(opts)`) を提供
- VU lifecycle 管理 (per-VU の Engine インスタンス、共有 Exporter Pipeline へのアタッチ)
- Topology Loading は process-singleton (1 度ロード、全 VU 共有)
- Journey 実行: `Engine.Execute()` の呼び出しとエラーハンドリング、k6 のロガーへのエラー記録

### 主な型
- `Module` — k6 `modules.Module` interface 実装
- `Instance` — k6 `modules.Instance` interface 実装 (per-VU)
- `TopologyHandle` — JS から見える handle 型 (`.runJourney(name)` メソッドを持つ)

### 公開インターフェース (k6 JS から呼ばれるメソッド)
- `Load(path string) (*TopologyHandle, error)` — トポロジー YAML 読み込み (singleton キャッシュ)
- `Configure(opts goja.Value) error` — Pipeline 設定の JS 側上書き
- `(*TopologyHandle).RunJourney(name string) error` — 1 ジャーニー実行
- `(*TopologyHandle).Journeys() []string` — 利用可能なジャーニー名

### 外部依存
- `go.k6.io/k6/js/modules` — k6 JS module SDK
- `topology`, `journey`, `synth`, `exporter`

---

## C6 — k6 Output Module (`k6output`)

### 責務 (Q3=C — デュアル機能)
- **(a) 合成シグナル egress**: C5 (JS Module) と同一の `exporter.Pipeline` を共有し、合成シグナルの OTLP 送信を担当
- **(b) k6 ネイティブメトリクスの OTLP 化 (実行中のストリーミングのみ)**: k6 の `Output.AddMetricSamples` 経由で受け取る Samples (http_req_duration, vus, iterations, checks 等) を OTLP/Metrics に変換し、同じ Pipeline で送信
- 起動時にコマンドラインオプションをパース (`--out otel-gen=...` または `--out otel-gen` で env から)
- k6 ネイティブメトリクスのリソース属性は本拡張のもの (`service.name="xk6-otel-gen-runner"` 等) で区別

### 責務外 (Out of Scope) — 設計上の明示
- **End-of-run Summary は本モジュールが扱わない**。k6 アーキテクチャ上 Summary は `Output` モジュールには渡されない設計のため、本拡張は Summary に一切触れない。利用者は k6 標準機構をそのまま使える:
  - デフォルトの **stdout サマリ出力**
  - `--summary-export=summary.json` での **JSON ファイル書き出し**
  - JS スクリプト内の **`handleSummary(data)`** での任意フォーマット出力 (テキスト/JSON/HTML 等任意のファイルへ)
- これにより「実行中のメトリクスは OTLP に流しつつ、完了レポートは stdout/ファイル」というユースケースが追加設定なしで成立する。

### 主な型
- `Output` — k6 `output.Output` interface 実装
- `OutputConfig` — `--out otel-gen=KEY=VALUE,...` のパース結果

### 公開インターフェース (k6 Output SDK 契約)
- `New(params output.Params) (output.Output, error)` — k6 が呼ぶファクトリ
- `(o *Output) Start() error`
- `(o *Output) AddMetricSamples(samples []metrics.SampleContainer)`
- `(o *Output) Stop() error`
- `(o *Output) Description() string`

### 外部依存
- `go.k6.io/k6/output` — k6 Output SDK
- `go.k6.io/k6/metrics` — k6 metric 型
- `exporter` (共有 Pipeline)

---

## Samples & Distribution (補助)

### 責務
- `examples/minimal/` — 3 サービス例 (frontend → backend → postgres) + 対応する k6 スクリプト + Docker compose (Collector 起動)
- `examples/astroshop/` — OTel Demo 由来の 10+ サービス例
- README — xk6 ビルド手順、JS API リファレンス、YAML スキーマリファレンス、セキュリティ告知 (Q10: プリビルドバイナリ提供だが自前ビルド推奨を明記)
- `.github/workflows/release.yml` — GoReleaser によるバイナリ自動公開 (Build and Test ステージで完成)
- `JSON Schema` (`schemas/topology.schema.json`) — エディタ補完用

---

## Open Design Decisions (Functional Design / NFR Design で確定)

- レイテンシ分布の具体的なパラメータ表現 (lognormal / normal / exponential のサポート範囲)
- `Edge.OnFailure.Fallback` の **YAML 表現形式**: インラインネスト方式 vs 名前付き共有 `recovery_flows:` セクションの参照方式
- リカバリーチェーンの**ネスト**: fallback 内の Edge にもさらに `OnFailure` を許可するか / 深さ制限
- `DefaultResponse` の attribute 構造とそれが span/log にどう反映されるか (`fallback.default_used=true` 属性 等)
- リカバリー試行中のレイテンシ加算ルール (primary timeout + fallback latency = step total) と timeout 伝播
- 内部メトリクスの命名規約と粒度 (リカバリー使用率の自己観測メトリクス含む)
- `parallel:` ブロックのバウンディング (最大並列数、ネスト可否)
- k6 ネイティブメトリクスの OTLP 変換における attribute マッピング詳細
- Resource 属性の合成ロジック (`telemetry.sdk.language` の生成ルール等)
