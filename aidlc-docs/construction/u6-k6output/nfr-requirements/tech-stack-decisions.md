# U6 k6output — Tech Stack Decisions

本書は U6 (`k6output/`) が依存するパッケージ・採用された代替案・却下された案を確定する。

---

## 1. 依存モジュール (Production code)

### 1.1 採用一覧

| Module | Version | 用途 | 必要性 |
|---|---|---|---|
| `go.k6.io/k6/output` | latest stable | k6 Output SDK (`output.Output` interface, `output.Params`, `output.RegisterExtension`) | 必須 |
| `go.k6.io/k6/metrics` | latest stable | k6 metric type (`metrics.Sample`, `metrics.MetricType`, `metrics.SampleContainer`) | 必須 |
| `go.opentelemetry.io/otel/sdk/metric` | latest stable | 独自 MeterProvider 構築 (runner Resource attached) | 必須 |
| `go.opentelemetry.io/otel/metric` | latest stable | Float64Counter / Histogram / Gauge interface | 必須 |
| `go.opentelemetry.io/otel/sdk/resource` | latest stable | Runner Resource 構築 | 必須 |
| `go.opentelemetry.io/otel/attribute` | latest stable | `k6.tag.*` attribute key + Set | 必須 |
| `go.opentelemetry.io/otel/semconv/v1.27.0` | v1.27.0 (pinned) | `service.name`, `telemetry.sdk.*` const | 必須 |
| `github.com/ymotongpoo/xk6-otel-gen/exporter` | (local) | Config, GetShared, Pipeline, **NEW: `Pipeline.MetricExporter()`** | 必須 |
| stdlib `context`, `sync`, `time`, `strings`, `strconv`, `fmt`, `errors`, `os`, `runtime` | Go 1.25 | 標準 | 必須 |

### 1.2 採用しないモジュール

- U1 `topology` — k6 native metrics は topology 非依存
- U2 `journey` — synth path とは別
- U3 `synth` — 別 instrumentation
- U5 `k6otelgen` — exporter 経由でのみ共有
- `go.opentelemetry.io/otel/exporters/otlp/*` — exporter package が抽象化、U6 から直接 import せず
- `dop251/goja`, `grafana/sobek` — JS runtime は U5 のみ
- `golang.org/x/sync/errgroup` — sync.WaitGroup で十分

### 1.3 検証

`go list -deps ./k6output/...` の出力に `topology`, `journey`, `synth`, `k6otelgen` 等の direct dep がないことを CI で確認。

---

## 2. テスト依存 (Test-only)

| Module | Version | 用途 |
|---|---|---|
| `pgregory.net/rapid` | latest stable | PBT (TP-U6-1〜3) |
| `github.com/stretchr/testify` | latest stable | assertion |
| `go.opentelemetry.io/otel/sdk/metric/metricdata` | latest stable | ManualReader 経由で metric data 検証 |
| `go.k6.io/k6/lib/testutils` (or similar) | latest stable | k6 SDK test fixtures (`output.Params` builder) — k6 が exposing していれば |
| `github.com/ymotongpoo/xk6-otel-gen/testutil/generators` | (local) | `ValidK6Sample`, `ValidOutputParams` 等 |

---

## 3. Integration test 依存

- `xk6` — k6 binary build (U5 と共有 harness)
- Docker Engine + `docker compose`
- `otel/opentelemetry-collector-contrib:<pinned-tag>` (U3/U4/U5 と同 tag)

`-tags=integration` で default `go test` から除外。

### 3.1 U5 との integration harness 共有

U5 が既に `xk6 build` + Docker Collector を harness 化済 (`k6otelgen/integration/helpers.go`)。U6 は **同じ helper を copy or share**:
- **Option A**: shared helper as `testutil/integrationhelpers` package — DRY
- **Option B**: copy & paste — independent test suites
- 採用 Option B: integration test 互いに干渉せず、test suite が独立して動く方がメンテナンス容易。実装重複は小さい

NFR Design で確定。

---

## 4. Version 戦略

### 4.1 k6 SDK pinning

U5 と同じ k6 stable major を pin、minor は dependabot で追従。

### 4.2 OTel SDK

`go.opentelemetry.io/otel/sdk/metric` は U4 が pin する version に従う。U6 で direct import が増えるが、U4 と同じ version を使う限り conflict なし。

### 4.3 Go toolchain

- `go.mod`: `go 1.25`
- U1-U5 と整合

---

## 5. 代替案 (Rejected)

### 5.1 synth と同じ Resource を使い回し (rejected)

- 案: synth が emit する Resource (`service.name=<svc>`) で k6 metric も emit
- 却下理由: k6 runner の metric は「test 実行 agent の観測値」、simulated service の signal とは性質が異なる。dashboard で同じ service として混在すると混乱

