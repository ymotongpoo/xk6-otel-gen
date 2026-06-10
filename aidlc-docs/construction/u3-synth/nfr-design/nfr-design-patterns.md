# U3 synth — NFR Design Patterns

本書は U3 (`synth/`) の **「どう実装するか」** のパターン群を確定する。FD (`what`) + NFR-R (`what to achieve`) を受けて、Performance / Concurrency / Error / API / Documentation / Test の各カテゴリで実装パターンを決める。

参照:
- FD: `aidlc-docs/construction/u3-synth/functional-design/`
- NFR-R: `aidlc-docs/construction/u3-synth/nfr-requirements/`
- Plan + Answers: `aidlc-docs/construction/plans/u3-synth-nfr-d-plan.md`

---

## 1. Performance パターン

### 1.1 Instrument の物理配置 (Q1=A)

`defaultSynthesizer` 構造体内に named field として 9 個の Histogram / UpDownCounter を配置:

```go
type defaultSynthesizer struct {
    tracer trace.Tracer
    logger log.Logger
    meter  metric.Meter   // 保持しなくても良いが debug 用

    // HTTP namespace
    httpClientDur metric.Float64Histogram
    httpServerDur metric.Float64Histogram
    httpActiveReq metric.Int64UpDownCounter

    // RPC namespace
    rpcClientDur metric.Float64Histogram
    rpcServerDur metric.Float64Histogram
    rpcActiveReq metric.Int64UpDownCounter

    // DB namespace (Client only)
    dbClientDur metric.Float64Histogram

    // Messaging namespace (Producer / Consumer)
    msgProducerDur metric.Float64Histogram
    msgConsumerDur metric.Float64Histogram

    // Pre-computed static attribute Sets (Q2 hybrid 戦略)
    staticSetCache *staticSetCache
}
```

理由:
- map lookup コスト不要
- IDE で全 instrument が一覧可能
- 動的追加要件なし (namespace は固定 4 種)

### 1.2 Attribute build と allocation 戦略 (Q2 = A + C hybrid)

**重要訂正**: OTel Go SDK は `attribute.NewSet` を intern しない。per-call で sort + dedupe + memory allocation が発生する。`attribute.Set` 値は semantically 等価でも別アロケーション。

NFR-U3-6 の latency target (BeginSpan <10µs / RecordMetric <5µs) を **確実に** 達成するため、**hybrid 戦略** を採用:

```text
Attribute = Static (per Service+Edge+Operation で固定値) + Dynamic (Outcome に依存)

Static  -- Precompute された attribute.Set として cache
          (svc.name, http.route, http.method, rpc.system, db.system, etc.)

Dynamic -- per-call の KeyValue slice として build
          (http.response.status_code, error.type, rpc.grpc.status_code, etc.)
```

#### 1.2.1 staticSetCache の構造

```go
// staticSetCache caches precomputed attribute.Set values keyed by the
// (Service, Operation, Edge) tuple that determines a span's static
// attributes. Lookups are O(1) via sync.Map and zero-allocation on hit.
type staticSetCache struct {
    sets sync.Map // key: cacheKey, value: attribute.Set
}

type cacheKey struct {
    svcName string
    op      string
    edgeID  string // Edge.From + "->" + Edge.To + "/" + Edge.Kind; "" for nil edge
    dir     direction // client | server | producer | consumer | internal
}

func (c *staticSetCache) get(key cacheKey) (attribute.Set, bool) { ... }
func (c *staticSetCache) put(key cacheKey, set attribute.Set)     { ... }
```

#### 1.2.2 RecordMetric の hot path

```go
func (s *defaultSynthesizer) RecordMetric(ctx context.Context, in MetricInput) {
    // validate (panic on programmer error per NFR-U3-4)
    if in.Service == nil { panic("synth: RecordMetric: Service must not be nil") }

    policy := policyFor(in.Service.Kind, edgeKind(in.Edge))
    hist := s.histogramFor(policy)
    if hist == nil { return } // namespace=="" (internal); no metric

    // 1. Static attribute set (cached)
    key := cacheKeyFor(in, policy)
    staticSet, ok := s.staticSetCache.get(key)
    if !ok {
        staticSet = buildStaticSet(in, policy)
        s.staticSetCache.put(key, staticSet)
    }

    // 2. Dynamic attributes (per-call)
    dynamic := make([]attribute.KeyValue, 0, 2)
    if in.Outcome.StatusCode != 0 {
        dynamic = append(dynamic, statusCodeAttr(policy, in.Outcome.StatusCode))
    }
    if !in.Outcome.Success && in.Outcome.ErrorType != "" {
        dynamic = append(dynamic, semconv.ErrorTypeKey.String(in.Outcome.ErrorType))
    }

    // 3. Record with both
    hist.Record(ctx, in.Latency.Seconds(),
        metric.WithAttributeSet(staticSet),
        metric.WithAttributes(dynamic...),
    )
}
```

