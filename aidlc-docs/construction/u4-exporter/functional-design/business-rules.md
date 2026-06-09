# U4 exporter — Business Rules

本書は U4 の業務規則・不変条件・Testable Properties を確定する。

---

## 1. Config 値の制約

### 1.1 構文・形式

| Field | 規則 |
|---|---|
| `Protocol` | `ProtocolGRPC` または `ProtocolHTTP` のみ。他は `*ConfigError` |
| `Endpoint` | 非空文字列、`scheme://host[:port]` 形式または `host:port` 形式 (gRPC 慣例)。空の場合 `fillDefaults` が `localhost:4317` を入れる |
| `Headers` | key/value とも非空、key は `[A-Za-z0-9_\-]+` (HTTP header 命名) |
| `Insecure` | bool、デフォルト false (TLS 検証する) |
| `Compression` | `"gzip"` または `""` のみ。他の値は warn ログ相当 (将来追加可能) |
| `Timeout` | `> 0` (零は default に置換される)、< 30s 推奨 (1 min を超えると k6 batch が滞留) |
| `BatchSize` | `> 0`、< 65536 (OTLP message size 上限を考慮) |
| `BatchTimeout` | `> 0`、< 60s 推奨 |
| `MaxQueueSize` | `> 0`、`>= BatchSize` (queue が batch より小さいと drop が頻発) |
| `ResourceOverrides` | key/value とも非空。`service.name` のような Semantic Conventions key が想定主用途 |

### 1.2 ConfigError

```go
type ConfigError struct {
    Field   string
    Value   any
    Message string
}
func (e *ConfigError) Error() string {
    return fmt.Sprintf("exporter: invalid Config.%s = %v: %s", e.Field, e.Value, e.Message)
}
```

`Config.Validate()` (任意 API、業務上は New 内で呼ぶ) が `*ConfigError` を返す。複数違反は `errors.Join`。

---

## 2. Config Merge ルール (Q1=A + Q2=A)

### 2.1 優先順位 (high → low)

```
JS API options  >  環境変数 (OTEL_EXPORTER_OTLP_*)  >  YAML defaults  >  built-in defaults
```

利用パターン:
```go
final := builtIn.MergeWith(yamlDefaults).MergeWith(envCfg).MergeWith(jsCfg)
```

`MergeWith` は左辺ベース、右辺の non-zero フィールドで上書き。

### 2.2 各 Field の override 条件 (再掲)

| Field | override 条件 |
|---|---|
| `Protocol` | `override.Protocol != ProtocolGRPC` |
| `Endpoint` | `override.Endpoint != ""` |
| `Headers` | `override.Headers != nil` (map ごと置換) |
| `Insecure` | `override.Insecure == true` (片方向) |
| `Compression` | `override.Compression != ""` |
| `Timeout` | `override.Timeout != 0` |
| `BatchSize` / `BatchTimeout` / `MaxQueueSize` | `> 0` のみ |
| `ResourceOverrides` | `!= nil` (map ごと置換) |

### 2.3 Headers の merge ポリシー

`Headers` は **map ごと置換** (key 単位の merge ではない)。理由:
- 利用者が「特定の header だけ追加」したい場合、JS から full headers を渡してもらう設計
- key 単位 merge は意図しない default 残留を生む

### 2.4 ConfigFromEnv の signal-specific 不一致時

`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` と `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` が異なる値を持つ場合 → `*ConfigError`。Q1=A の単一 Config モデルでは信号別 endpoint 非サポート。

汎用 (`OTEL_EXPORTER_OTLP_ENDPOINT`) と signal-specific が併存する場合、signal-specific が **優先**。3 信号で signal-specific が同じ値ならその値、不一致なら error。

---

## 3. Provider 構築の規則

### 3.1 順序

```
buildResource → buildTraceExporter → buildMetricExporter → buildLogExporter → NewTracerProvider / NewMeterProvider / NewLoggerProvider
```

### 3.2 失敗時の cleanup

Q5=A all-or-nothing:
- 第 N 信号の Exporter 構築が失敗したら、既に作った N-1 個の Exporter を Shutdown して破棄
- `New(Config)` が `*PipelineError{Stage: <stage>, Inner: err}` を返す
- `*Pipeline` は返さない

### 3.3 Provider のオプション

