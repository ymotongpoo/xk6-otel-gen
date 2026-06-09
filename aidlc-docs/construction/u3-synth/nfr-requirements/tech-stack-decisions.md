# U3 synth — Tech Stack Decisions

本書は U3 (`synth/`) が依存するパッケージ・採用された代替案・却下された案を確定する。

---

## 1. 依存モジュール (Production code)

### 1.1 採用一覧 (Go module)

| Module | Version | 用途 | 必要性 |
|---|---|---|---|
| `go.opentelemetry.io/otel/trace` | latest stable | TracerProvider / Tracer / SpanKind / Status interface | 必須 |
| `go.opentelemetry.io/otel/metric` | latest stable | MeterProvider / Meter / Histogram / UpDownCounter interface | 必須 |
| `go.opentelemetry.io/otel/log` | latest stable | LoggerProvider / Logger / Record / Severity interface | 必須 |
| `go.opentelemetry.io/otel/sdk/resource` | latest stable | `Resource` 型 (`BuildResource` 戻り値) | 必須 |
| `go.opentelemetry.io/otel/attribute` | latest stable | `KeyValue` 構築 | 必須 |
| `go.opentelemetry.io/otel/semconv/v1.27.0` | v1.27.0 (pinned) | semantic conventions 定数 (HTTP/RPC/DB/Messaging) | 必須 |
| `github.com/google/uuid` | latest stable | UUID v5 (deterministic `service.instance.id`) | 必須 |
| `github.com/ymotongpoo/xk6-otel-gen/topology` | (local) | `Service` / `Edge` / `Operation` 型参照 | 必須 |

### 1.2 採用しないモジュール

- `go.opentelemetry.io/otel/propagation` — in-process telemetry 合成のため不要 (U4 と同方針)
- `go.opentelemetry.io/otel/baggage` — 同上
- `go.opentelemetry.io/otel/sdk/trace`, `sdk/metric`, `sdk/log` の concrete 型 — U3 は interface 経由のみ参照、SDK 直接 import すると U4 と循環依存リスク
- `go.opentelemetry.io/otel/exporters/*` — U4 の責務
- `go.opentelemetry.io/otel/exporters/stdout/*` — debug の場合は test code 側で別途
- `go.opentelemetry.io/otel/sdk/instrumentation` — U3 では Tracer/Meter/Logger の `instrumentation.Scope` を自動構築 (instrumentation name のみ指定)、明示構築不要

### 1.3 検証

`go list -deps ./synth/...` の出力に上記許可リスト外の `go.opentelemetry.io/otel/*` モジュールが含まれないことを CI で sanity check。

---

## 2. テスト依存 (Test-only)

| Module | Version | 用途 |
|---|---|---|
| `pgregory.net/rapid` | latest stable | PBT (TP-U3-1〜4) |
| `github.com/stretchr/testify` | latest stable | assertion (`require.*`, `assert.*`) |
| `go.opentelemetry.io/otel/sdk/trace/tracetest` | latest stable | in-memory span exporter (mock provider) |
| `go.opentelemetry.io/otel/sdk/metric/metricdata` | latest stable | metric data inspection |
| `go.opentelemetry.io/otel/sdk/log/logtest` | latest stable | in-memory log recorder (存在しない場合は自前 mock) |

### 2.1 mock provider strategy (Q8=A)

OTel SDK 公式の test utility を活用:
- **Trace**: `sdktrace.NewTracerProvider(sdktrace.WithBatcher(tracetest.NewInMemoryExporter()))` でテスト構築
- **Metric**: `sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))` + `reader.Collect(ctx, &rm)` で内容取得
- **Log**: SDK にまだ official in-memory recorder がない場合、自前で interface mock を一時的に作る (将来 official 化されたら置き換え)

---

## 3. Integration test 依存

Q11=A の `synth/integration/` で使用:
- Docker Engine + `docker compose`
- `otel/opentelemetry-collector-contrib:<pinned-tag>` (U4 と同 tag)
- `exporter` (U4) パッケージ — Pipeline 構築用

`-tags=integration` build tag で default `go test` から除外。`golangci-lint` も skip。

---

## 4. Version 戦略

### 4.1 OTel Go SDK の latest 追従

- Dependabot を `.github/dependabot.yml` で `go.opentelemetry.io/otel/*` group に設定
- 月次 PR、CI green なら自動 merge 可
- Breaking change がある場合は手動 review

### 4.2 semconv version の明示固定

- 現在: `v1.27.0`
- bump 規約: 新 LTS が出たら明示的に PR 作成
  - `attributes.go` の import path 変更
  - 廃止 symbol を新 symbol に置換
  - PBT TP-U3-2 の allowed key set 更新
  - NFR-U3-8 の文書化 version 更新

### 4.3 Go toolchain

- `go.mod`: `go 1.25`
- toolchain: `go1.25.x` 最新
- U4 / U1 と整合

### 4.4 transitive dependency

- `go mod tidy` のみで管理、`replace` directive は使わない
- `vendor/` を作らない (`go.sum` で integrity 確保)

---

## 5. 代替案 (Rejected)

### 5.1 SDK concrete 型を直接 import (rejected)