#### 1.2.3 BeginSpan の同様パターン

Span attributes も同じく static+dynamic に分割し、`trace.WithAttributes` で **set + slice 両方** を渡せないため `[]attribute.KeyValue` を一度組み立てる。Span の場合、SDK 内部で Set 変換が起きるが span 単体のアロケーションコスト (>1KB) に比べれば誤差。

```go
attrs := make([]attribute.KeyValue, 0, staticSet.Len()+2)
staticSet.ToSlice() を append → dynamic を append
tracer.Start(ctx, name, trace.WithAttributes(attrs...), ...)
```

#### 1.2.4 Bench で target 未達なら更に対応

`BenchmarkRecordMetric` で 5µs 超えた場合の fall back 順:
1. `sync.Pool[*[]attribute.KeyValue]` で dynamic slice 再利用
2. cacheKey の文字列構築を avoid (構造体直接比較)
3. dynamic attrs が空のときは `metric.WithAttributes` 呼び出しを skip

### 1.3 Resource cache (Q3=A)

`BuildResource` は **cache しない**。毎回 UUID v5 計算 + attribute slice 構築。

- 計測目安: `BuildResource < 50 µs` (NFR-U3-6)
- Typical k6 ワークロード: VU=1000 × Service=10 → 起動時 10,000 回呼ばれる、その後はゼロ
- Hot path ではない、cache の lifecycle 管理コストが上回る

caller (U2 Engine) が必要なら caller 側で per-VU cache を持つ責務 (Q3=A 決定理由)。

### 1.4 Instrument の eager 生成 (NFR-R Q7=A)

`NewDefault` 内で 9 個全 instrument を構築。失敗時は panic:

```go
func NewDefault(tp trace.TracerProvider, mp metric.MeterProvider, lp log.LoggerProvider) Synthesizer {
    if tp == nil { panic("synth: NewDefault: tp must not be nil") }
    if mp == nil { panic("synth: NewDefault: mp must not be nil") }
    if lp == nil { panic("synth: NewDefault: lp must not be nil") }

    instrumentation := "github.com/ymotongpoo/xk6-otel-gen/synth"
    meter := mp.Meter(instrumentation)

    s := &defaultSynthesizer{
        tracer: tp.Tracer(instrumentation),
        meter:  meter,
        logger: lp.Logger(instrumentation),
        staticSetCache: &staticSetCache{},
    }

    var err error
    s.httpClientDur, err = meter.Float64Histogram("http.client.request.duration", metric.WithUnit("s"))
    if err != nil { panic(fmt.Sprintf("synth: NewDefault: build http.client.request.duration: %v", err)) }
    // ... similarly for 8 others
    return s
}
```

理由:
- 9 個程度のメモリは誤差 (< 10 KB)
- Hot path 上での `meter.Histogram(name)` lookup を回避
- 起動時 fail-fast (NFR-U3-4)

### 1.5 Histogram bucket (Q9=A)

SDK default bucket を採用。explicit な `WithExplicitBucketBoundaries` 指定なし。

将来 bench 計測後、k6 ワークロード fit が悪ければ semconv 推奨値 (`[0.005, 0.01, 0.025, ..., 10]` 秒) に変更検討。

---

## 2. Concurrency パターン

### 2.1 Stateless defaultSynthesizer (NFR-U3-3)

`defaultSynthesizer` 構造体に **可変フィールドなし**:
- 全 instrument は provider から取得した interface (SDK が thread-safe)
- `staticSetCache` の `sync.Map` は thread-safe (concurrent Load/Store safe)
- それ以外のフィールドは構築時に決定、以後 read-only

→ 複数 goroutine から同じ `Synthesizer` を呼び出して race-free。

### 2.2 finishFunc closure と double-call protection (Q5=A)

