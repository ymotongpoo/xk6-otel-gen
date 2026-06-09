# U3 synth — Business Logic Model

本書は `synth/` パッケージの **ビジネスロジック (synthesizer の動作)** を確定する。

参照: Application Design (`component-methods.md` §C3)、`plans/u3-synth-fd-plan.md` の Q1..Q13 回答。

---

## 1. パッケージの責務

`synth/` は **Journey Engine から渡される実行イベント (span / metric / log)** を受け取り、OTel Semantic Conventions に準拠した形で span 開始/終了、metric 記録、log 発出を行う。

### 1.1 入力

| 入力 | 提供者 | 内容 |
|---|---|---|
| `SpanInput` | Journey Engine | Service / Edge / Operation / StartTime / InstanceIdx |
| `Outcome` | Journey Engine (via FinishSpanFunc) | Success / StatusCode / ErrorType / EndTime |
| `MetricInput` | Journey Engine | Service / Edge / Operation / Latency / Outcome |
| `LogInput` | Journey Engine | Service / Severity / Body / Attributes |
| `tp / mp / lp` | k6 module (U5) | OTel SDK Provider 群 (NewDefault 時注入) |

### 1.2 出力

`synth/` 自体は telemetry を直接エクスポートしない。SDK Provider に span/metric/log を流すと、Provider が内部 Processor/Reader 経由で U4 exporter へ流す。

```text
Journey Engine ──(BeginSpan/RecordMetric/EmitLog)──▶ synth
                                                       │
                                                       ▼
                                          OTel SDK Provider (tp/mp/lp)
                                                       │
                                                       ▼
                                          U4 exporter Pipeline ──▶ OTLP endpoint
```

### 1.3 BuildResource

`BuildResource(svc, instanceIdx) *resource.Resource` は **per-service-instance Resource** を構築する。Journey Engine が `BeginSpan` ごとに正しい Resource をどう関連付けるかは U2 (journey) の責務 — 本 unit は per-instance Resource を返すユーティリティを提供するのみ。

> **NOTE**: 1 つの `defaultSynthesizer` インスタンスは複数 service の Resource を内部キャッシュする可能性あり (NFR Design で確定)。FD 時点では「per call BuildResource」を想定 (Idempotency property TP-U3-1)。

---

## 2. Synthesizer interface のフロー

### 2.1 BeginSpan の流れ

```
1. Journey Engine が SpanInput を準備 (Service, Edge, Operation, StartTime, InstanceIdx)
2. synth.BeginSpan(ctx, in):
   a. (Service.Kind, Edge.Kind) から「attribute policy」を Q2 マッピングテーブルで決定
   b. policy.SpanKind (Client / Server / Internal / Producer / Consumer) を確定
   c. tracer = tp.Tracer("synth")
   d. attrs = buildSpanAttributes(in, policy) — semconv keys, 例: http.request.method, rpc.service, db.system
   e. ctx2, span = tracer.Start(ctx, "<Service.Name>.<Operation>",
        trace.WithTimestamp(in.StartTime),
        trace.WithSpanKind(policy.SpanKind),
        trace.WithAttributes(attrs...))
   f. return ctx2, finishFunc(span)
3. Engine が処理を進める (子 span は Engine が ctx2 を継承して再帰的に作成)
4. Engine が Outcome を得たら finishFunc(outcome) を呼ぶ
5. finishFunc:
   a. span.SetStatus(Q7 マッピングで決定)
   b. span.SetAttributes(終了時の attrs: http.response.status_code, error.type 等)
   c. span.End(trace.WithTimestamp(outcome.EndTime))
```

### 2.2 RecordMetric の流れ

```
1. Journey Engine が MetricInput を準備
2. synth.RecordMetric(ctx, in):
   a. policy = Q2 マッピング (Service.Kind, Edge.Kind から namespace 決定)
   b. namespace = policy.MetricNamespace ("http"/"rpc"/"db"/"messaging")
   c. meter = mp.Meter("synth")
   d. histogram = meter.Histogram(namespace + ".client.request.duration" or ".server.request.duration")
   e. attrs = buildMetricAttributes(in, policy)
   f. histogram.Record(ctx, in.Latency.Seconds(), attrs...)
   g. active_requests (UpDownCounter) は span 開始/終了で +1/-1 する (RecordMetric ではなく BeginSpan/FinishSpan が司る — NFR Design で確認)
```

