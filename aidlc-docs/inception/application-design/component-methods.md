# Component Methods — xk6-otel-gen

各コンポーネントの **公開メソッドシグネチャ** を Go 風疑似コードで示します。詳細な業務ルールは Construction フェーズの Functional Design で確定します。

---

## C1 — `topology`

### 設計メモ: 2-pass parse with resolved pointer references

YAML 内では service / edge / step は **名前文字列** で参照しあう。Go 公開型では `*Service` / `*Edge` / `*Step` の **解決済みポインタ** を保持する。`Parse()` が以下の 2 パスで構築する:

1. **Pass 1**: YAML を非公開の `rawSchema` 中間型にデコード (純粋に string 参照のみ持つ)
2. **Pass 2**: `Service` インスタンスを生成して `Schema.Services` マップに登録し、すべての string 参照を `*Service` / `*Edge` に解決。未解決参照があれば行番号付きエラーで停止

これによりタイポは Parse 時 (Validate 前) に拾え、IDE の rename/usages 検索が効き、ジャーニー実行ロジックが nil チェックや lookup を省略できる。

YAML への逆変換は `MarshalYAML` で `*Service` → service name string に書き戻す。PBT-02 ラウンドトリップでは `topology.Equal(a, b)` (識別子ベース等価) を使う (`reflect.DeepEqual` は循環参照のため不適)。

```go
package topology

import (
    "io"
)

// ServiceID is a newtype for service name identifiers. It exists to:
//   - prevent accidental mixing of service names with arbitrary strings
//   - serve as the map key type for Schema.Services
//   - leave room for future namespacing (e.g., "<namespace>/<name>" multi-file support)
type ServiceID string

// Schema represents the parsed and resolved root of a topology YAML file.
// All cross-references inside Schema are resolved pointers (never strings).
type Schema struct {
    Services map[ServiceID]*Service
    Journeys map[string]*Journey
    Faults   []FaultSpec
}

type Service struct {
    Name       ServiceID
    Kind       ServiceKind   // application | database | external_api | cache | queue
    Replicas   int
    Language   string
    Framework  string
    Version    string
    Operations map[string]*Operation   // keyed by operation name; each operation owns its outgoing calls
}

// Operation is a callable unit at a Service (HTTP endpoint / RPC method /
// message topic). Operations are the first-class concept: edges live inside
// operations, not directly on services. A journey step references an
// operation; the engine then traverses that operation's Calls tree.
type Operation struct {
    Name    string          // unique within its owning Service
    Service *Service        // back-pointer (populated by Parse)
    Calls   []*CallNode     // sequential calls (with optional Parallel groups)
}

// CallNode is either a single outgoing Edge or a Parallel group of CallNodes.
// Exactly one field is populated per node.
type CallNode struct {
    Edge     *Edge           // a single call to another operation
    Parallel []*CallNode     // fan-out group (children run concurrently, then join)
}

// Edge is a directed call from one operation to another.
// After Parse, From and To are always non-nil resolved pointers.
type Edge struct {
    From         *Operation   // owning operation (back-pointer; populated by Parse)
    To           *Operation   // target operation (resolved by Parse, guaranteed non-nil)
    Protocol     Protocol     // http | grpc | messaging
    Latency      LatencyDist  // distribution parameters
    ErrorRate    float64
    Timeout      Duration
    Retries      int
    RetryBackoff BackoffPolicy
    OnFailure    *RecoveryPolicy   // optional — cache-aside / fallback / default-response
}

// RecoveryPolicy describes what happens when an edge call fails.
// Fallback edges are tried in order; each one is a real call that emits
// its own span (and its own metrics/logs). If all fallbacks also fail,
// OnExhausted decides whether to propagate the failure (cascade) or
// to terminate the recovery flow with a synthesized "successful" outcome.
type RecoveryPolicy struct {
    Fallback        []*Edge            // ordered chain of alternative calls
    OnExhausted     ExhaustedAction    // propagate | return_default | succeed_silently
    DefaultResponse map[string]any     // used when OnExhausted == return_default
}

type ExhaustedAction int
const (
    ExhaustedPropagate    ExhaustedAction = iota   // cascade to downstream (status=error)
    ExhaustedReturnDefault                         // success outcome with synthesized default
    ExhaustedSucceedSilently                       // success outcome with no extra payload
)

type Journey struct {
    Name   string         // journey identifier (could become JourneyID newtype later)
    Steps  []*Step        // sequence of operation invocations (each step kicks off an operation tree)
    Weight float64        // weighted selection across journeys (defaults to 1.0)
}

// Step is one operation invocation in a journey.
// When executed, the Journey Engine starts at Op and traverses Op.Calls
// recursively (following Edge.To.Calls for each call).
// Parallel, if non-empty, makes the step itself a fan-out group of steps
// (rare — most fan-out lives within Operation.Calls, but available here too).
type Step struct {
    Op       *Operation    // resolved by Parse (never nil after Parse, unless Parallel is set)
    Parallel []*Step       // optional fan-out group at the journey level
}

// FaultTarget identifies what a fault is attached to.
// Exactly one of Service / Operation / Edge is non-nil after Parse (resolved
// from the YAML "node:<name>" / "operation:<svc>.<op>" / "edge:<from>-><to>" spec).
type FaultTarget struct {
    Kind      TargetKind   // TargetNode | TargetOperation | TargetEdge
    Service   *Service     // populated when Kind == TargetNode
    Operation *Operation   // populated when Kind == TargetOperation
    Edge      *Edge        // populated when Kind == TargetEdge
}

type FaultSpec struct {
    Target      FaultTarget
    Kind        FaultKind     // latency_inflation | error_rate_override | disconnect | crash
    Severity    SeverityParams
    // NOTE: cascade is NOT pre-computed here. The Journey Engine resolves
    // cascade at runtime based on whether the edge's RecoveryPolicy is
    // exhausted with OnExhausted=propagate. This makes cascading conditional
    // on the actual presence and success of recovery flows.
}

type FaultOverlay struct{ /* opaque: computed lookup tables keyed by *Service / *Edge */ }

// Top-level functions
func Parse(r io.Reader) (*Schema, error)        // YAML decode → resolve references → validate
func ParseFile(path string) (*Schema, error)
func Validate(s *Schema) error                  // structural invariants (DAG, journey reachability, etc.)
func Equal(a, b *Schema) bool                   // identifier-based deep equality (PBT-safe; replaces reflect.DeepEqual)

// Methods
func (s *Schema) FindServiceByName(id ServiceID) (*Service, bool)
func (s *Schema) JourneyNames() []string
func (s *Schema) ApplyFaults() *FaultOverlay
func (s *Schema) ExportJSONSchema() ([]byte, error)

// MarshalYAML serializes Schema back to YAML by converting resolved pointers
// (*Service, *Edge) back to their identifier strings. This makes round-trip
// (`Parse(Marshal(s)) ≡ s`) hold against topology.Equal.
func (s *Schema) MarshalYAML() (any, error)
```