- `WithResource(res)` 必須 (Q10=A の resource を渡す)
- `WithBatcher(exp, ...)` / `WithReader(reader)` / `WithProcessor(...)` でカスタム batch 設定 (Config から)
- Sampler は本 unit では設定しない (= `AlwaysSample`、OTel SDK 既定)。将来必要なら Config 拡張

---

## 4. Stats 不変条件

### 4.1 Monotonicity (PBT-03 / TP-U4-4)

- すべての `*Exported` / `*Failed` counter は **減少しない** (`atomic.Add` のみ)

### 4.2 Atomic snapshot (Q7=A)

- 各 field は `atomic.Int64` で個別に管理
- `Stats()` は各 field を順次 `Load`、構造体に詰めて返す
- **field 間で同一時点保証はない** (例: `TracesExported` を読んだ後、`MetricsExported` を読むまでに別 goroutine の write が入りうる)。これは monotonicity が壊れない限り問題なし

### 4.3 Stats と OTel SDK の関係

OTel SDK の Exporter interface (`ExportSpans/Metrics/Logs`) をラップして stats を更新。SDK 自体が内部で持つ stats (Provider 内の dropped span count 等) は **直接読まない** (Q6=A 最小スコープ)。

---

## 5. Shutdown SLA (Q8=A)

| 状況 | 振る舞い |
|---|---|
| `ctx` に deadline あり、SDK が flush 完了する | nil 返却 |
| `ctx` に deadline あり、deadline 超過 | `ctx.Err()` (`context.DeadlineExceeded`) を含む joined error 返却 |
| `ctx` が cancel された | `context.Canceled` を含む joined error 返却 |
| `ctx.Background()` | OTel SDK の内部 default timeout に従う (典型的に 30s) |
| 多重呼び出し | 2 回目以降は同じ error/nil を返却 (no-op、`sync.Once` で初回結果キャッシュ) |

---

## 6. Testable Properties (PBT-01、Q11=A)

### TP-U4-1: Config Merge 優先順位 (PBT-03 Invariant)

```text
For all Configs a, b drawn from ValidConfig():
    For any non-zero field f in b:
        a.MergeWith(b).{f} == b.{f}
```

実装:
```go
func TestMergeWith_OverrideWins(t *testing.T) {
    t.Parallel()
    rapid.Check(t, func(t *rapid.T) {
        a := generators.ValidConfig().Draw(t, "a")
        b := generators.ValidConfig().Draw(t, "b")
        merged := a.MergeWith(b)
        if b.Endpoint != "" { require.Equal(t, b.Endpoint, merged.Endpoint) }
        if b.Timeout != 0   { require.Equal(t, b.Timeout, merged.Timeout) }
        // ... 各 non-zero field を確認
    })
}
```

### TP-U4-2: Config Merge Idempotency (PBT-04)

```text
For all Config c drawn from ValidConfig():
    c.MergeWith(c) == c
```

### TP-U4-3: OTLP Protobuf Round-trip (PBT-02)

```text
For all otlpcollectortrace.ExportTraceServiceRequest msg drawn from AnyOTLPRequest():
    msg2 := proto.Unmarshal(proto.Marshal(msg))
    proto.Equal(msg, msg2) == true
```

このテストは `go.opentelemetry.io/proto/otlp` の型を直接使う。U4 が exporter で何を送るかを検証するというより、protobuf round-trip が成立することの sanity check。

### TP-U4-4: Stats Monotonicity (PBT-03)

```text
For any sequence of operations on a Pipeline:
    Stats().TracesExported at time t2 >= Stats().TracesExported at time t1 (where t2 > t1)
    Same for MetricsExported, LogsExported, *Failed counters.
```

実装は stateful PBT 寄り — 一定回数 Exporter を呼ぶ間に Stats を複数回観測、各観測ペアで monotonicity を確認:

```go
func TestStats_Monotonic(t *testing.T) {
    t.Parallel()
    rapid.Check(t, func(t *rapid.T) {
        p := newTestPipeline(t) // mock exporter で構築
        var prevStats Stats
        nOps := rapid.IntRange(1, 20).Draw(t, "n_ops")
        for i := 0; i < nOps; i++ {
            simulateExport(p)
            s := p.Stats()
            require.GreaterOrEqual(t, s.TracesExported, prevStats.TracesExported)
            // ... 他の counter も
            prevStats = s
        }
    })
}
```

---

## 7. Shared Pipeline Holder の規則 (Q3=A, Q9=A)

