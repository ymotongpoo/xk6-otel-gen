# U3 synth — Logical Components

本書は `synth/` 内の **論理コンポーネント** (LC) を確定する。各 LC について 責務 / 公開 API / 実装スケッチ / 依存関係 を定義。

参照:
- FD: `aidlc-docs/construction/u3-synth/functional-design/`
- NFR Design Patterns: `nfr-design-patterns.md` (本ディレクトリ内)

---

## コンポーネント一覧

| LC | 名前 | ファイル | 責務 |
|---|---|---|---|
| LC-0 | Package Documentation | `doc.go` | パッケージレベル GoDoc |
| LC-1 | Public Interface & Types | `interface.go` | Synthesizer interface, SpanInput / MetricInput / LogInput / Outcome / FinishSpanFunc |
| LC-2 | Resource Builder | `resource.go` | BuildResource + UUID v5 namespace + InstanceID helper |
| LC-3 | Attribute Policy & Builders | `attributes.go` | (ServiceKind, EdgeKind) → policy mapping + semconv key references + Static/Dynamic attribute builders + staticSetCache |
| LC-4 | Synthesizer Implementation | `synthesizer.go` | defaultSynthesizer struct + NewDefault + BeginSpan / RecordMetric / EmitLog + finishFunc + active_requests management |
| LC-5 | (omitted, errors.go) | — | NFR-R Q4-Q5 で panic 採用、独立 error 型不要のため省略 (Q13 案 B 相当) |

> **Note**: FD §3 では `errors.go` を含めて 6 production files としたが、NFR-D 検討で **error 型を持たない** (全 panic) ことが確定したため、`errors.go` は不要。実 production file は **5 つ**:
> `doc.go`, `interface.go`, `synthesizer.go`, `resource.go`, `attributes.go`
> + race build tag 用 `race_on.go` / `race_off.go` の 2 ファイル (`raceEnabled` 定数定義) を追加 → 計 **7 production files**

---

## LC-0: Package Documentation (`doc.go`)

### 責務
- パッケージ全体の GoDoc コメント
- 利用例の overview
- Lifecycle (NewDefault → BeginSpan → finish → exporter Shutdown)
- semconv version 明記

### 実装スケッチ
```go
// Package synth synthesizes OpenTelemetry spans, metrics, and log records
// for simulated services described by the topology package. It is intended
// to be driven by a Journey Engine (package journey) that supplies
// SpanInput / MetricInput / LogInput values per executed step.
//
// Typical usage:
//
//   tp := exporterPipeline.TracerProvider()
//   mp := exporterPipeline.MeterProvider()
//   lp := exporterPipeline.LoggerProvider()
//   syn := synth.NewDefault(tp, mp, lp)
//
//   ctx, finish := syn.BeginSpan(ctx, synth.SpanInput{...})
//   // ... journey executes children ...
//   finish(synth.Outcome{Success: true, EndTime: time.Now()})
//
// All attributes follow OpenTelemetry Semantic Conventions v1.27.0.
// See package attributes for the (Service.Kind, Edge.Kind) -> policy table.
package synth
```

### 依存
- なし

---

## LC-1: Public Interface & Types (`interface.go`)

### 責務
- `Synthesizer` interface 定義
- 入出力型 (`SpanInput`, `MetricInput`, `LogInput`, `Outcome`)
- `FinishSpanFunc` type alias

### 公開 API
```go
type Synthesizer interface {
    BeginSpan(ctx context.Context, in SpanInput) (context.Context, FinishSpanFunc)
    RecordMetric(ctx context.Context, in MetricInput)
    EmitLog(ctx context.Context, in LogInput)
}

type SpanInput struct {
    Service     *topology.Service
    Edge        *topology.Edge
    Operation   string
    StartTime   time.Time
    InstanceIdx int
}

type MetricInput struct {
    Service     *topology.Service
    Edge        *topology.Edge
    Operation   string
    Latency     time.Duration
    Outcome     Outcome
    InstanceIdx int
}

type LogInput struct {
    Service    *topology.Service
    Severity   log.Severity
    Body       string
    Attributes map[string]any
}

type Outcome struct {
    Success    bool
    StatusCode int
    ErrorType  string
    EndTime    time.Time
}

type FinishSpanFunc func(outcome Outcome)
```

