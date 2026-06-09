# U3 (synth) — Functional Design Plan

## ユニットコンテキスト

- **Unit ID**: U3
- **パッケージ**: `synth/`
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → **U3 (this)** → U2 → U5 → U6 → U8
- **Purpose** (Application Design より):
  - OTel Semantic Conventions 主要部分準拠の span / metric / log 合成
  - Resource 属性の per-service 生成 (`service.name`, `service.instance.id`, `telemetry.sdk.*`)
  - HTTP / RPC / Error 属性付与
  - Synthesizer interface (Journey Engine がこれを呼ぶ)
  - TracerProvider / MeterProvider / LoggerProvider の **interface 注入** (U4 に直接依存しない)
- **Upstream artifacts**:
  - `aidlc-docs/inception/application-design/component-methods.md` §C3 (synth interface + types)
  - `aidlc-docs/inception/application-design/components.md`
  - `aidlc-docs/inception/requirements/requirements.md` (R-7, R-12 semconv 言及)
  - U4 FD (`exporter/`) — Synthesizer は U4 と疎結合だが、Pipeline accessor を受け取る前提

## FD で確定すべき事項

FD は「**何をする / どんなドメインルールに従う**」を確定する (NFR-D が「どう実装するか」)。U3 FD で扱う事項:

- **OTel Semantic Conventions のバージョン固定** と適用範囲 (HTTP / RPC / DB / Generic)
- **Span attributes 決定ルール** — Service.Kind と Edge.Kind から span kind と attribute set を決定
- **Metric 計装の種類と命名** — request count / latency histogram / active gauge の semconv key
- **Resource attribute セット** — service.* / telemetry.sdk.* / host.* / process.* (どこまで本 unit で扱うか)
- **service.instance.id 生成戦略** — Replicas > 1 の場合の分配
- **Log severity マッピング** — Outcome.Success / ErrorType → log.Severity
- **タイム制御** — Journey Engine から渡る StartTime/EndTime をそのまま使うか
- **Trace context 伝搬** — context.Context 経由のみ (U4 で確認済の通り propagation package 非使用)
- **エラー型タクソノミー** — Outcome.ErrorType の正規化と semconv `error.type` への mapping
- **PBT properties** — semantic conventions 準拠の round-trip / invariant
- **U7 への generator 追加リクエスト** — SpanInput / MetricInput / LogInput / Outcome の generator

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u3-synth/functional-design/business-logic-model.md`
- [ ] `aidlc-docs/construction/u3-synth/functional-design/business-rules.md`
- [ ] `aidlc-docs/construction/u3-synth/functional-design/domain-entities.md`

---

## 設計確定のための質問

### Question 1: OTel Semantic Conventions バージョン

合成 telemetry が従う semconv のバージョンを固定する必要がある。

A) **`semconv/v1.27.0` 系の HTTP / RPC / DB 安定 namespace を採用**、ただし `go.opentelemetry.io/otel/semconv` を **import せず** raw string key を使う (U4 で確認した方針: バージョン coupling を避ける)。本 unit 内に `const` でキーを定義し、コメントで参照 semconv version を明記 (推奨)

B) `go.opentelemetry.io/otel/semconv/v1.27.0` を直接 import — タイポ防止、IDE 補完。ただし将来 semconv module bump の影響を直接受ける

C) **最新 (`v1.30.x` 以降を都度追従)** — `go.opentelemetry.io/otel/semconv/v1.NN.0` の latest を tech-stack-decisions に明記し、年 1 回程度更新

X) Other

[Answer]: B - なぜ semconv パッケージを使わないのかがよくわからない。将来 module bump があったとしても、対応すれば良いのでは？

---

### Question 2: Service.Kind → Span Kind / Attribute Set マッピング

`topology.Service.Kind` (`application | database | external_api | cache | queue`) と `Edge` の関係から span 属性をどう決定?

A) **マッピングテーブル方式**: 内部 const map で `(ServiceKind, EdgeKind) → SpanKind + AttrPolicy` を定義。例:
   - `application + http` → `SpanKind=Client/Server`, HTTP attrs (`http.request.method`, `http.response.status_code` 等)
   - `application + rpc` → RPC attrs (`rpc.system`, `rpc.service`, `rpc.method`)
   - `database + any` → DB attrs (`db.system`, `db.operation`)
   - `cache + any` → `db.system=redis` 等 (cache を DB の特殊形として)
   - `queue + any` → Messaging attrs (`messaging.system`, `messaging.operation`)
   - `external_api + any` → HTTP Client attrs + `peer.service`
   (推奨)

B) **Service.Kind 単独で決定** (Edge.Kind 無視) — シンプルだが HTTP と RPC の区別ができない

C) **Edge.Kind 単独で決定** (Service.Kind 無視) — Service の特性 (DB 固有 attrs) が出せない

X) Other

[Answer]: A

---

### Question 3: Metric の種類と命名

Journey 実行から記録する metric の種類:

A) **3 種類の semconv 準拠 metric** (推奨):
   1. `{namespace}.client.request.duration` (Histogram, seconds, by Edge) — outgoing call 計装
   2. `{namespace}.server.request.duration` (Histogram, seconds, by Operation) — incoming call 計装
   3. `{namespace}.server.active_requests` (UpDownCounter, by Operation) — 並行リクエスト数
   namespace は `http` / `rpc` / `db` / `messaging` (Q2 のマッピングに準じる)

B) **A + counter `*.request.count`** — 4 種類。Histogram から count は導出できるが explicit に持つ案

C) **単一汎用 metric `synth.operation.duration`** — semconv 非準拠、シンプルだがダッシュボード親和性低

X) Other

[Answer]: A

---

### Question 4: Resource 属性セットと service.instance.id

`BuildResource(svc, instanceIdx)` で生成する Resource attributes:

A) **最小+セマンティック**: `service.name`, `service.version`, `service.instance.id`, `telemetry.sdk.name=opentelemetry`, `telemetry.sdk.language=go`, `telemetry.sdk.version=<sdk-version>`, `host.name=<empty>` (Synthesizer は host を持たない、空または `topology://<service>`)。`service.instance.id` は `fmt.Sprintf("%s-%d", svc.Name, instanceIdx)` の UUID v5 ハッシュ (deterministic、テストしやすい)

