# U3 synth — Business Rules

本書は U3 (`synth/`) の業務規則・不変条件・Testable Properties を確定する。

---

## 1. Semantic Conventions バージョン (Q1=B)

### 1.1 採用方針

- **`go.opentelemetry.io/otel/semconv/v1.27.0`** を直接 import する
- すべての semconv key 参照は constants 経由 (`semconv.HTTPRequestMethodKey`, `semconv.RPCSystemKey` 等)
- 全 import は `synth/attributes.go` 1 ファイルに集約 (将来 bump 時の影響を局所化)

### 1.2 プロジェクト全体での統一方針

`semconv/v1.27.0` の import は **本プロジェクトの標準** とする。U4 (`exporter/`) でも import 可 (NFR-R `tech-stack-decisions.md` §1.2 で許可済)。U4 production code は user-provided key を扱うため現状 semconv 定数の出番がないが、テストコードや将来の hard-coded attribute 追加時には semconv 定数を使う。

差異があった当初の議論 (U4 で raw string を採用するとした案) は撤回済 — typo 防止 / IDE 補完 / 単一 import path での bump 管理の利点を全 unit で享受する。

### 1.3 Bump プロトコル

将来 semconv の上位 version (例: `v1.28.0`, `v1.30.0`) に bump する場合:
1. `attributes.go` の import path を `semconv/v1.27.0` → `semconv/v1.NN.0` に変更
2. 廃止された symbol を新しい symbol に grep+replace
3. PBT TP-U3-2 で新規 attribute set の invariant を更新
4. NFR Design で許可された semconv version range を更新

---

## 2. (Service.Kind, Edge.Kind) → AttributePolicy マッピング (Q2=A)

### 2.1 マッピング table

| Service.Kind | Edge.Kind | SpanKind | Attribute namespace | Metric namespace |
|---|---|---|---|---|
| `application` | `http` | Client / Server | `http.*` | `http` |
| `application` | `rpc` | Client / Server | `rpc.*` | `rpc` |
| `application` | (nil/internal) | Internal | (none) | (none) |
| `database` | (any) | Client | `db.*` | `db` |
| `cache` | (any) | Client | `db.*` | `db` |
| `queue` | `messaging` | Producer / Consumer | `messaging.*` | `messaging` |
| `queue` | (any) | Producer | `messaging.*` | `messaging` |
| `external_api` | (any) | Client | `http.*` + `peer.service` | `http` |

### 2.2 SpanKind 内訳 (Q10=A)

- Client/Server の区別: 自 service が Edge.From → Client、Edge.To → Server
- Producer/Consumer の区別: 同上、From → Producer、To → Consumer
- Edge nil (journey entry) → Server (デフォルト、将来 `SpanInput.SpanKindHint` で上書き可能化検討)

### 2.3 HTTP 属性セット (Q1=B / Q2=A)

`policy.AttributeNamespace == "http"` 時に付与:

| 属性 (semconv key) | Span 種別 | 必須/任意 |
|---|---|---|
| `http.request.method` (semconv.HTTPRequestMethodKey) | Client + Server | 必須 |
| `http.response.status_code` (semconv.HTTPResponseStatusCodeKey) | Client + Server | finishFunc 時 |
| `http.route` (semconv.HTTPRouteKey) | Server | 推奨 (Operation 名から) |
| `server.address` (semconv.ServerAddressKey) | Client | 推奨 |
| `server.port` (semconv.ServerPortKey) | Client | 任意 |
| `url.path` (semconv.URLPathKey) | Client | 任意 (Operation 名から導出) |
| `peer.service` | Client (external_api) | external_api 限定 |
| `error.type` (semconv.ErrorTypeKey) | Client + Server | Outcome.Success=false 時 必須 |

### 2.4 RPC 属性セット

| 属性 | Span 種別 |
|---|---|
| `rpc.system` (semconv.RPCSystemKey) | Client + Server (`"grpc"` 固定 初期は) |
| `rpc.service` (semconv.RPCServiceKey) | Client + Server (Service.Name) |
| `rpc.method` (semconv.RPCMethodKey) | Client + Server (Operation) |
| `rpc.grpc.status_code` (semconv.RPCGRPCStatusCodeKey) | finishFunc 時 |
| `error.type` | failure 時 必須 |

### 2.5 DB 属性セット

| 属性 | Span 種別 |
|---|---|
| `db.system` (semconv.DBSystemKey) | Client (Service.Framework から推定: `"postgresql"` / `"redis"` 等) |
| `db.operation` (semconv.DBOperationKey) | Client (Operation 名) |
| `db.statement` | (省略 — 合成データに real SQL は無い) |
| `error.type` | failure 時 必須 |

### 2.6 Messaging 属性セット