### 不変条件
- すべての型は value type (`*topology.Service` / `*topology.Edge` のみ pointer)
- 全 exported identifier に GoDoc 必須

### 依存
- `context`
- `time`
- `go.opentelemetry.io/otel/log` (log.Severity 型のみ)
- `github.com/ymotongpoo/xk6-otel-gen/topology`

---

## LC-2: Resource Builder (`resource.go`)

### 責務
- `BuildResource(svc, instanceIdx) *resource.Resource` の実装
- UUID v5 namespace 定数 + `InstanceID` helper
- semconv 準拠 Resource attribute build

### 公開 API
```go
func BuildResource(svc *topology.Service, instanceIdx int) *resource.Resource
```

### 実装スケッチ
```go
// synthInstanceNamespace pins the UUID v5 namespace used to derive
// deterministic service.instance.id values.
var synthInstanceNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("xk6-otel-gen/synth"))

// InstanceID returns a deterministic UUID v5 identifier for the given
// (svcName, idx) pair. Exposed for testing; production code goes through
// BuildResource.
func InstanceID(svcName string, idx int) string {
    return uuid.NewSHA1(synthInstanceNamespace, []byte(svcName+"/"+strconv.Itoa(idx))).String()
}

func BuildResource(svc *topology.Service, instanceIdx int) *resource.Resource {
    if svc == nil {
        panic("synth: BuildResource: svc must not be nil")
    }
    if svc.Name == "" {
        panic("synth: BuildResource: svc.Name must not be empty")
    }
    if instanceIdx < 0 {
        panic(fmt.Sprintf("synth: BuildResource: instanceIdx %d must be >= 0", instanceIdx))
    }

    attrs := []attribute.KeyValue{
        semconv.ServiceName(svc.Name),
        semconv.ServiceInstanceID(InstanceID(svc.Name, instanceIdx)),
        semconv.TelemetrySDKName("opentelemetry"),
        semconv.TelemetrySDKLanguage("go"),
    }
    if svc.Version != "" {
        attrs = append(attrs, semconv.ServiceVersion(svc.Version))
    }
    if svc.Language != "" {
        // See attributes.go for the rationale of mapping svc.Language to
        // process.runtime.name in the synthesis context.
        attrs = append(attrs, semconv.ProcessRuntimeName(svc.Language))
    }
    if svc.Framework != "" {
        // Custom namespace (not in semconv); see attributes.go.
        attrs = append(attrs, attribute.String("synth.service.framework", svc.Framework))
    }
    return resource.NewSchemaless(attrs...)
}
```

### 依存
- `fmt`, `strconv`
- `github.com/google/uuid`
- `go.opentelemetry.io/otel/attribute`
- `go.opentelemetry.io/otel/sdk/resource`
- `go.opentelemetry.io/otel/semconv/v1.27.0`
- `github.com/ymotongpoo/xk6-otel-gen/topology`

---

## LC-3: Attribute Policy & Builders (`attributes.go`)

### 責務
- `(Service.Kind, Edge.Kind) → attributePolicy` マッピング
- Static attribute Set (svc.name, http.method 等の per-(svc, op, edge) 不変属性) の construction + cache
- Dynamic attribute (status code, error type) の construction
- semconv key constants の import 集約

### 内部型
```go
type direction uint8

const (
    dirUnset    direction = iota
    dirClient
    dirServer
    dirProducer
    dirConsumer
    dirInternal
)

type attributePolicy struct {
    SpanKind           trace.SpanKind
    AttributeNamespace string // "http" | "rpc" | "db" | "messaging" | ""
    MetricNamespace    string // same
    Direction          direction
}

// policyFor returns the policy for a given (ServiceKind, EdgeKind) pair.
// The Direction is determined by the caller's role relative to Edge (caller
// passes their own role; we don't infer from Edge.From/To here).
func policyFor(svcKind topology.ServiceKind, edgeKind topology.EdgeKind, dir direction) attributePolicy
```

### 公開 API
- なし (全 internal helper)。policy / set builder は `synthesizer.go` から呼び出される。