- 案: `synth.NewDefault(*sdktrace.TracerProvider, *sdkmetric.MeterProvider, ...)`
- 却下理由: U4 (`exporter.Pipeline`) を直接 import する循環構造になる。interface 経由なら U4 を import せずに済む

### 5.2 semconv 非 import (rejected)

- 案: U4 と同じく raw string key 採用
- 却下理由: U3 は attribute key を **数十** 使うので typo リスク大。IDE 補完の便益が大きい (`u3-synth/functional-design/business-rules.md` §1 で詳述)
- ※ U4 側も exclusive な exclusion を撤回 (`u4-exporter/nfr-requirements/tech-stack-decisions.md` §1.2 更新済)

### 5.3 functional options for NewDefault (rejected)

- 案: `synth.NewDefault(opts ...synth.Option)` で provider を option として渡す
- 却下理由: provider 3 つは必須引数、option 化のメリットがない。明示引数で十分

### 5.4 Self-stats / Self-metric を持つ (rejected)

- 案: `synth.Stats{SpansBegun, SpansFinished, ...}` を expose
- 却下理由: Q3=A、U4 の `exporter.Stats` で送信側の観測が済む。重複計装は冗長

### 5.5 Generics 化した Synthesizer (rejected)

- 案: `Synthesizer[T any]` で signal 型を generics 化
- 却下理由: trace / metric / log は OTel SDK で interface 型がそれぞれ異なる、generics 化のメリット小

### 5.6 OTel auto-instrumentation (rejected)

- 案: `go.opentelemetry.io/contrib/instrumentation/*` を組み込んで k6 の HTTP req を自動計装
- 却下理由: 本ツールは合成データを生成する責務、real HTTP/RPC traffic を計装する unit ではない。U6 (k6output) で k6 native metric 変換は別途行うが、それも auto-instrumentation とは異なる

### 5.7 Self-rolled OTel attribute set (rejected)

- 案: `attribute.KeyValue` ではなく自前構造体で管理
- 却下理由: OTel 標準を直接使う方が後方互換性、export 経路で確実

---

## 6. CI / Lint 統合

### 6.1 必須 CI ジョブ

| ジョブ | コマンド | DoD blocking? |
|---|---|---|
| Build | `go build ./synth/...` | Yes |
| Unit test (race) | `go test -race -count=1 ./synth/...` | Yes |
| Coverage | `go test -cover ./synth/...` ≥ 80% | Yes |
| Bench (regression) | `go test -bench=. -benchmem ./synth/...` vs baseline | Yes |
| Lint | `golangci-lint run ./synth/...` | Yes |
| `go vet` | `go vet ./synth/...` | Yes |
| Integration (Docker) | `go test -tags=integration ./synth/integration/...` | nightly + manual trigger |

### 6.2 lint rules

`.golangci.yml` で project 共通設定 (U4 と同じ):
- `revive` (GoDoc 網羅)
- `govet`
- `staticcheck`
- `errcheck`
- `unused`

---

## 7. Cross-unit dependency summary

```text
U3 (synth) imports:
  - go.opentelemetry.io/otel/{trace,metric,log,attribute}        (interfaces)
  - go.opentelemetry.io/otel/sdk/resource                        (Resource type only)
  - go.opentelemetry.io/otel/semconv/v1.27.0
  - github.com/google/uuid
  - github.com/ymotongpoo/xk6-otel-gen/topology

U3 does NOT import:
  - exporter/ (U4)         — interface 経由のみ依存、source code 非依存
  - journey/ (U2)          — Engine が synth を import する方向のみ
  - propagation, baggage   — in-process telemetry 合成のため不要

U3 is imported by:
  - journey/ (U2)          — Engine が Synthesizer を呼ぶ
  - k6otelgen/ (U5)        — NewDefault を呼んで Engine に注入
  - testutil/generators/   — ValidSpanInput 等が synth 型を生成
```

---

## 8. Migration / Upgrade Notes

### 8.1 semconv v1.27.0 → v1.28.0+ への bump 手順

1. `go get go.opentelemetry.io/otel/semconv/v1.NN.0@latest`
2. `synth/attributes.go` の `import` を変更
3. 廃止された const があれば新 const に grep + replace
4. PBT TP-U3-2 の `allowedAttrKeys` を更新
5. `NFR-U3-8` 文書化 version を更新
6. `go test -race -count=1 ./synth/...` で確認

### 8.2 OTel SDK major bump 時

- breaking change の影響範囲を `interface.go` / `synthesizer.go` で確認
- 必要なら U4 の Provider accessor signature にも影響、U4 NFR Design と coordination

---

## 9. Open questions for Future revisit

| 質問 | 想定 trigger |
|---|---|
| `service.instance.id` UUID v5 namespace の固定値選択 | NFR Design で確定する placeholder (`00000000-0000-0000-0000-000000000003` 案) |
| `process.runtime.name=svc.Language` の semantic 妥当性 | OTel community の semconv discussion fork で確認 (今後) |
| Histogram bucket boundaries の k6 ワークロード fit | bench 実装後の実測値 |
| Log 自動 Body 生成のテンプレート確定 | NFR Design |
| Stateful PBT (PBT-06) の `active_requests` 平衡検証 | 実装時に必要なら追加 |