```go
func (s *defaultSynthesizer) BeginSpan(ctx context.Context, in SpanInput) (context.Context, FinishSpanFunc) {
    // validate
    if in.Service == nil { panic("synth: BeginSpan: Service must not be nil") }
    if in.Operation == "" { panic("synth: BeginSpan: Operation must not be empty") }
    if in.InstanceIdx < 0 || in.InstanceIdx >= in.Service.Replicas {
        panic(fmt.Sprintf("synth: BeginSpan: InstanceIdx %d out of range [0, %d)",
            in.InstanceIdx, in.Service.Replicas))
    }

    policy := policyFor(in.Service.Kind, edgeKind(in.Edge))

    // build span attrs (static + initial-state dynamic)
    spanAttrs := buildSpanAttributes(in, policy)
    spanName := in.Service.Name + "." + in.Operation

    ctx2, span := s.tracer.Start(ctx, spanName,
        trace.WithTimestamp(in.StartTime),
        trace.WithSpanKind(policy.SpanKind),
        trace.WithAttributes(spanAttrs...),
    )

    // active_requests +1 (Q6=A)
    s.maybeIncActive(ctx, in, policy, +1)

    var finished atomic.Bool
    return ctx2, func(outcome Outcome) {
        if !finished.CompareAndSwap(false, true) {
            // Q6=A: silent in production, panic in -race build (race detector)
            if raceEnabled {
                panic("synth: FinishSpanFunc called more than once")
            }
            return
        }
        // SetStatus (Q7=A semconv)
        span.SetStatus(statusFor(in, outcome))
        // SetAttributes (dynamic outcome attrs)
        span.SetAttributes(finishAttrs(in, outcome, policy)...)
        // End
        span.End(trace.WithTimestamp(outcome.EndTime))
        // active_requests -1
        s.maybeIncActive(ctx, in, policy, -1)
    }
}
```

`raceEnabled` は build tag で切り替え:
```go
// race_on.go     //go:build race
const raceEnabled = true

// race_off.go    //go:build !race
const raceEnabled = false
```

### 2.3 active_requests の +1/-1 (Q6=A)

```go
func (s *defaultSynthesizer) maybeIncActive(ctx context.Context, in SpanInput, policy attributePolicy, delta int64) {
    udc := s.activeUDC(policy)  // returns nil if no active gauge for this namespace
    if udc == nil { return }
    if policy.SpanKind != trace.SpanKindServer && policy.SpanKind != trace.SpanKindConsumer { return }

    udc.Add(ctx, delta, metric.WithAttributeSet(activeSet(in, policy)))
}
```

`activeSet` は `(method, route)` のみの static set として cache 可能。

---

## 3. Error パターン

### 3.1 Panic message フォーマット (Q7=A)

統一フォーマット:
```text
synth: <method>: <field/condition>[ = <value>]: <reason>
```

例:
- `"synth: NewDefault: tp must not be nil"`
- `"synth: BeginSpan: Service must not be nil"`
- `"synth: BeginSpan: InstanceIdx 5 out of range [0, 3)"`
- `"synth: RecordMetric: Service.Replicas == 0 (cannot derive instance.id)"`
- `"synth: BuildResource: svc must not be nil"`

`fmt.Sprintf` でフォーマット、`panic(string)` で raise。grep しやすさ + recover 不要のシンプル運用。

### 3.2 finishFunc 重複呼び出し protection

§2.2 で `atomic.Bool` + `raceEnabled` を使用 (Q5=A)。

### 3.3 NewDefault の instrument 構築失敗時

`meter.Float64Histogram` が error を返した場合 (OTel SDK では通常起きないが API 上 error 型) → panic。NewDefault は programmer error / SDK 内部障害として fail-fast。

---

## 4. API パターン

### 4.1 9 Instrument の named field (Q1=A)

§1.1 で確定。

### 4.2 UUID v5 namespace の pinning (Q4=A)

```go
// synthInstanceNamespace is the UUID v5 namespace used to generate
// service.instance.id values from (Service.Name, instanceIdx) pairs.
// Pinned to the SHA1 of "xk6-otel-gen/synth" under uuid.NameSpaceDNS.
// Computed once at package init and stored as a constant string.
var synthInstanceNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("xk6-otel-gen/synth"))

func InstanceID(svcName string, idx int) string {
    return uuid.NewSHA1(synthInstanceNamespace, []byte(svcName+"/"+strconv.Itoa(idx))).String()
}
```

namespace UUID 自体の値は実装時に `go run` で計算してコメントに記録 (debug 用)。

### 4.3 process.runtime.name semantic (Q11=A)

`svc.Language` → `process.runtime.name` で確定。`attributes.go` のコメントで明示:

```go
// We map topology.Service.Language -> process.runtime.name (semconv).
// The semconv definition originally describes the runtime that the SDK
// itself runs in. For synthesized services, we reinterpret this as
// "the runtime the simulated service would run in" — keeping the
// attribute name semconv-compliant while allowing per-service breakdown
// by language in downstream dashboards.
```

### 4.4 Log Body 自動生成テンプレート (Q10=A)

```go
func defaultBody(in LogInput, success bool, errType string) string {
    op := derivedOperation(in)
    if success {
        return in.Service.Name + "." + op + " succeeded"
    }
    if errType == "" {
        return in.Service.Name + "." + op + " failed"
    }
    return in.Service.Name + "." + op + " failed: " + errType
}
```

`LogInput.Body != ""` の場合は caller 提供値を尊重。

### 4.5 公開 API surface (FD §4 / domain-entities.md 参照)

NFR Design で新規 export は **追加しない**。FD で確定した最小セットのみ。

---

## 5. Documentation パターン

### 5.1 3 Example function (NFR-R Q10=A)

`synth/doc_test.go`:

```go
// ExampleNewDefault demonstrates building a Synthesizer from injected
// OTel SDK providers.
func ExampleNewDefault() {
    tp := sdktrace.NewTracerProvider()
    mp := sdkmetric.NewMeterProvider()
    lp := sdklog.NewLoggerProvider()
    syn := synth.NewDefault(tp, mp, lp)
    _ = syn
    // Output:
}

// ExampleBuildResource shows constructing a per-instance Resource.
func ExampleBuildResource() {
    svc := &topology.Service{Name: "checkout", Version: "1.2.3", Replicas: 2, Language: "go"}
    res := synth.BuildResource(svc, 0)
    fmt.Println(res.Attributes()) // service.name=checkout, service.instance.id=<uuid>, ...
}

// ExampleSynthesizer_BeginSpan illustrates the standard span lifecycle.
func ExampleSynthesizer_BeginSpan() {
    syn := makeSynthesizer()
    svc := &topology.Service{Name: "frontend", Replicas: 1, Kind: topology.ServiceApplication}
    ctx, finish := syn.BeginSpan(context.Background(), synth.SpanInput{
        Service: svc, Operation: "GET /api", StartTime: time.Now(),
    })
    // ... do work ...
    finish(synth.Outcome{Success: true, StatusCode: 200, EndTime: time.Now()})
    _ = ctx
}
```

### 5.2 doc.go の structure

- Package overview
- Quick start
- Lifecycle (NewDefault → BeginSpan → finish → Shutdown via U4)
- semconv version note
- Attribute keys hint

### 5.3 GoDoc 網羅性

全 exported identifier に doc comment。CI で `revive` enforce。

---

## 6. Test パターン

### 6.1 Helper の集約 (Q8=A)

```text
synth/
└── helpers_test.go   // newTestProviders, makeService, makeSpanInput, etc.
```

```go
// newTestProviders builds in-memory OTel SDK providers backed by official
// tracetest / metricdata / logtest exporters.
func newTestProviders(t *testing.T) (
    trace.TracerProvider, metric.MeterProvider, log.LoggerProvider,
    *tracetest.InMemoryExporter,
    *sdkmetric.ManualReader,
    *logRecorder,
) {
    spanExporter := tracetest.NewInMemoryExporter()
    tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))

    reader := sdkmetric.NewManualReader()
    mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

    recorder := &logRecorder{}
    lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(recorder)))

    t.Cleanup(func() {
        _ = tp.Shutdown(context.Background())
        _ = mp.Shutdown(context.Background())
        _ = lp.Shutdown(context.Background())
    })

    return tp, mp, lp, spanExporter, reader, recorder
}
```

`logRecorder` は SDK にまだ in-memory log exporter がない場合のみ自前で `sdklog.Exporter` interface 実装。

### 6.2 PBT (TP-U3-1〜4) は `pbt_test.go` に集約

- TP-U3-1 BuildResource Idempotency: `rapid.Check` で同じ (svc, idx) 入力 → resource.Equal
- TP-U3-2 Allowed Attribute Keys: span 生成 → keys ⊆ allowedSet
- TP-U3-3 Histogram Bucket Insertion: RecordMetric → manualReader.Collect で count+1
- TP-U3-4 Error.Type Required on Failure: finish(outcome{Success:false}) → attrs に error.type

### 6.3 Integration test (Q12=A)

```text
synth/integration/
├── docker-compose.yaml      // U4 と同じ Collector image を pin
├── collector-config.yaml    // file_exporter で /var/log/otel/{traces,metrics,logs}.json
├── helpers.go               // StartCollector / ReadXxx / BuildPipeline (uses exporter pkg)
└── integration_test.go      //go:build integration
```