### staticSetCache の実装
```go
type cacheKey struct {
    svcName string
    op      string
    edgeID  string
    dir     direction
}

type staticSetCache struct {
    sets sync.Map
}

func (c *staticSetCache) get(k cacheKey) (attribute.Set, bool) {
    v, ok := c.sets.Load(k)
    if !ok { return attribute.Set{}, false }
    return v.(attribute.Set), true
}

func (c *staticSetCache) put(k cacheKey, set attribute.Set) {
    c.sets.Store(k, set)
}

func cacheKeyFor(svc *topology.Service, op string, edge *topology.Edge, dir direction) cacheKey {
    edgeID := ""
    if edge != nil {
        edgeID = edge.From.Name + "->" + edge.To.Name + "/" + string(edge.Kind)
    }
    return cacheKey{svcName: svc.Name, op: op, edgeID: edgeID, dir: dir}
}
```

### Static attribute builder per namespace
```go
func buildStaticSet(svc *topology.Service, op string, edge *topology.Edge, policy attributePolicy) attribute.Set {
    var kvs []attribute.KeyValue
    switch policy.AttributeNamespace {
    case "http":
        kvs = httpStaticAttrs(svc, op, edge, policy.Direction)
    case "rpc":
        kvs = rpcStaticAttrs(svc, op, edge, policy.Direction)
    case "db":
        kvs = dbStaticAttrs(svc, op, edge)
    case "messaging":
        kvs = messagingStaticAttrs(svc, op, edge, policy.Direction)
    }
    return attribute.NewSet(kvs...)
}

func httpStaticAttrs(svc *topology.Service, op string, edge *topology.Edge, dir direction) []attribute.KeyValue {
    method, route := parseHTTPOp(op) // e.g. "GET /api/users" -> ("GET", "/api/users")
    kvs := []attribute.KeyValue{
        semconv.HTTPRequestMethodKey.String(method),
    }
    if dir == dirServer {
        kvs = append(kvs, semconv.HTTPRouteKey.String(route))
    } else if dir == dirClient {
        if edge != nil {
            kvs = append(kvs, semconv.ServerAddressKey.String(edge.To.Name))
            if svc.Kind == topology.ServiceExternalAPI {
                kvs = append(kvs, attribute.String("peer.service", edge.To.Name))
            }
        }
    }
    return kvs
}
// rpcStaticAttrs / dbStaticAttrs / messagingStaticAttrs 同様
```

### Dynamic attribute builder
```go
func dynamicOutcomeAttrs(policy attributePolicy, outcome Outcome) []attribute.KeyValue {
    var kvs []attribute.KeyValue
    if outcome.StatusCode != 0 {
        switch policy.AttributeNamespace {
        case "http":
            kvs = append(kvs, semconv.HTTPResponseStatusCodeKey.Int(outcome.StatusCode))
        case "rpc":
            kvs = append(kvs, semconv.RPCGRPCStatusCodeKey.Int(outcome.StatusCode))
        }
    }
    if !outcome.Success && outcome.ErrorType != "" {
        kvs = append(kvs, semconv.ErrorTypeKey.String(outcome.ErrorType))
    }
    return kvs
}
```

### Allowed attribute keys (TP-U3-2)
```go
// allowedAttrKeys is the union of all semconv keys this package may emit.
// Used by TP-U3-2 to assert no surprise keys leak out.
var allowedAttrKeys = map[string]struct{}{
    string(semconv.HTTPRequestMethodKey):       {},
    string(semconv.HTTPResponseStatusCodeKey):  {},
    string(semconv.HTTPRouteKey):               {},
    string(semconv.ServerAddressKey):           {},
    string(semconv.ServerPortKey):              {},
    string(semconv.URLPathKey):                 {},
    string(semconv.RPCSystemKey):               {},
    string(semconv.RPCServiceKey):              {},
    string(semconv.RPCMethodKey):               {},
    string(semconv.RPCGRPCStatusCodeKey):       {},
    string(semconv.DBSystemKey):                {},
    string(semconv.DBOperationKey):             {},
    string(semconv.MessagingSystemKey):         {},
    string(semconv.MessagingOperationKey):      {},
    string(semconv.MessagingDestinationNameKey):{},
    string(semconv.ErrorTypeKey):               {},
    "peer.service":                             {},
    "outcome":                                  {},
    "synth.service.framework":                  {},
}
```