B) **A + Service.Language / Framework を `telemetry.sdk.language` と `<framework名>` attribute に**

C) **OTel SDK 標準 resource.Detect 全部 (Host / Process / OS) も含む** — 過剰、Synthesizer は仮想サービスを模擬する

X) Other

[Answer]: B

---

### Question 5: Multi-replica の trace への影響

`Service.Replicas > 1` の場合、journey 実行ごとにどの replica を選ぶ?

A) **per-step ランダム選択** (Journey Engine 側の責務、Synthesizer は instance index を受け取るだけ) — Synthesizer は責務最小化、Engine が `BeginSpan` 時に index を `SpanInput` に詰める。`SpanInput.InstanceIdx int` フィールド追加 (推奨)

B) **per-VU sticky選択** (k6 VU = replica 固定) — k6 VU 概念を Synthesizer が知る必要があり、責務漏れ

C) **Synthesizer 内部で hash-based に決定** — input が増えると非決定的

X) Other

[Answer]: A

---

### Question 6: ErrorType の正規化

`Outcome.ErrorType string` を semconv `error.type` にどう mapping?

A) **Journey Engine が semconv 準拠 string を直接渡す** (`"timeout"`, `"connection_refused"`, `"http.500"`, `"http.503"`, `"grpc.unavailable"` 等) — Synthesizer は as-is で attribute 値に詰める (推奨)。Engine 側の責務として `error.type` 命名規約のドキュメント整備が必要

B) **Synthesizer 内に正規化テーブル** — 自由 string を semconv 値に変換。命名規約を Synthesizer に集約できるが Engine とのインタフェースが緩む

C) **Outcome.ErrorType を enum 化** (`ErrorTypeTimeout`, `ErrorTypeConnRefused` 等) — 型安全、ただし拡張時 enum 追加コストあり

X) Other

[Answer]: A

---

### Question 7: Span Status / Status Code

Span の `Status` (`Ok` / `Error` / `Unset`) と HTTP/RPC status code の関係:

A) **semconv 準拠**: HTTP `4xx` → Span Status `Unset` (client error は server から見て normal)、HTTP `5xx` → `Error`。RPC code 同様 (`OK` → Unset、それ以外 → Error)。`Outcome.Success=false` で `Status=Error` を強制。詳細は semconv `http.server.request.duration` ドキュメントに準拠 (推奨)

B) **`Outcome.Success` を最優先**: false なら無条件 `Error`、true なら `Ok`。Status code は attribute としてのみ記録

C) **常に `Unset`** (semconv 推奨される最小設定)

X) Other

[Answer]: A

---

### Question 8: Log 計装の範囲

`EmitLog(ctx, l)` をいつ呼ぶか (Journey Engine 側の決定だが、Synthesizer の想定として):

A) **失敗時のみ structured log** (`severity=Error`, `body="{Service}.{Operation} failed: {ErrorType}"`, attrs に span_id/trace_id/error.type を含む)。成功時はログ無し (推奨、Trace に情報がすべて含まれるので冗長回避)

