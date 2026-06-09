# U4 exporter — Tech Stack Decisions

本書は U4 で採用する技術スタック・バージョン方針・依存追加・integration test 用 Collector スタックの判断を確定する。

---

## 1. OTel Go SDK 採用モジュール (Q1=A 最小セット)

### 1.1 確定モジュールリスト

| モジュール | 用途 | 使用箇所 |
|---|---|---|
| `go.opentelemetry.io/otel/sdk/trace` | TracerProvider, BatchSpanProcessor | `pipeline.go`, `exporters.go` |
| `go.opentelemetry.io/otel/sdk/metric` | MeterProvider, PeriodicReader | `pipeline.go`, `exporters.go` |
| `go.opentelemetry.io/otel/sdk/log` | LoggerProvider, BatchLogProcessor | `pipeline.go`, `exporters.go` |
| `go.opentelemetry.io/otel/sdk/resource` | Resource 構築 | `resource.go` |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace` | Trace exporter base | `exporters.go` |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` | gRPC trace exporter | `exporters.go` (Q1 of FD = gRPC + HTTP 両方) |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` | HTTP trace exporter | 同上 |
| `go.opentelemetry.io/otel/exporters/otlp/otlpmetricgrpc` | gRPC metric exporter | `exporters.go` |
| `go.opentelemetry.io/otel/exporters/otlp/otlpmetrichttp` | HTTP metric exporter | 同上 |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc` | gRPC log exporter | `exporters.go` |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp` | HTTP log exporter | 同上 |
| `go.opentelemetry.io/otel/attribute` | Resource attribute 構築 | `resource.go` |
| `go.opentelemetry.io/proto/otlp` | OTLP protobuf 型 (test only、TP-U4-3) | `otlp_roundtrip_test.go` |

### 1.2 採用しないモジュール

- **`go.opentelemetry.io/otel/propagation`** — **本プロジェクト全体で不要** (U4 だけでなく U3/U5 等の上位 unit でも不要)。理由:
  - 本ツールは **擬似テレメトリーシグナルを直接生成** する: U2 (journey) が `tracer.Start(ctx, ...)` でルート span を作り、同じ `ctx` を渡して下流の擬似 span を組み上げる → 同一プロセス内の `context.Context` 機構だけで親子関係 / trace_id 共有が完結
  - U4 が OTLP で送るのは **既に組み上がった protobuf** (trace_id は payload 内 field)。HTTP/gRPC header に traceparent を inject する必要なし
  - 本ツールは外部 service を実際には呼ばない (架空 journey を全部 in-process で合成してから OTLP に送るだけ) ため、`propagation.Inject` / `Extract` を要するシナリオが存在しない
  - 3 信号の correlation (NFR-U4-4) も `Counter.Add(ctx, ...)` / `Logger.Emit(ctx, record)` 経由で SDK が in-process Context から自動付与するため、propagation は不要
  - 将来 (k6 本物 HTTP リクエストとの混在、外部から trace_id seed を受け取る等の) 拡張で必要になれば後追い import で良い
- `go.opentelemetry.io/otel/exporters/stdout/*` — デバッグ用、本 unit からは import しない (開発者が必要なら自分の test code で別途 import)
- `go.opentelemetry.io/otel/semconv/v1.27.0` — **U3 (synth) と整合して U4 でも import 可**。U4 production code は `ResourceOverrides map[string]string` を user-provided key として受け取るので semconv 定数の出番は現状ないが、将来 U4 内で hard-coded semconv attribute を出す箇所が発生したら import する (export attr key の typo 防止)。テストコードでは `"service.name"` 等の固定値検証に `string(semconv.ServiceNameKey)` を使って良い。Bump プロトコルは U3 と同じ (`attributes.go` 等 1 ファイルに集約) — U4 では `resource.go` に局所化される想定。
- `go.opentelemetry.io/otel/baggage` — 本 unit では使わない (propagation と同様の理由 + そもそも baggage を運ぶ HTTP request も無いため)

### 1.3 検証

`go list -deps ./exporter/...` の出力に上記許可リスト外の `go.opentelemetry.io/otel/*` モジュールが含まれないことを CI で sanity check (Build and Test ステージで grep ベースの簡易チェックを CI に組み込む)。

---

## 2. OTel SDK バージョン方針 (Q2=A 最新追従、利用者注記反映)

### 2.1 戦略

| 項目 | 内容 |
|---|---|
| **追従ポリシー** | 最新 stable に追従。**開発中も継続的に最新安定版を取り込む** (利用者注記) |
| **`go.mod` 表現** | minimum version を記載 (例: `go.opentelemetry.io/otel/sdk v1.x.y`)。実バージョンは U4 Code Generation 開始時に確定 |
| **アップデート手段** | (a) ローカル `go get -u go.opentelemetry.io/otel/...` (b) CI で dependabot 週次自動 PR (Build and Test ステージで設定) (c) 開発中も `go get @latest` を folder で手動実行可 |
| **Major version up (v1 → v2 等)** | **手動評価**。changelog / breaking change を確認、影響範囲を判定してから採用 |
| **Minor / Patch** | dependabot で自動採用 (CI が green ならマージ可) |

### 2.2 OTel Go SDK の構造的注意

OTel Go SDK は **複数 module repository** で、`otel-go` / `otel-go-contrib` 等に分割。本 unit は主に `otel-go` 配下のモジュールを使うが、それぞれが独立にバージョン管理されているため `go.mod` には複数のエントリが並ぶ。dependabot が個別に PR を出すので、整合性 (同じ minor) を保つよう手動で揃える運用ルール:
- すべての `go.opentelemetry.io/otel/...` モジュールを同じ minor に揃える
- 一括 update する場合は `go get go.opentelemetry.io/otel/sdk@latest && go mod tidy` を実行し、関連モジュールが揃うか確認

---

## 3. Integration Test 用 OTel Collector スタック (Q3=A, Q4=A + correlation)

### 3.1 採用ツール

| ツール | バージョン | 用途 |
|---|---|---|
| `otel/opentelemetry-collector-contrib` (Docker image) | `latest` (production stable tag, `0.108.0` 以上想定) | 受信検証用 Collector |
| Docker / Docker Compose | システム提供 (CI runner / 開発者 host) | Collector 起動 |
| `file_exporter` (Collector 組込) | Collector に付属 | 受信内容を JSON ファイルに書き出して assert 可能に |

### 3.2 Collector 設定 (整備対象)

`test/integration/exporter/collector-config.yaml` (例):

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:

exporters:
  file:
    path: /var/log/otel/traces.json
    rotation:
      max_megabytes: 10
  file/metrics:
    path: /var/log/otel/metrics.json
  file/logs:
    path: /var/log/otel/logs.json

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [file]
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [file/metrics]
    logs:
      receivers: [otlp]
      processors: [batch]
      exporters: [file/logs]
```

### 3.3 Docker Compose

`test/integration/exporter/docker-compose.yaml`:

```yaml
services:
  collector:
    image: otel/opentelemetry-collector-contrib:0.108.0
    command: ["--config=/etc/otel-collector-config.yaml"]
    volumes:
      - ./collector-config.yaml:/etc/otel-collector-config.yaml:ro
      - /var/log/otel:/var/log/otel
    ports:
      - "4317:4317"
      - "4318:4318"
```

### 3.4 Correlation 検証 (利用者注記 "可能ならcorrelationできてほしい")

Integration test で 3 信号を **同一 trace context** で発行:

```go
//go:build integration

package exporter_test

func TestIntegration_ThreeSignals_Correlated(t *testing.T) {
    // 1. Pipeline を構築
    pipe, _ := exporter.New(exporter.Config{
        Protocol: exporter.ProtocolGRPC,
        Endpoint: "localhost:4317",
        Insecure: true,
        ResourceOverrides: map[string]string{"service.name": "u4-int-test"},
    })
    defer pipe.Shutdown(context.Background())

    // 2. Tracer から span を作り、その span context 上で metric + log を発行
    tracer := pipe.TracerProvider().Tracer("u4-int-test")
    ctx, span := tracer.Start(context.Background(), "correlated-op")
    expectedTraceID := span.SpanContext().TraceID()

    meter := pipe.MeterProvider().Meter("u4-int-test")
    counter, _ := meter.Int64Counter("u4.int.counter")
    counter.Add(ctx, 1) // span context が乗ると exemplar に trace_id が入る

    logger := pipe.LoggerProvider().Logger("u4-int-test")
    var rec log.Record
    rec.SetTimestamp(time.Now())
    rec.SetBody(log.StringValue("correlated log message"))
    logger.Emit(ctx, rec) // span context が乗ると trace_id field に伝播

    span.End()

    // 3. Pipeline.Shutdown を待ち、Collector の出力ファイルを assert
    pipe.Shutdown(context.Background())
    waitForFile("traces.json", 10*time.Second)
    waitForFile("metrics.json", 10*time.Second)
    waitForFile("logs.json", 10*time.Second)

    // 4. 各 JSON ファイルから trace_id を抽出し、expectedTraceID と比較
    assertTraceIDPresent("traces.json", expectedTraceID)
    assertMetricExemplarTraceID("metrics.json", expectedTraceID) // best-effort
    assertLogTraceID("logs.json", expectedTraceID)
}
```

Notes:
- **Metric exemplar の trace_id 紐付け**: OTel SDK の metric exporter が exemplar を含めるよう設定が必要。OTel SDK 既定で有効か要確認 → 必要なら明示的に `metric.WithReader(metric.NewPeriodicReader(exp, metric.WithExemplarFilter(...)))` を追加。本 NFR では best-effort、最終的な実装は U4 Code Generation で決定
- **Log の trace_id 紐付け**: OTel SDK log の `Emit(ctx, record)` は ctx の span context を log record の `TraceID` / `SpanID` field に自動で乗せる (SDK 0.5+)。これは追加設定不要

### 3.5 CI 統合

Build and Test ステージ (U8) で GitHub Actions workflow に組み込む:

```yaml
# .github/workflows/integration.yml (sketch)
- name: Start Collector
  run: docker compose -f test/integration/exporter/docker-compose.yaml up -d
- name: Wait for Collector
  run: sleep 5
- name: Run integration tests
  run: go test -tags=integration ./...
- name: Stop Collector
  if: always()
  run: docker compose -f test/integration/exporter/docker-compose.yaml down
```

---

## 4. Go バージョン要件

| 項目 | 内容 |
|---|---|
| **最低バージョン** | **Go 1.25** (プロジェクト全体 NFR-3.2、AGENTS.md §5 と統一) |
| **`go.mod` の `go` directive** | `go 1.25` (U1 で既に設定済み、U4 は変更しない) |

---

## 5. 採用バージョン (Code Generation 開始時に確定する記録)

U4 Code Generation Phase 0 で以下を実行し、結果を `code-generation-summary.md` に記録する想定:

```bash
go get go.opentelemetry.io/otel/sdk@latest
go get go.opentelemetry.io/otel/sdk/metric@latest
go get go.opentelemetry.io/otel/sdk/log@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlpmetricgrpc@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlpmetrichttp@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp@latest
go get go.opentelemetry.io/proto/otlp@latest   # test only
go mod tidy
```

---

## 6. 採用しなかった代替案

| 代替案 | 不採用理由 |
|---|---|
| 自前 OTLP protobuf 実装 | OTel SDK が成熟しており、自前実装は redundancy と保守負荷増 |
| 旧式 (`otel/api` v1 pre-stable) | 既に obsolete、現行 SDK v1.x の安定版を使う |
| `opentelemetry-collector` を自前 host (Collector を import) | 本 unit は exporter side のため Collector 不要、import すると大きな依存 |
| OTel Auto-Instrumentation libraries | 本 unit は手動計装、自動計装は不要 |
| `OTEL_EXPORTER_OTLP_TRACES_*` 等の信号別 env を完全無視 (Q2 of FD で C 選択時) | Q2=A により採用、エコシステム互換性優先 |

---

## 7. 維持・進化方針

### 7.1 OTel SDK breaking change への対応

- 半年に 1 回程度の頻度で OTel Go SDK が breaking change を含む release を出す可能性あり
- CI dependabot で fail する PR が来たら、changelog 確認 → 本 unit の影響範囲評価 → 修正方針決定
- 修正は Cursor batch / Codex どちらでも可。Cursor の codebase-aware が、API 変更の伝播に強い

### 7.2 Deprecated API への対応

- OTel SDK の `// Deprecated:` 付き API は段階的に廃止
- 本 unit でも同様に GoDoc コメントで明示し、1 minor version の猶予期間

### 7.3 拡張時のチェックリスト

新しい OTel exporter (例: `otlpfile`, `otlpkafka` 等) を追加する場合:
- AGENTS.md §2 の「依存追加禁止」を再評価
- Config に新 Protocol enum 値を追加
- `buildTraceExporter` / `buildMetricExporter` / `buildLogExporter` に switch case 追加
- TP-U4-1 (MergeWith priority) のテスト範囲を新 Protocol まで拡張

---

## 8. PBT-09 (Framework Selection) compliance restatement

PBT-09 は U7 NFR-R で確定済み (`pgregory.net/rapid`)。本 unit はそれを継承し、追加 PBT framework は採用しない。