### 依存
- `sync`
- `go.opentelemetry.io/otel/attribute`
- `go.opentelemetry.io/otel/semconv/v1.27.0`
- `go.opentelemetry.io/otel/trace` (SpanKind)
- `github.com/ymotongpoo/xk6-otel-gen/topology`

---

## LC-4: Synthesizer Implementation (`synthesizer.go`)

### 責務
- `defaultSynthesizer` struct と `NewDefault` constructor
- `BeginSpan` / `RecordMetric` / `EmitLog` の実装
- `FinishSpanFunc` closure 生成 (atomic.Bool double-call protection)
- active_requests +1/-1 の管理 (BeginSpan / finishFunc)
- Log Body 自動生成テンプレ
- Status Code → Span Status マッピング

### 公開 API
```go
func NewDefault(tp trace.TracerProvider, mp metric.MeterProvider, lp log.LoggerProvider) Synthesizer
```

### defaultSynthesizer struct
```go
type defaultSynthesizer struct {
    tracer trace.Tracer
    meter  metric.Meter
    logger log.Logger

    httpClientDur metric.Float64Histogram
    httpServerDur metric.Float64Histogram
    httpActiveReq metric.Int64UpDownCounter
    rpcClientDur  metric.Float64Histogram
    rpcServerDur  metric.Float64Histogram
    rpcActiveReq  metric.Int64UpDownCounter
    dbClientDur   metric.Float64Histogram
    msgProducerDur metric.Float64Histogram
    msgConsumerDur metric.Float64Histogram

    staticSetCache *staticSetCache
}
```

### NewDefault 実装スケッチ
```go
func NewDefault(tp trace.TracerProvider, mp metric.MeterProvider, lp log.LoggerProvider) Synthesizer {
    if tp == nil { panic("synth: NewDefault: tp must not be nil") }
    if mp == nil { panic("synth: NewDefault: mp must not be nil") }
    if lp == nil { panic("synth: NewDefault: lp must not be nil") }

    const instrumentation = "github.com/ymotongpoo/xk6-otel-gen/synth"
    meter := mp.Meter(instrumentation)

    s := &defaultSynthesizer{
        tracer: tp.Tracer(instrumentation),
        meter:  meter,
        logger: lp.Logger(instrumentation),
        staticSetCache: &staticSetCache{},
    }

    s.httpClientDur = mustHistogram(meter, "http.client.request.duration", "s")
    s.httpServerDur = mustHistogram(meter, "http.server.request.duration", "s")
    s.httpActiveReq = mustUDC(meter, "http.server.active_requests", "{request}")
    s.rpcClientDur  = mustHistogram(meter, "rpc.client.duration", "s")
    s.rpcServerDur  = mustHistogram(meter, "rpc.server.duration", "s")
    s.rpcActiveReq  = mustUDC(meter, "rpc.server.active_requests", "{request}")
    s.dbClientDur   = mustHistogram(meter, "db.client.operation.duration", "s")
    s.msgProducerDur = mustHistogram(meter, "messaging.publish.duration", "s")
    s.msgConsumerDur = mustHistogram(meter, "messaging.receive.duration", "s")
    return s
}

func mustHistogram(m metric.Meter, name, unit string) metric.Float64Histogram {
    h, err := m.Float64Histogram(name, metric.WithUnit(unit))
    if err != nil {
        panic(fmt.Sprintf("synth: NewDefault: build %s: %v", name, err))
    }
    return h
}
// mustUDC 同様
```

### BeginSpan / finishFunc 実装スケッチ (詳細)

NFR Design Patterns §2.2 を参照。`atomic.Bool` + `raceEnabled` で double-call protection、active_requests +1/-1 を BeginSpan / finishFunc で対称。

### RecordMetric 実装スケッチ
NFR Design Patterns §1.2.2 を参照。hybrid static+dynamic strategy。