### 2.3 EmitLog の流れ

```
1. Journey Engine が LogInput を準備 (Severity, Body, Attributes)
2. synth.EmitLog(ctx, in):
   a. logger = lp.Logger("synth")
   b. record = log.Record{}
   c. record.SetTimestamp(time.Now())
   d. record.SetObservedTimestamp(time.Now())
   e. record.SetSeverity(in.Severity)
   f. record.SetBody(log.StringValue(in.Body))
   g. record.AddAttributes(toKeyValues(in.Attributes)...)
   h. span context (ctx 経由) は OTel SDK の log/bridge が自動的に span_id/trace_id を関連付ける
   i. logger.Emit(ctx, record)
```

### 2.4 Q8=B: 成功・失敗どちらもログ発出

Synthesizer 側でフィルタリングはしない。フィルタリングが必要なら下流の OTel Collector で行う方針 (separation of concerns)。

- 成功時: `Severity=Info`, `Body="<Service>.<Operation> succeeded"`, `Attributes` に `outcome=success`, `latency_ms`, `status_code`
- 失敗時: `Severity=Error`, `Body="<Service>.<Operation> failed: <ErrorType>"`, `Attributes` に `outcome=failed`, `error.type`, `status_code`, span_id/trace_id (SDK が自動付与)

---

## 3. BuildResource ロジック (Q4=B)

```
1. attrs を build:
   - service.name = svc.Name
   - service.version = svc.Version
   - service.instance.id = uuidV5(svc.Name + "-" + instanceIdx)   ← Q4=A deterministic
   - telemetry.sdk.name = "opentelemetry"
   - telemetry.sdk.language = "go"
   - telemetry.sdk.version = <runtime detected OTel SDK version>
   - process.runtime.name = svc.Language                          ← Q4=B 追加
   - process.runtime.version = "" (未提供)
   - synth.service.framework = svc.Framework                      ← Q4=B 追加 (semconv 非該当のため custom namespace)
2. return resource.NewSchemaless(attrs...)
```

> **NOTE (Q4=B 解釈)**: ユーザー回答 "Service.Language / Framework を `telemetry.sdk.language` と `<framework名>` attribute に" は意味的に微妙。`telemetry.sdk.language` は **SDK 自身の実装言語** (Go) を指す semconv 標準 attribute なので、シミュレートされる service の言語に流用するのは semantic 違反。本 FD は次の妥協案を取る:
> - `telemetry.sdk.language=go` (SDK 標準のまま、これは synth が Go 実装である事実)
> - `process.runtime.name=svc.Language` (semconv 準拠、シミュレートされる service の言語)
> - `synth.service.framework=svc.Framework` (custom namespace、semconv に直接該当なし)
>
> NFR Design 時点で他の解釈に変更したい場合は再議論。

### 3.1 service.instance.id 生成

- **deterministic**: 同じ `(svc.Name, instanceIdx)` から常に同じ UUID
- **アルゴリズム**: UUID v5 (SHA-1 namespace based)
- **namespace UUID**: package-local 固定値 (例: `xk6-otel-gen/synth` の DNS-derived UUID)
- **name**: `svc.Name + "/" + strconv.Itoa(instanceIdx)`

理由: テスト時に予測可能、production でも load test 再現性が確保される。

---

## 4. (Service.Kind, Edge.Kind) → Policy マッピング (Q2=A)

| Service.Kind | Edge.Kind | SpanKind | Attribute namespace | Metric namespace |
|---|---|---|---|---|
| `application` | `http` | Client (outgoing) / Server (incoming) | `http.*` | `http` |
| `application` | `rpc` | Client / Server | `rpc.*` | `rpc` |
| `application` | (nil/internal) | Internal | (none) | (none) |
| `database` | (any) | Client | `db.*` | `db` |
| `cache` | (any) | Client | `db.*` (system=redis 等) | `db` |
| `queue` | `messaging` | Producer / Consumer | `messaging.*` | `messaging` |
| `queue` | (any) | Producer | `messaging.*` | `messaging` |
| `external_api` | (any) | Client | `http.*` + `peer.service` | `http` |