B) **成功・失敗どちらも 1 log** (severity Info / Error) — Loki/Splunk 等 log-first ダッシュボードで欠落を防ぐ

C) **JS 側から明示的に呼ばれた時のみ** (Synthesizer は JS API 経由の log emission を受けて 1 行記録)

X) Other

[Answer]: B - 極力Synth側ではフィルタリングはしない。必要ならフィルタリングだけのOpenTelemetry Collectorプロキシを立てればよい

---

### Question 9: タイム制御

`SpanInput.StartTime` と `Outcome.EndTime` の扱い:

A) **両方とも Journey Engine 提供値をそのまま使用** — fault による latency inflation は Engine が StartTime/EndTime を調整して反映。Synthesizer は受領値を `WithTimestamp` で span に詰めるだけ。テスト容易性確保 (推奨)

B) **StartTime は Engine 提供、EndTime は `time.Now()`** — fault が無い場合の reality に近いが latency_inflation を Engine から表現できない

C) **両方とも Synthesizer 内で `time.Now()`** — 最もシンプル、fault 表現不可

X) Other

[Answer]: A

---

### Question 10: span kind の決定

`SpanInput` に対する span kind:

A) **Edge から決定** — `Edge` non-nil なら `Client` (outgoing call の計装)、`Edge` nil (journey entry root) なら `Server` または `Internal`。`messaging` の場合は `Producer/Consumer` (推奨)

B) **Service.Kind から決定** — `database` / `cache` → Client、`application` → Server

C) **両方の組み合わせ + 明示 hint** — `SpanInput.SpanKind` フィールドを追加し Engine が決定

X) Other

[Answer]: A

---

### Question 11: Testable Properties (PBT)

U3 で扱う property の見積もり (PBT-01 識別):

A) **4 properties** (推奨):
   - TP-U3-1: BuildResource — 同じ (svc, instanceIdx) で同じ resource (Idempotency, PBT-04)
   - TP-U3-2: span attributes ⊆ semconv allowed keys (Invariant, PBT-03) — 全 attribute key が固定 const set に含まれる
   - TP-U3-3: metric data point — RecordMetric で Latency が `> 0` の場合 histogram bucket に必ず 1 件追加 (Invariant)
   - TP-U3-4: error.type の不変条件 — Outcome.Success=false の場合 error.type attribute が必ず付与される (Invariant)

B) A + TP-U3-5: span timestamp の monotonicity (StartTime ≤ EndTime)

C) A + B + TP-U3-6: log emission count = failure count (Outcome.Success=false の回数)

X) Other

[Answer]: A

---

### Question 12: U7 への generator 追加リクエスト

PBT 用に U7 (`testutil/generators/`) へ追加する generator:

A) **4 generator** (推奨):
   - `ValidSpanInput()` / `AnySpanInput()` — `synth.SpanInput` (Service, Edge, Operation, StartTime, InstanceIdx)
   - `ValidMetricInput()` / `AnyMetricInput()` — `synth.MetricInput`
   - `ValidLogInput()` / `AnyLogInput()` — `synth.LogInput`
   - `ValidOutcome()` / `AnyOutcome()` — `synth.Outcome` (Success, StatusCode, ErrorType, EndTime)
   合計 8 関数 (4 pairs)

B) A + `ValidErrorType()` (semconv 準拠 error.type 値の generator)

C) **必要に応じて追加**、PBT 設計時に決める

X) Other

[Answer]: C - ValidErrorType() もあったほうがいいように思うけど、正直テストを見ないと判断できない

---

### Question 13: ファイル分割

`synth/` パッケージのファイル構成案:

A) **6 production + 5 test files** (推奨):
   - `doc.go`
   - `interface.go` — Synthesizer interface + I/O 型 (SpanInput / MetricInput / LogInput / Outcome / FinishSpanFunc)
   - `synthesizer.go` — defaultSynthesizer struct + NewDefault + BeginSpan/RecordMetric/EmitLog 実装
   - `resource.go` — BuildResource (per-service instance Resource)
   - `attributes.go` — Q2 マッピングテーブル + const semconv key 群 + attribute build helpers
   - `errors.go` — synth-specific error 型 (もしあれば、なければ omit)
   - tests: `interface_test.go`, `synthesizer_test.go`, `resource_test.go`, `attributes_test.go`, `pbt_test.go`

B) **すべて 1 ファイル `synth.go`** — 規模次第だが multi-responsibility になりがち

C) **信号別に分割** — `traces.go`, `metrics.go`, `logs.go` — 共通の attribute build 部分が重複

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 3 つの FD アーティファクト (business-logic-model / business-rules / domain-entities) を生成して承認ゲートへ進みます。