---

## C2 — `journey`

```go
package journey

import (
    "context"
    "github.com/ymotongpoo/xk6-otel-gen/topology"
    "github.com/ymotongpoo/xk6-otel-gen/synth"
)

type Engine struct{ /* unexported */ }

type Plan struct {
    JourneyName string
    Root        *Node
}

type Node struct {
    Service   *topology.Service
    Operation string
    Edge      *topology.Edge   // edge used to reach this node from parent (nil for root)
    Parallel  []*Node          // siblings in parallel group (mutually exclusive with sequential Children)
    Children  []*Node          // sequential next steps from this service
}

type Outcome struct {
    Success    bool
    Latency    time.Duration  // includes time spent on failed primary + fallback retries
    StatusCode int            // HTTP / gRPC code as applicable (of the call that determined the outcome)
    ErrorType  string         // "" if Success, otherwise error.type semantic value
    Cascaded   bool           // true if this outcome was forced by upstream cascade (no recovery available)
    // Recovery tracking (NEW): present when an OnFailure RecoveryPolicy was traversed.
    PrimaryFailed       bool                 // true if primary edge failed (regardless of final Success)
    FallbackAttempts    []*topology.Edge     // ordered list of fallback edges tried (all of which failed)
    FallbackUsed        *topology.Edge       // fallback edge that ultimately succeeded (nil if none)
    DefaultUsed         bool                 // true if OnExhausted==return_default consumed the failure
    SilentlySucceeded   bool                 // true if OnExhausted==succeed_silently consumed the failure
}

// Top-level functions
func NewEngine(
    schema *topology.Schema,
    overlay *topology.FaultOverlay,
    synth synth.Synthesizer,
) *Engine

// Methods
func (e *Engine) BuildPlan(journeyName string) (*Plan, error)
func (e *Engine) Execute(ctx context.Context, plan *Plan) error
func (e *Engine) ListJourneys() []string
```

---

## C3 — `synth`