> **NOTE**: `Client` vs `Server` の決定は **Edge の起点・終点** で決まる。Engine が `BeginSpan` を呼ぶ視点で:
> - 自 service が **caller** (Edge.From) → Client span (outgoing)
> - 自 service が **callee** (Edge.To) → Server span (incoming)
> - Edge nil (journey root) → Server (HTTP 受信エンドポイント) または Internal (cron 等)
>
> Engine が `SpanInput` で Client/Server を区別するために必要なら、`SpanInput.Direction` フィールドを追加することを提案する (これは U2 Journey Engine FD 時点で再議論)。

---

## 5. Metric の種類 (Q3=A)

| Metric 名 | 計装タイプ | Unit | キー attr | 計装タイミング |
|---|---|---|---|---|
| `<ns>.client.request.duration` | Histogram | s | `<ns>.method`, `peer.service`, `error.type` | Outgoing call の Finish |
| `<ns>.server.request.duration` | Histogram | s | `<ns>.method`, `<ns>.route`, `<ns>.status_code`, `error.type` | Incoming request の Finish |
| `<ns>.server.active_requests` | UpDownCounter | {request} | `<ns>.method`, `<ns>.route` | BeginSpan で +1、FinishSpan で -1 |

`<ns>` は Q2 マッピングの `Metric namespace` (`http` / `rpc` / `db` / `messaging`)。

---

## 6. Time 制御 (Q9=A)

- `SpanInput.StartTime` をそのまま `trace.WithTimestamp` で span 開始時刻に
- `Outcome.EndTime` をそのまま `trace.WithTimestamp` で span 終了時刻に
- Latency は `EndTime - StartTime` で導出されるが、metric には `MetricInput.Latency` を直接使う (`time.Duration` → 秒)
- log の `Timestamp` / `ObservedTimestamp` は `time.Now()` (log は journey 実行と並行的に発出されるため、Engine 提供の StartTime に縛らない)

### 6.1 Fault による latency inflation

`topology.FaultSpec.Kind=latency_inflation` は Journey Engine が **StartTime/EndTime の間隔を引き延ばす** ことで表現する。Synthesizer は受け取った時刻をそのまま span に詰めるので、fault が transparent に反映される。

---

## 7. Span Status (Q7=A)

```
if outcome.Success:
    if HTTP and outcome.StatusCode in [200..299]:
        Status = Unset (semconv 推奨: success は span status を立てない)
    if HTTP and outcome.StatusCode in [400..499]:
        Status = Unset (client error は span status の対象外)
    if HTTP and outcome.StatusCode in [500..599]:
        Status = Error (server error は明示)
    if RPC and outcome.StatusCode == 0 (OK):
        Status = Unset
    if RPC and outcome.StatusCode != 0:
        Status = Error
else:
    Status = Error  (Success=false なら無条件 Error)
```

具体例:
- HTTP 200 + Success → Unset
- HTTP 404 + Success → Unset (semconv: 4xx は span status の対象外)
- HTTP 503 + !Success → Error
- gRPC OK + Success → Unset
- gRPC UNAVAILABLE + !Success → Error
- Timeout (StatusCode=0) + !Success → Error, error.type="timeout"

---

## 8. ErrorType の扱い (Q6=A)

Journey Engine が `Outcome.ErrorType` を semconv `error.type` 準拠の string として渡す:
- `"timeout"`, `"connection_refused"`, `"dns_failure"` (汎用)
- `"http.500"`, `"http.502"`, `"http.503"`, `"http.504"` (HTTP)
- `"grpc.unavailable"`, `"grpc.deadline_exceeded"`, `"grpc.unauthenticated"` (gRPC)
- `"db.connection_lost"`, `"db.constraint_violation"` (DB)

Synthesizer は受け取った値を `error.type` attribute に **as-is で詰める**。正規化や enum 変換はしない (Q6=A 決定理由: Engine 側でドメインに即した値を確定する責務)。