| 属性 | Span 種別 |
|---|---|
| `messaging.system` (semconv.MessagingSystemKey) | Producer + Consumer (`"kafka"` 等、Service.Framework から) |
| `messaging.operation` (semconv.MessagingOperationKey) | Producer (`"publish"`) / Consumer (`"receive"` or `"process"`) |
| `messaging.destination.name` (semconv.MessagingDestinationNameKey) | Producer + Consumer (Edge target Service.Name) |
| `error.type` | failure 時 必須 |

---

## 3. Metric 命名と単位 (Q3=A)

### 3.1 計装の種類

| Metric 名 (テンプレート) | Instrument | Unit | キー attrs |
|---|---|---|---|
| `<ns>.client.request.duration` | Histogram | `s` (秒) | `<ns>.method`, `peer.service`, `error.type` |
| `<ns>.server.request.duration` | Histogram | `s` | `<ns>.method`, `<ns>.route` (HTTP) / `rpc.service`+`rpc.method` (RPC), status code, `error.type` |
| `<ns>.server.active_requests` | UpDownCounter | `{request}` | `<ns>.method`, `<ns>.route` |

`<ns>` は Q2 マッピングの `Metric namespace`。

### 3.2 Histogram bucket boundaries

OTel SDK default の advice (5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s) を **adopt**。本 unit では explicit には設定しない (SDK 既定に従う、NFR Design で再評価)。

### 3.3 active_requests のライフサイクル

```
BeginSpan:
    if policy.MetricNamespace != "" and SpanKind == Server (or Consumer):
        active_requests.Add(ctx, 1, server attrs...)

FinishSpan (finishFunc 内):
    if same condition:
        active_requests.Add(ctx, -1, server attrs...)
```

Client span では active_requests は更新しない (server 側 metric)。

### 3.4 Latency = 0 の扱い

`MetricInput.Latency == 0` でも Histogram に record する (0s bucket にカウント)。Engine が `Latency` を明示的に渡さない場合 (rare) は呼び出しスキップする責務は Engine 側。

---

## 4. Resource 属性 (Q4=B 解釈版)

### 4.1 属性セット (BuildResource が必ず付与)

| 属性 key | 値 | Source |
|---|---|---|
| `service.name` (semconv.ServiceNameKey) | `svc.Name` | topology.Service.Name |
| `service.version` (semconv.ServiceVersionKey) | `svc.Version` | topology.Service.Version (空なら attribute 省略) |
| `service.instance.id` (semconv.ServiceInstanceIDKey) | UUID v5 of `svc.Name/instanceIdx` | deterministic 生成 |
| `telemetry.sdk.name` (semconv.TelemetrySDKNameKey) | `"opentelemetry"` | 固定 |
| `telemetry.sdk.language` (semconv.TelemetrySDKLanguageKey) | `"go"` | 固定 (SDK 自体は Go) |
| `telemetry.sdk.version` (semconv.TelemetrySDKVersionKey) | runtime detected | OTel SDK が提供 |
| `process.runtime.name` (semconv.ProcessRuntimeNameKey) | `svc.Language` | topology.Service.Language (例: `"go"`, `"python"`, `"java"`) |
| `synth.service.framework` | `svc.Framework` | topology.Service.Framework (custom namespace、semconv 未該当) |

### 4.2 不変条件

- 同じ `(svc, instanceIdx)` 入力で **同じ Resource attribute set** が返る (TP-U3-1)
- attribute set は ↑ table の固定 superset (TP-U3-2 の対象)
- `service.name` は必ず空でない (svc.Name が空なら panic — 上流 topology Validate でブロック済前提)
- `service.instance.id` は必ず UUID 形式 (deterministic でも有効な v5 UUID)

### 4.3 UUID namespace

```go
// xk6-otel-gen 内 synth で生成する service.instance.id の namespace UUID。
// 固定値で deterministic を保証する。
var synthInstanceNamespace = uuid.MustParse("00000000-0000-0000-0000-000000000003")
//                                                                       ^ U3
```

具体値は NFR Design で確定 (上記は placeholder)。

---

## 5. Log 計装ルール (Q8=B)

### 5.1 成功・失敗どちらもログ発出

Synthesizer は **filtering をしない**。フィルタリングは下流の OTel Collector で行う。

### 5.2 Severity マッピング

| Outcome.Success | log.Severity |
|---|---|
| `true` | `Info` |
| `false` | `Error` |

明示的に Engine が `LogInput.Severity` を指定した場合はそれを尊重。デフォルトは ↑。

### 5.3 Body フォーマット

| 状況 | Body テンプレート |
|---|---|
| Success | `"<Service.Name>.<Operation> succeeded"` |
| Failure | `"<Service.Name>.<Operation> failed: <ErrorType>"` |

Engine が `LogInput.Body` を明示した場合はそれを優先。Synthesizer のデフォルト Body 生成は failure log の場合のみ自動 (success log は journey-level 集約 log を想定し、デフォルト Body 生成しない選択もあり — NFR Design で再評価)。