```go
package synth

import (
    "context"
    "go.opentelemetry.io/otel/log"
    "go.opentelemetry.io/otel/metric"
    "go.opentelemetry.io/otel/sdk/resource"
    "go.opentelemetry.io/otel/trace"
    "github.com/ymotongpoo/xk6-otel-gen/topology"
)

// Synthesizer is the interface Journey Engine sees.
type Synthesizer interface {
    // BeginSpan starts a span for a journey node and returns a context that
    // carries the span, plus a finish function that must be called after the
    // outcome is known.
    BeginSpan(ctx context.Context, node SpanInput) (context.Context, FinishSpanFunc)

    // RecordMetric records request count / duration histograms / active gauges
    // for the given service+operation+outcome.
    RecordMetric(ctx context.Context, m MetricInput)

    // EmitLog emits a structured log entry tied to current span context.
    EmitLog(ctx context.Context, l LogInput)
}

type SpanInput struct {
    Service   *topology.Service
    Edge      *topology.Edge   // nil for entry node
    Operation string
    StartTime time.Time
}

type FinishSpanFunc func(outcome Outcome)

type Outcome struct {
    Success    bool
    StatusCode int
    ErrorType  string
    EndTime    time.Time
}

type MetricInput struct {
    Service   *topology.Service
    Edge      *topology.Edge
    Operation string
    Latency   time.Duration
    Outcome   Outcome
}

type LogInput struct {
    Service   *topology.Service
    Severity  log.Severity
    Body      string
    Attributes map[string]any
}

// Top-level constructor
func NewDefault(
    tp trace.TracerProvider,
    mp metric.MeterProvider,
    lp log.LoggerProvider,
) Synthesizer

// Resource builder (per service instance)
func BuildResource(svc *topology.Service, instanceIdx int) *resource.Resource
```

---

## C4 — `exporter`

```go
package exporter

import (
    "context"
    "go.opentelemetry.io/otel/log"
    "go.opentelemetry.io/otel/metric"
    "go.opentelemetry.io/otel/trace"
)

type Protocol int
const (
    ProtocolGRPC Protocol = iota
    ProtocolHTTP
)

type Config struct {
    Protocol    Protocol
    Endpoint    string             // e.g. "https://otel.example.com:4317"
    Headers     map[string]string
    Insecure    bool
    Compression string             // "gzip" | "" | future
    BatchSize       int
    BatchTimeout    time.Duration
    ExportTimeout   time.Duration
    ResourceOverrides map[string]string
}

type Pipeline struct{ /* unexported */ }

type Stats struct {
    SpansExported   uint64
    SpansFailed     uint64
    MetricsExported uint64
    MetricsFailed   uint64
    LogsExported    uint64
    LogsFailed      uint64
    QueueDepth      int
}

// Construction
func New(cfg Config) (*Pipeline, error)
func ConfigFromEnv() Config
func (c Config) MergeWith(override Config) Config   // override wins on present fields

// Provider accessors
func (p *Pipeline) TracerProvider() trace.TracerProvider
func (p *Pipeline) MeterProvider() metric.MeterProvider
func (p *Pipeline) LoggerProvider() log.LoggerProvider

// Lifecycle
func (p *Pipeline) Shutdown(ctx context.Context) error
func (p *Pipeline) Stats() Stats
```

---

## C5 — `k6otelgen`

```go
package k6otelgen

import (
    "go.k6.io/k6/js/modules"
)

// Registered as "k6/x/otel-gen" via modules.Register.
type RootModule struct{ /* singleton state */ }

type ModuleInstance struct{ /* per-VU state */ }

// modules.Module / modules.Instance contract
func New() *RootModule
func (r *RootModule) NewModuleInstance(vu modules.VU) modules.Instance
func (i *ModuleInstance) Exports() modules.Exports

// JS-exposed top-level methods (registered as Exports.Named)
//   - Load(path string) (*TopologyHandle, error)
//   - Configure(opts goja.Value) error
//   - Stats() Stats   // pipeline self-observability for debugging
//   - Journeys() []string

type TopologyHandle struct{ /* references shared Schema, Engine factory */ }

// JS-exposed handle methods
//   - (*TopologyHandle).RunJourney(name string) error
//   - (*TopologyHandle).Journeys() []string
```

---

## C6 — `k6output`

```go
package k6output

import (
    "go.k6.io/k6/metrics"
    "go.k6.io/k6/output"
)

// Registered as "otel-gen" via output.RegisterExtension.
type Output struct{ /* unexported */ }

type Params struct {
    OutputParams output.Params
    Endpoint     string
    Protocol     string         // "grpc" | "http"
    Insecure     bool
    Headers      map[string]string
    // ... merged with env + JS
}

// k6 output.Output contract
func New(p output.Params) (output.Output, error)
func (o *Output) Description() string
func (o *Output) Start() error
func (o *Output) AddMetricSamples(samples []metrics.SampleContainer)
func (o *Output) Stop() error
```

---

## Notes for Functional Design

- 各 `*Input` 構造体の attributes ペイロードのキー命名は OTel Semantic Conventions に従う (`synth/attributes` で集中管理)
- `Outcome` の `ErrorType` 値の語彙 (例: `"http.client_error"`, `"timeout"`, `"upstream_unavailable"`, `"server.crash"`) は Functional Design でテーブル化
- `FaultSpec.SeverityParams` の具体的な数値表現 (確率、レイテンシ倍率、絶対値の追加) は Functional Design で確定
- `LatencyDist` のサポート分布 (lognormal / normal / exponential) と必須パラメータも Functional Design で確定