### EmitLog 実装スケッチ
```go
func (s *defaultSynthesizer) EmitLog(ctx context.Context, in LogInput) {
    if in.Service == nil { panic("synth: EmitLog: Service must not be nil") }

    sev := in.Severity
    if sev == log.SeverityUndefined {
        // No outcome context available here; caller (Engine) supplies Severity.
        // If absent, default to Info (Q10=A default body assumes success).
        sev = log.SeverityInfo
    }
    body := in.Body
    if body == "" {
        // Default body templates require knowing outcome. Caller responsibility
        // to pass Severity+Body together; if Body is empty here, use Service.Name only.
        body = in.Service.Name + " event"
    }

    record := log.Record{}
    now := time.Now()
    record.SetTimestamp(now)
    record.SetObservedTimestamp(now)
    record.SetSeverity(sev)
    record.SetBody(log.StringValue(body))
    for k, v := range in.Attributes {
        record.AddAttributes(log.KeyValue{Key: k, Value: toLogValue(v)})
    }
    // service.name auto-attribute
    record.AddAttributes(log.KeyValue{
        Key:   string(semconv.ServiceNameKey),
        Value: log.StringValue(in.Service.Name),
    })
    s.logger.Emit(ctx, record)
}
```

### 依存
- `context`, `fmt`, `sync/atomic`, `time`
- `go.opentelemetry.io/otel/attribute`
- `go.opentelemetry.io/otel/log`
- `go.opentelemetry.io/otel/metric`
- `go.opentelemetry.io/otel/trace`
- `go.opentelemetry.io/otel/semconv/v1.27.0`
- `github.com/ymotongpoo/xk6-otel-gen/topology`

---

## LC-5 (omitted)

`errors.go` は省略 (`Q13=A` の FD 提案から逸脱)。

### 削除判断の精査

「panic を採用したから error 型不要」という短絡を避け、`synth` が error 値を **どこに流すか** を経路ごとに確認:

| 経路 | error 値の出口 | 判定 |
|---|---|---|
| 公開 method の戻り値 | `NewDefault`, `BuildResource`, `BeginSpan`, `RecordMetric`, `EmitLog`, `FinishSpanFunc` いずれも error を返さない | 不要 |
| callback / closure | `FinishSpanFunc(outcome)` は戻り値なし、他 callback なし | 不要 |
| panic payload | NFR-R Q4-Q6 で fail-fast 設計、caller は recover しない前提 → typed payload にしても識別場面なし | 不要 |
| internal error wrap | `meter.Float64Histogram` 等の SDK error は `mustHistogram` で panic 化、外部に漏れない | 不要 |
| OTel global error handler | `otel.SetErrorHandler` は SDK 内部 error 用、`synth` は使わない | 経路外 |

→ caller が `errors.As(err, &synthErr)` で `synth` 製 error を識別する場面が **存在しない** ため、`errors.go` を持つメリットがない。

#### 「panic 採用」と「error 型不要」は別判断

- **panic 採用** (NFR-R Q4-Q6): 戻り値 signature を error なしに保つ判断 (`RecordMetric` の signature 等)
- **error 型不要** (本節): caller が型階層で error を識別する場面の有無

両者は独立。U3 では偶然どちらも "no errors.go" を支持するが、**理由付けは別**。例えば仮に `RecordMetric` が `(error)` を返す設計 (panic 不採用) であっても、caller が型 assertion を必要としない汎用 error で済むなら `errors.go` は不要。

### 将来 errors.go が必要になる兆候

- 公開 method の signature に `error` 戻り値が追加された場合 (e.g., `RecordMetric` が SDK 制約で error を返す必要が出た)
- caller (U2 Engine 等) が `synth` 特有の失敗種別を識別する必要が出た (e.g., "metric build failed" vs "log emit failed" を区別)
- panic を recover する layer を導入し、payload 種別で動作を変える設計に転換する場合

これらいずれも現時点で発生しない。NFR-R / FD の決定が変わったときに再評価。

実 production file:
```text
synth/
├── doc.go               (LC-0)
├── interface.go         (LC-1)
├── resource.go          (LC-2)
├── attributes.go        (LC-3)
├── synthesizer.go       (LC-4)
├── race_on.go           //go:build race  — const raceEnabled = true
└── race_off.go          //go:build !race — const raceEnabled = false
```

= **7 production files** (FD §3 の 6 から `errors.go` を削除、`race_on/off.go` を追加で +1 → +0 net、ただし race tag による分割で実質ファイル数 7)

> **NOTE**: FD §3 と乖離する変更なので、Code Generation Plan 作成時に再確認が必要。FD 改訂を出すか、NFR-D での deviation として留めるか、ユーザーに確認。