`TestIntegration_SynthToCollector_Correlated` で 3 信号 trace_id correlation を確認 (U4 と同パターン)。

### 6.4 active_requests の stateful PBT (任意)

NFR-R NFR-U3-12 PBT-06 の候補:
- BeginSpan / finish の random sequence で active_requests の値が常に >= 0 かつ全 finish 後に 0 に戻る
- 実装時に難易度を見て採用判断 (現時点で「optional」扱い)

---

## 7. NFR-R Open Questions の解消

| Open Question | 確定 |
|---|---|
| UUID v5 namespace 固定値 | §4.2 — `uuid.NewSHA1(uuid.NameSpaceDNS, []byte("xk6-otel-gen/synth"))` |
| `process.runtime.name=svc.Language` semantic | §4.3 — そのまま採用、コメントで再解釈を明記 |
| Histogram bucket boundaries | §1.5 — SDK default、bench 後に再評価 |
| Log Body 自動生成テンプレート | §4.4 — `"{svc}.{op} (succeeded|failed[: errType])"` |
| Stateful PBT (PBT-06) `active_requests` 平衡 | §6.4 — 実装時に難易度判断 (optional) |

---

## 8. NFR-R 各項目との対応

| NFR-R | 対応する Design パターン |
|---|---|
| NFR-U3-1 (API stability) | §4.5 公開 API 最小性、§4.1 named fields は internal |
| NFR-U3-2 (Lifecycle) | §1.4 NewDefault 1-shot 構築 |
| NFR-U3-3 (Concurrency) | §2.1 stateless + sync.Map cache |
| NFR-U3-4 (Error contract) | §3 全般 |
| NFR-U3-5 (Resource determinism) | §4.2 UUID v5 namespace pinning |
| NFR-U3-6 (Performance) | §1.2 hybrid attribute strategy、§1.4 eager instruments、§1.5 SDK default bucket |
| NFR-U3-7 (Observability) | self-metric なし (struct に counter フィールドなし、§1.1 確認) |
| NFR-U3-8 (Semconv conformance) | §4.3 process.runtime.name 解釈、PBT TP-U3-2 |
| NFR-U3-9 (Documentation) | §5 Example + GoDoc + doc.go |
| NFR-U3-10 (Testability) | §6 helper + PBT + integration |
| NFR-U3-11 (Compatibility) | semconv/v1.27.0 import、UUID v5 namespace pinning |
| NFR-U3-12 (PBT compliance) | §6.2 / §6.4 |

---

## 9. Anti-patterns (採用しない)

| アンチパターン | 不採用理由 |
|---|---|
| `map[string]metric.Float64Histogram` で動的 dispatch (Q1 案 B) | namespace 拡張要件なし、lookup コスト + race 管理が無駄 |
| `sync.Pool` で attribute slice 再利用 (Q2 案 B 単独) | bench で必要性確認前に複雑化、まず hybrid strategy を試す |
| 全 attribute set を `(svc, op, edge, outcome)` まで cache (Q2 案 C 単独) | Outcome の組み合わせ爆発で cache が肥大化 |
| Resource を cache (Q3 案 B) | BuildResource は hot path ではない |
| `service.instance.id` を `uuid.New()` (random) | 再現不可、test 困難 |
| `sync.Once` で finishFunc 保護 (Q5 案 B) | `atomic.Bool` で十分、`sync.Once` は重い |
| `MarkActive` / `MarkInactive` 別 API (Q6 案 C) | API surface 増、誤用リスク |
| `error` 型を panic に渡す (Q7 案 B) | string panic で grep 容易、recover 不要 |
| Per-test ファイルで mock provider 重複定義 (Q8 案 B) | DRY 違反 |
| Histogram bucket を最初から explicit 指定 (Q9 案 B/C) | NFR-R で「bench 後再評価」と明記済 |
| `synth.simulated.runtime` custom namespace (Q11 案 B) | semconv 準拠 + 再解釈で十分 |
| Integration test を U4 に併載 (Q12 案 B) | U3 単独の checking が困難 |
| `synthesizer.go` を信号別 3 ファイル分割 (Q13 案 C) | 共通の policy lookup が重複 |

---

## 10. 設計確定 — Open Items なし

13 質問すべて確定。NFR-R Open Questions も §7 で解消。`logical-components.md` で各論理コンポーネントの詳細を確定する。