| 規則 | 内容 |
|---|---|
| 初期化は 1 回のみ | `sync.Once` により保証 |
| 初回 factory が成功 → 以降 `GetShared` は同じ `*Pipeline` | k6 lifecycle 全体で 1 つの Pipeline |
| 初回 factory が失敗 → 以降 `GetShared` は同じ error | 失敗の再試行はしない (k6 ラン全体 fail fast) |
| `SetShared` は `sharedPipeline == nil` の時のみ成功 | テスト用、production code から呼ばない |
| `ResetShared` はテスト用 | `sync.Once` を新規 instance に置換、shared を nil |
| 並行 `GetShared` 呼び出し | `sync.Once` でシリアライズ、全 caller が同じ結果を得る |

---

## 8. Resource 規則 (Q10=A)

### 8.1 自動検出

`buildResource` 内で OTel SDK の自動 detector を有効化:
- `resource.WithFromEnv()` — `OTEL_RESOURCE_ATTRIBUTES` env 等
- `resource.WithHost()` — `host.name` / `host.id`
- `resource.WithProcess()` — `process.pid` / `process.executable.name` 等
- `resource.WithOS()` — `os.type` / `os.description` 等

### 8.2 Override の merge

`cfg.ResourceOverrides` が non-empty なら:
- `sdkresource.Merge(detected, override)` — override key が detected key と衝突する場合、override が勝つ

具体例:
- detected: `host.name=host123, service.name=unknown`
- override: `service.name=catalog-service, deployment.environment=prod`
- merge result: `host.name=host123, service.name=catalog-service, deployment.environment=prod`

### 8.3 必須属性

`service.name` は OTel Semantic Conventions の必須属性だが、本 unit は **強制しない**。`service.name` 不在の場合、SDK が `unknown_service` を assign する (SDK 既定挙動)。利用者が明示するべきだが、エラーにはしない。

---

## 9. Error 型階層

```go
// PipelineError は New が返すエラー (Pipeline 構築失敗).
type PipelineError struct {
    Stage   string // "resource" | "trace_exporter" | "metric_exporter" | "log_exporter" | "validate"
    Inner   error
}

// ConfigError は Config 検証 / parse のエラー.
type ConfigError struct {
    Field   string
    Value   any
    Message string
}

// SharedError は GetShared / SetShared のエラー.
type SharedError struct {
    Reason string // "already_initialized" | "init_failed" | "not_set"
    Inner  error
}
```

すべて `errors.As` で型抽出可能。多重エラーは `errors.Join` で集約。

---

## 10. 設定値 vs 環境変数 vs SDK 既定の優先関係

開発者は混乱しやすいので明示:

```text
┌─────────────────────────────────────────────────────────────┐
│ 利用者の意図                                                 │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ 1. JS API options (k6 script の otelgen.configure({})) │ │  最優先
│ ├─────────────────────────────────────────────────────────┤ │
│ │ 2. OTEL_EXPORTER_OTLP_* 環境変数                       │ │
│ ├─────────────────────────────────────────────────────────┤ │
│ │ 3. YAML defaults (本拡張固有、topology YAML の          │ │
│ │    `exporter:` セクションに将来定義可能、初期は未実装) │ │
│ ├─────────────────────────────────────────────────────────┤ │
│ │ 4. built-in defaults                                    │ │
│ │    (Endpoint=localhost:4317, Timeout=10s, ...)         │ │
│ └─────────────────────────────────────────────────────────┘ │  最後の砦
└─────────────────────────────────────────────────────────────┘
```

3. YAML defaults は **未実装** (将来拡張)。本 unit の `New(cfg)` は cfg を受け取るのみで、YAML defaults の読み込み責務は持たない。利用者 (k6otelgen) が `topology.Schema` から YAML defaults を抽出して Config に詰める想定。

---

## 11. パフォーマンスとリソース

| 項目 | 期待値 |
|---|---|
| `New(Config)` の所要時間 | < 100 ms (gRPC connection 確立込み、再試行なし) |
| `Stats()` 呼び出し | < 1 µs (6 個の `atomic.Load`) |
| `Shutdown(ctx)` 通常時 | < BatchTimeout + buffer (~6 s) |
| `Shutdown(ctx)` deadline 即時 | < 100 ms (即座に context.DeadlineExceeded を返す) |
| Pipeline インスタンス 1 個のメモリ | < 100 KB (3 Exporter + 3 Provider + Resource + Stats、connection bufferを除く) |

NFR-U4 (後の NFR-R で詳細確定) で各項目を厳密化する予定。