---

## コンポーネント間依存図

```text
              ┌──────────────────┐
              │ LC-0 doc.go      │
              └──────────────────┘
                       (no deps)

              ┌──────────────────┐
              │ LC-1 interface.go│
              │ - Synthesizer    │
              │ - SpanInput      │
              │ - MetricInput    │
              │ - LogInput       │
              │ - Outcome        │
              │ - FinishSpanFunc │
              └────────┬─────────┘
                       │
       ┌───────────────┴────────────────┐
       │                                │
       ▼                                ▼
┌──────────────────┐          ┌──────────────────┐
│ LC-2 resource.go │          │ LC-3 attributes  │
│ - BuildResource  │          │   .go            │
│ - InstanceID     │          │ - policyFor      │
│ - synthInstance- │          │ - staticSetCache │
│   Namespace      │          │ - buildStaticSet │
└──────────────────┘          │ - dynamic attrs  │
                               │ - allowed keys   │
                               └────────┬─────────┘
                                        │
                                        ▼
                               ┌──────────────────────────────┐
                               │ LC-4 synthesizer.go          │
                               │ - defaultSynthesizer struct  │
                               │ - NewDefault                 │
                               │ - BeginSpan / FinishSpanFunc │
                               │ - RecordMetric               │
                               │ - EmitLog                    │
                               │ - active_requests +/-1       │
                               └──────────────────────────────┘
                                        │
                                        ▼
                                 race_on.go / race_off.go
                                 (raceEnabled const)
```

---

## ビルド時の依存外部パッケージ

| 用途 | パッケージ |
|---|---|
| Trace interface | `go.opentelemetry.io/otel/trace` |
| Metric interface | `go.opentelemetry.io/otel/metric` |
| Log interface | `go.opentelemetry.io/otel/log` |
| Resource type | `go.opentelemetry.io/otel/sdk/resource` |
| KeyValue / Set | `go.opentelemetry.io/otel/attribute` |
| Semconv constants | `go.opentelemetry.io/otel/semconv/v1.27.0` |
| UUID v5 | `github.com/google/uuid` |
| Topology types | `github.com/ymotongpoo/xk6-otel-gen/topology` |

**Excluded**: `propagation`, `baggage`, `sdk/trace`, `sdk/metric`, `sdk/log`, exporters, semconv の他 version.

---

## テストコンポーネント (Code Generation 時に詳細化)

| テストファイル | LC 対象 | テスト形式 |
|---|---|---|
| `interface_test.go` | LC-1 | example-based 入力 validate |
| `resource_test.go` | LC-2 | example-based + TP-U3-1 (Idempotency) |
| `attributes_test.go` | LC-3 | policy mapping table + TP-U3-2 (Allowed Keys) |
| `synthesizer_test.go` | LC-4 | example-based with mock providers (helpers_test.go) |
| `pbt_test.go` | LC-3, LC-4 | TP-U3-3 (Histogram Insertion), TP-U3-4 (error.type Required) |
| `helpers_test.go` | (全 LC 共通) | newTestProviders 等の test helper |
| `doc_test.go` | LC-0..LC-4 | 3 Example functions |
| `bench_test.go` | LC-2, LC-4 | BenchmarkBeginSpan / RecordMetric / EmitLog / BuildResource |
| `integration/integration_test.go` | LC-2..LC-4 全体 | `//go:build integration`、Docker Collector + 3-signal correlation |

`helpers_test.go` 自体は LC-X (LC として番号付けしない、テスト共通) として扱う。

---

## まとめ

- 7 production files (race build tag 含む)
- 1 LC omitted (`errors.go`) — `panic(string)` で error 型不要
- 各コンポーネントは Single Responsibility
- 依存関係は単方向 (LC-0/1 → LC-2 / LC-3 → LC-4)
- 公開 API は FD §4 で確定済の最小セット
- すべての設計判断が `nfr-design-patterns.md` の 13 質問回答および NFR-R Open Questions §7 解消と整合
- **NFR-R Open Question の `errors.go` 削除** は FD §3 / domain-entities.md §3 を **NFR Design 側で更新** する形式的逸脱 — Code Generation Plan 時にユーザー確認推奨