### 5.4 自動付与 attribute

すべての log record に以下を attached:

| 属性 | 値 |
|---|---|
| `service.name` | `svc.Name` (LogInput.Service から) |
| `outcome` | `"success"` / `"failed"` |
| `span_id` | (SDK が context から自動付与) |
| `trace_id` | (SDK が context から自動付与) |
| `error.type` | failure log のみ |
| (caller-supplied attrs) | LogInput.Attributes を merge |

---

## 6. Testable Properties (PBT-01)

### TP-U3-1: BuildResource Idempotency (PBT-04)

```text
For all svc, instanceIdx drawn from ValidService() and ValidInstanceIdx():
    r1 := synth.BuildResource(svc, instanceIdx)
    r2 := synth.BuildResource(svc, instanceIdx)
    attributeSet(r1) == attributeSet(r2)
```

実装:
```go
func TestBuildResource_Idempotent(t *testing.T) {
    t.Parallel()
    rapid.Check(t, func(t *rapid.T) {
        svc := generators.ValidService().Draw(t, "svc")
        idx := rapid.IntRange(0, svc.Replicas-1).Draw(t, "idx")
        r1 := synth.BuildResource(svc, idx)
        r2 := synth.BuildResource(svc, idx)
        require.True(t, r1.Equal(r2), "Resource not idempotent")
    })
}
```

### TP-U3-2: Span Attribute Allowed Keys (PBT-03 Invariant)

```text
For any SpanInput in drawn from ValidSpanInput():
    let span = beginAndFinishSpan(in, defaultOutcome)
    let keys = attributeKeys(span)
    keys ⊆ semconv.AllowedKeySet ∪ {"synth.service.framework", "outcome"}
```

実装は `attributes.go` 内の `var allowedAttrKeys = map[string]bool{...}` で集合を維持。

### TP-U3-3: Histogram Bucket Insertion (PBT-03 Invariant)

```text
For any MetricInput in with in.Latency > 0:
    Before := histogramCount(name, attrs)
    synth.RecordMetric(ctx, in)
    After := histogramCount(name, attrs)
    After == Before + 1
```

### TP-U3-4: Error.Type Required on Failure (PBT-03 Invariant)

```text
For any SpanInput in and Outcome o with o.Success == false:
    span := beginAndFinishSpan(in, o)
    "error.type" ∈ attributeKeys(span)
    attrValue(span, "error.type") == o.ErrorType
```

---

## 7. Provider 接続規約

### 7.1 Tracer / Meter / Logger の取得 instrumentation name

統一して `"github.com/ymotongpoo/xk6-otel-gen/synth"` を使う:

```go
tracer := s.tp.Tracer("github.com/ymotongpoo/xk6-otel-gen/synth")
meter  := s.mp.Meter("github.com/ymotongpoo/xk6-otel-gen/synth")
logger := s.lp.Logger("github.com/ymotongpoo/xk6-otel-gen/synth")
```

理由: OTel コレクタや bench での grouping/filtering を容易に。

### 7.2 Provider nil-handling

`NewDefault(tp, mp, lp)`:
- いずれか nil → panic (libraries should fail fast on programmer errors)
- nil 可能性は U5 caller 側が保証する責務

### 7.3 ノイズ防止

`Tracer.Start` / `Histogram.Record` / `Logger.Emit` の戻り値は無視する (戻り値 error がある場合のみ — OTel API はほぼ silent)。

---

## 8. Concurrency

- `defaultSynthesizer` は **stateless**: provider 参照のみを保持、可変フィールドなし
- 複数 goroutine から同じ `Synthesizer` を呼び出して race-free
- OTel SDK の Tracer / Meter / Logger は thread-safe (公式保証)
- `finishFunc` は **同じ goroutine** で呼ぶ必要は **ない** (span は thread-safe で End できる) が、span context の意味的整合のため通常は同じ goroutine 内で呼ぶ

---

## 9. パフォーマンスとリソース (FD 時点目安、NFR-R で確定)

| 項目 | 期待値 |
|---|---|
| `BeginSpan` 所要時間 | < 10 µs (semconv attribute build + span start) |
| `RecordMetric` 所要時間 | < 5 µs |
| `EmitLog` 所要時間 | < 10 µs |
| `BuildResource` 所要時間 | < 50 µs (UUID v5 計算込み) |
| `Synthesizer` インスタンスメモリ | < 1 KB (provider 参照 3 個のみ) |
| Per-span heap allocation | < 256 B (span 自体は SDK 側でアロケート) |

---

## 10. Out of Scope (U3 では扱わない)

- Provider 構築 (U4 の責務)
- Replica 選択 (U2 の責務)
- Fault injection の検出 (U2 の責務)
- k6 native metric 変換 (U6 の責務)
- 動的なエラー値生成 (Engine が semconv 準拠 string を渡す前提)