Engine 側の責務 (U2 FD で確定する):
- `ErrorType` の命名規約ドキュメント作成
- 不正な値 (空文字、空白のみ) を渡さないこと
- 大文字小文字統一 (snake_case 推奨)

---

## 9. Span Kind 決定 (Q10=A)

```
if Edge != nil:
    if policy.MetricNamespace == "messaging":
        Producer if 自 service が Edge.From else Consumer
    else:
        Client if 自 service が Edge.From else Server
else:
    Server (journey entry HTTP endpoint と想定)
```

> **OPEN**: Journey entry node が cron や internal trigger の場合 `Internal` にすべきだが、`SpanInput` から判別する手段がない。U2 Journey Engine FD で `SpanInput.SpanKindHint` のような明示フィールド導入を検討。

---

## 10. Multi-replica (Q5=A)

- Synthesizer は `SpanInput.InstanceIdx int` を受け取る
- Replica 選択ロジックは Journey Engine の責務 (per-step ランダム、per-VU sticky、weighted 等は Engine 側で確定)
- Synthesizer は `service.instance.id` の生成と attribute 付与のみ

### 10.1 SpanInput への InstanceIdx 追加 (FD 修正)

Application Design `component-methods.md` §C3 の `SpanInput` には `InstanceIdx` フィールドが現状なし。U3 FD で **追加する** ことを決定:

```go
type SpanInput struct {
    Service     *topology.Service
    Edge        *topology.Edge   // nil for entry node
    Operation   string
    StartTime   time.Time
    InstanceIdx int              // NEW: replica index, 0..Service.Replicas-1
}
```

`MetricInput` にも同じ `InstanceIdx` を追加する (per-instance breakdown を可能にする)。

---

## 11. Trace context 伝搬

- 全てのフローは `context.Context` 経由で span context を伝搬する
- OTel `propagation` package は使用しない (U4 で確認済の方針 — in-process telemetry 合成、外部リクエストへの context inject 不要)
- 子 span の構築は、`BeginSpan` が返す `ctx2` を Engine が次の呼び出しに渡すことで親子関係を自動的に確立

---

## 12. Provider 注入 (interface 経由)

`NewDefault(tp, mp, lp)` は **OTel 公開 interface** を受け取る:
- `trace.TracerProvider` (`go.opentelemetry.io/otel/trace`)
- `metric.MeterProvider` (`go.opentelemetry.io/otel/metric`)
- `log.LoggerProvider` (`go.opentelemetry.io/otel/log`)

U4 `exporter.Pipeline` の concrete SDK 型 (`*sdktrace.TracerProvider` 等) を直接受け取らない。これにより U3 は U4 を直接 import しない。

```text
U5 (k6 JS module)
    │
    │ exporter.GetShared(...).{TracerProvider, MeterProvider, LoggerProvider}
    ▼
synth.NewDefault(tp, mp, lp)  ← U3 は OTel interface 型しか知らない
```

---

## 13. PBT properties (Q11=A)

| ID | 名前 | 種別 | 概要 |
|---|---|---|---|
| TP-U3-1 | BuildResource Idempotency | PBT-04 | 同じ (svc, idx) で同じ Resource (attribute set 完全一致) |
| TP-U3-2 | Span Attribute Allowed Keys | PBT-03 Invariant | 全 attribute key が semconv const set + 既知 custom namespace に含まれる |
| TP-U3-3 | Histogram Bucket Insertion | PBT-03 Invariant | Latency > 0 の RecordMetric で histogram に必ず 1 件追加 |
| TP-U3-4 | Error.Type Required on Failure | PBT-03 Invariant | Outcome.Success=false の finishFunc 呼び出し後、span.Attributes に `error.type` キーが必ず存在 |

詳細は `business-rules.md` §6 参照。

---

## 14. Out of Scope (U3 では扱わない)

- **Replica 選択ロジック** — Engine の責務 (per-step ランダム / sticky 等)
- **Fault injection の検出/適用** — Engine が timestamp 調整・status code 変更で表現済
- **Pipeline lifecycle** — U4 の責務
- **JS-level API** — U5 の責務
- **k6 native metrics** — U6 の責務
- **Propagation (W3C trace context inject/extract)** — このツールは in-process 合成のため不要