### 5.2 synth と同じ MeterProvider を使い回し (rejected)

- 案: `pipeline.MeterProvider()` をそのまま使う
- 却下理由: 同 5.1。MeterProvider の Resource は immutable、独立 MeterProvider が必要 → U4 patch (`Pipeline.MetricExporter()` accessor)

### 5.3 同期 `AddMetricSamples` emit (rejected)

- 案: queue なし、毎 sample で OTel meter.Record を直呼び
- 却下理由: k6 high throughput で AddMetricSamples が blocking → k6 throughput 阻害

### 5.4 自前 OTLP encoder (rejected)

- 案: OTel SDK を使わず直接 OTLP protobuf encode
- 却下理由: 車輪の再発明、SDK で十分

### 5.5 k6 metric を semconv namespace に rewrite (rejected, FD Q4=B)

- 案: `http_req_duration` → `http.client.request.duration` (semconv)
- 却下理由: k6 runner 由来であるコンテキストが失われる。dashboard で synth signal と区別できない

### 5.6 Cardinality limit を U6 内で実装 (rejected, FD Q13=B/C)

- 案: attribute set cache upper bound (10000 等)、超過時 warn + skip
- 却下理由: silent data loss、user に予測困難。Collector 側で対応する方が responsibility 分離が clean

### 5.7 Stop() で error を返却 (rejected, FD Q7)

- 案: Shutdown 失敗時に error を return
- 却下理由: k6 lifecycle を crash させる、user friendly でない

---

## 6. CI / Lint 統合

### 6.1 必須 CI ジョブ

| ジョブ | コマンド | DoD blocking? |
|---|---|---|
| Build | `go build ./k6output/...` | Yes |
| Unit test (race) | `go test -race -count=1 ./k6output/...` | Yes |
| Coverage | `go test -cover ./k6output/...` ≥ 80% | Yes |
| Lint | `golangci-lint run ./k6output/...` | Yes |
| `go vet` | `go vet ./k6output/...` | Yes |
| Bench (per-sample) | `BenchmarkAddMetricSamples` < 1µs/sample, `BenchmarkFlushLoop` < 5µs/sample | Yes (strict) |
| Bench (lifecycle) | `BenchmarkStart`, `BenchmarkStop` | informational (Q1=C, Q4=C soft) |
| Integration (xk6 + Docker) | `go test -tags=integration ./k6output/integration/...` | nightly + manual |

### 6.2 lint rules

`.golangci.yml` で project 共通設定 (U2-U5 と同じ):
- `revive`, `govet`, `staticcheck`, `errcheck`, `unused`

---

## 7. Cross-unit dependency summary

```text
U6 (k6output) imports:
  - go.k6.io/k6/{output, metrics}
  - go.opentelemetry.io/otel/{sdk/metric, metric, sdk/resource, attribute, semconv/v1.27.0}
  - github.com/ymotongpoo/xk6-otel-gen/exporter (with patch: Pipeline.MetricExporter())
  - stdlib

U6 does NOT import:
  - topology, journey, synth, k6otelgen (within xk6-otel-gen)
  - go.opentelemetry.io/otel/exporters/otlp/*
  - errgroup, JS runtime libs

U6 is imported by:
  - cmd/xk6 (build target for U8)  — k6 binary build に link される
  - k6output/integration/         — integration test
```

---

## 8. Migration / Upgrade Notes

### 8.1 k6 output SDK upgrade

- `output.Output` interface 変更時、`output.go` の Start/Stop/AddMetricSamples を更新
- breaking change は手動対応 phase で

### 8.2 OTel sdk/metric upgrade

- `sdkmetric.NewMeterProvider` の API 変更時、runner MeterProvider 構築コードを更新
- `WithReader` / `WithResource` の signature 維持を期待

### 8.3 exporter package upgrade

- U4 が `Pipeline.MetricExporter()` を refactor / 削除する場合、U6 は依存破綻
- U4 NFR-D 改訂 + U6 patch を coordination

---

## 9. Open questions for Future revisit

| 質問 | 想定 trigger |
|---|---|
| queue size の最適 default 値 | bench 結果見て調整、現状 100 |
| flush ticker 間隔 (現 1 sec) の調整可能化 | high-frequency burst に対応する場合 |
| OTel exemplar 機能対応 (k6 sample に exemplar を attach) | OTel exemplar が安定したら検討 |
| k6 group / scenario hierarchy を Resource に attach | k6 SDK が提供開始した場合 |
| `--out otel-gen=queueSize=N` の validate 範囲 | NFR Design で具体的に [10, 10000] 等を決める |
