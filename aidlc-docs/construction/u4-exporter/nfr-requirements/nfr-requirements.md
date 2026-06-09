# U4 exporter — NFR Requirements

本書は U4 (`exporter/`) に適用される NFR と N/A の NFR を列挙する。U7 / U1 のスタイルを踏襲し、根拠を明示。

---

## 1. U4 に適用される NFR

### NFR-U4-1: OTel SDK 依存の明示と最小化

| 項目 | 内容 |
|---|---|
| **要件** | `exporter/` パッケージは以下の OTel SDK モジュールのみ import (Q1=A 最小セット): `otel/sdk/{trace,metric,log,resource}`, `otel/exporters/otlp/otlptrace{,grpc,http}`, `otel/exporters/otlp/otlpmetric{grpc,http}`, `otel/exporters/otlp/otlplog{grpc,http}`, `otel/attribute`。`otel/propagation` / `otel/exporters/stdout/*` は Out of Scope |
| **検証方法** | `go list -deps ./exporter/...` の出力で許可リスト外の `go.opentelemetry.io/otel/*` モジュールが含まれないこと |
| **根拠** | 依存最小化、不要モジュールでのビルドサイズ増加防止 |

### NFR-U4-2: OTel SDK バージョン方針

| 項目 | 内容 |
|---|---|
| **要件** | OTel SDK の最新 stable に追従 (Q2=A)。**開発中も継続的に最新安定版を取り込む** (利用者注記)。`go.mod` には minimum version を記載 |
| **更新手段** | (a) ローカル `go get -u go.opentelemetry.io/otel/...` (b) CI で dependabot 週次自動 PR (Build and Test ステージで `.github/dependabot.yml` 設定) |
| **採用バージョンの確定タイミング** | U4 Code Generation 開始時に `go get @latest` の結果を `go.mod` に記録。以降は dependabot で更新 |
| **Breaking change への対応** | OTel SDK 自身の major version up (例: `v1.x` → `v2.x`) は **手動評価**、minor/patch は dependabot で自動採用 |

### NFR-U4-3: Integration test (real OTel Collector)

| 項目 | 内容 |
|---|---|
| **要件** | `//go:build integration` build tag で gate された integration test を `exporter/integration_test.go` (または `test/integration/exporter/...`) に配置 (Q3=A) |
| **CI 実行** | CI で `go test -tags=integration ./...` をジョブとして走らせる (Build and Test ステージで GitHub Actions workflow に組み込む) |
| **ローカル実行** | `go test -tags=integration ./...` で明示的に走らせる。デフォルトの `go test ./...` ではスキップ |
| **Collector 起動** | テストハーネス側で Docker compose 経由で `otel/opentelemetry-collector-contrib:latest` を起動、`file_exporter` を有効化して受信内容を JSON ファイルに書き出す。compose 定義は `test/integration/exporter/docker-compose.yaml` |

### NFR-U4-4: Integration test の検証内容 + Correlation

| 項目 | 内容 |
|---|---|
| **要件 (Q4=A 最小 e2e)** | `New(Config)` 直後に 3 信号それぞれ 1 サンプル送信し、Collector の `file_exporter` 出力 JSON を読んで以下を assert: (a) traces に span 1 件、metrics に datapoint 1 件、logs に log record 1 件存在、(b) 各サンプルの `service.name` / `service.namespace` が Config の `ResourceOverrides` と一致 |
| **追加要件 (利用者注記: "可能ならcorrelationできてほしい")** | **3 信号のサンプルを同一 trace context (= 同一 `trace_id`) で発行**、Collector の出力 JSON で各サンプルの `trace_id` (または `traceId`) が一致することを assert。OTel Semantic Conventions では Metrics の exemplar、Logs の Span Context fields でこれが可能 |
| **テスト名** | `TestIntegration_ThreeSignals_Correlated`、`TestIntegration_ResourceAttributesApplied` |
| **TLS / gzip / Headers の各設定** | 本 NFR の必須スコープ外。Q4=A による「最小 e2e」を超える組み合わせは別途 Build and Test ステージの comprehensive matrix で対応 |

### NFR-U4-5: `New(Config)` 性能目標

| 項目 | 内容 |
|---|---|
| **要件** | `New(Config)` < **100 ms** (gRPC 接続確立込み、再試行なし) (Q5=A) |
| **検証方法** | `BenchmarkNew` (`exporter/bench_test.go`) で計測。loopback (Collector が localhost) で計測し中央値で評価 |
| **超過時** | プロファイル → Exporter 構築の遅延箇所特定 → OTel SDK 側の lazy connect 有効化など |

### NFR-U4-6: Steady-state スループット

| 項目 | 内容 |
|---|---|
| **要件** | 1k RPS の k6 ジャーニーシナリオで、U4 Pipeline の CPU 使用 < 10% (4 vCPU baseline)。バックプレッシャー時に span/metric drop は許容、k6 ラン全体は継続 (Q6=A) |
| **検証方法** | Build and Test ステージで実 k6 ランを使った性能評価 (integration 寄り)。U4 単体テストでは厳密に検証しない (本 NFR は目標値、CI gate ではない) |
| **drop の挙動** | drop が発生したら `Stats.*Failed` カウンタが増える。利用者は `Stats()` で観測可能 |

### NFR-U4-7: `Stats()` の性能 (目標値なし)

| 項目 | 内容 |
|---|---|
| **要件** | **目標値なし** (利用者注記: "100ms くらいかかったところでツールの性能には影響ない")。自己観測用途で数秒に 1 回呼ばれる程度を想定 |
| **実装上の方針** | 自然な実装 (per-field atomic.Load × 9) で十分。最適化作業はしない |
| **検証方法** | 簡易な assertion で 1 ms 以下を確認するだけ (実用上の sanity check)。性能 SLA としては記録せず |

### NFR-U4-8: Pipeline の並行 safety

| 項目 | 内容 |
|---|---|
| **要件** | Pipeline と Shared Holder は完全 thread-safe。複数 VU (goroutine) から `Stats()`, `TracerProvider()`, `MeterProvider()`, `LoggerProvider()`, `GetShared()` を同時呼び出ししても race なし |
| **検証方法** | `go test -race ./exporter/...` で race detector pass |
| **設計制約** | package-level mutable state は `sharedOnce`, `sharedPipeline`, `sharedErr` の 3 つのみ (Shared Holder)。すべて `sync.Once` でガードされ、初期化後は read-only |

### NFR-U4-9: ライブラリ内ログ出力なし

| 項目 | 内容 |
|---|---|
| **要件** | `exporter/` パッケージは `log` / `log/slog` を import しない (Q10=A、U1 と統一) |
| **エラー報告** | すべて戻り値の `*PipelineError` / `*ConfigError` / `*SharedError` 経由。`errors.Join` で集約 |
| **検証方法** | `go list -deps ./exporter/...` で `log` / `log/slog` が含まれないこと |

### NFR-U4-10: コードカバレッジ

| 項目 | 内容 |
|---|---|
| **要件** | U4 自身のコードカバレッジ **80% 以上** (Q8=A、U7/U1 と統一) |
| **計測範囲** | **Unit test のみ** (Q9=A)。`go test -cover ./exporter/...` (build tag なし) で 80% 以上 |
| **Integration test の coverage** | 計測対象外 (`//go:build integration` ファイルは default ビルドに含まれない) |

### NFR-U4-11: 後方互換性ポリシー

| 項目 | 内容 |
|---|---|
| **要件** | U7/U1 と同じ — v1.0.0 リリース前は破壊変更 OK、v1.0.0 以降は SemVer 厳守 (Q11=A) |
| **適用範囲** | `exporter` パッケージの public 識別子全て (`Config`, `Pipeline`, `Stats`, `New`, `MergeWith`, `ConfigFromEnv`, `GetShared`, `SetShared`, `ResetShared`, `Protocol`, 全 error types) |
| **Deprecation pattern** | `// Deprecated:` GoDoc コメント + 1 minor version の猶予 |

### NFR-U4-12: Shutdown SLA

| 項目 | 内容 |
|---|---|
| **要件** | `(*Pipeline).Shutdown(ctx)` は `ctx.Deadline()` を尊重、超過時は `context.DeadlineExceeded` を含む joined error 返却 (Q8 of FD = A) |
| **検証方法** | example-based test で deadline 即時の場合と十分長い deadline の場合を確認 |
| **多重 Shutdown** | `sync.Once` で 2 回目以降は no-op、同じ error/nil を返す |

---

## 2. N/A 一覧 (Q12=A — 明示)

### N/A: 永続化 / バックアップ / リカバリ
- **理由**: U4 は in-memory 構造のみ。Shutdown 後の状態復元は不要 (k6 ラン単発)

### N/A: 認証認可
- **理由**: 認可境界を持たない。Headers (`Authorization`) は利用者が JS API / env で渡す、U4 はそれを Collector に転送するのみ
- **補足**: Security Baseline は project 全体でオプトアウト済み (Requirements Q15=B)

### N/A: 暗号化 (at-rest)
- **理由**: 永続化なし

### N/A: 暗号化 (in-transit)
- **理由**: TLS は OTel SDK 側 (gRPC / HTTP exporter) が担う。U4 は `Insecure=true` か `false` を Config で受け取り SDK に渡すのみ

### N/A: i18n / a11y
- **理由**: 利用者向け UI を持たない。エラーメッセージは英語 (Go OSS 標準)

### N/A: GDPR / SOC2 等コンプライアンス
- **理由**: テレメトリーデータの内容は利用者が決める (U3 synth + topology)。U4 は受け取った OTLP データを Collector に転送するパススルー

### N/A: スループットの絶対値 (RPS) ターゲット
- **理由**: NFR-U4-6 で「相対的 CPU 比率」を採用、絶対 RPS は U2/U3 + 環境次第。U4 単体ではない

### N/A: production monitoring
- **理由**: U4 は library。Stats は API として露出するが、ホスト型監視サービスへの統合は U6 (k6output) の責任

### N/A: 災害復旧 (RTO / RPO)
- **理由**: ステートレスなライブラリ、SLA 対象外

### N/A: 国際化 (i18n)
- **理由**: §上記と重複 (utilities なし)

---

## 3. プロジェクト全体 NFR との traceability

`unit-of-work-traceability.md` の NFR 表を再掲し、U4 の役割を明示:

| プロジェクト NFR | U4 の役割 | 対応する U4 NFR |
|---|---|---|
| NFR-1.1 (1k RPS 持続) | Supporting | NFR-U4-5, NFR-U4-6 |
| NFR-1.2 (k6 比 30% 以内の影響) | Supporting | NFR-U4-6 |
| NFR-1.3 (非同期バッチ) | **Primary** | OTel SDK Batch{Span,Log,Periodic}Processor 活用 (FD 設計通り) |
| NFR-1.4 (バックプレッシャー時の drop) | **Primary** | NFR-U4-6 (drop 許容、Stats で観測) |
| NFR-2.1 (OTLP 未到達でも k6 継続) | **Primary** | New 失敗の fail-fast、send 失敗は Stats に記録、k6 ラン継続 |
| NFR-2.2 (設定エラー fail fast) | **Primary** | New(cfg) 内 Validate でエラー、Pipeline 返さない (FD 設計通り) |
| NFR-3.x (Compatibility) | Supporting | NFR-U4-2 (OTel SDK 最新追従) |
| NFR-4.1 (Unit + Integration test) | **Primary** | NFR-U4-3, NFR-U4-4 (integration test 実装) |
| NFR-4.2 (PBT Full) | Supporting | TP-U4-1..4 (FD で 4 properties 識別) |
| NFR-4.3 (CI seed log) | Supporting | rapid 既定挙動を尊重 |
| NFR-5.2 (内部メトリクス k6 へ) | Supporting | Stats() を U6 k6output が読んで k6 metric に変換 (U6 の責務) |

---

## 4. PBT 拡張ルール compliance summary

| ルール | 状態 | 根拠 |
|---|---|---|
| PBT-01 (Property Identification) | Compliant | FD §10 で 4 properties (TP-U4-1..4) 識別 |
| PBT-02 (Round-trip) | Compliant (U4 で実装) | TP-U4-3 (OTLP protobuf marshal/unmarshal) |
| PBT-03 (Invariants) | Compliant (U4 で実装) | TP-U4-1 (MergeWith priority), TP-U4-4 (Stats monotonic) |
| PBT-04 (Idempotency) | Compliant (U4 で実装) | TP-U4-2 (MergeWith idempotent) |
| PBT-05 (Oracle) | N/A | 参照実装が存在しない |
| PBT-06 (Stateful) | Partial (本 unit 適用) | TP-U4-4 は stateful PBT (`rapid.StateMachine` を使う可能性あり、最終形は Code Generation で確定) |
| PBT-07 (Generator Quality) | Inherits from U7 | ValidConfig / AnyConfig を本 NFR-R で U7 に追加リクエスト済み (FD §6) |
| PBT-08 (Shrinking & Reproducibility) | Inherits from U7 | rapid デフォルト + CI seed log |
| PBT-09 (Framework Selection) | Already satisfied | U7 NFR-R で確定済み (`pgregory.net/rapid`) |
| PBT-10 (Complementary) | Compliant | example-based test と PBT を別ファイル / 別関数名で分離 |

---

## 5. NFR 検証のチェックリスト (Construction 完了時)

U4 Code Generation 完了時に以下を確認:

- [ ] `go build ./...` 成功
- [ ] `go test -race -count=1 ./exporter/...` race なし (NFR-U4-8)
- [ ] `go test -cover ./exporter/...` ≥ 80% (NFR-U4-10)
- [ ] `go test -tags=integration ./...` で integration test pass (NFR-U4-3, NFR-U4-4)
- [ ] `BenchmarkNew` で `New` ≤ 100 ms (NFR-U4-5)
- [ ] `go list -deps ./exporter/...` に `log` / `log/slog` が含まれない (NFR-U4-9)
- [ ] `go list -deps ./exporter/...` に許可リスト外の OTel モジュール (propagation, stdout, ...) が含まれない (NFR-U4-1)
- [ ] すべての public 識別子に GoDoc あり (NFR-U4-11 の前提)
- [ ] `go.mod` に OTel SDK 依存が記載されている、最新 stable を採用 (NFR-U4-2)
- [ ] Integration test に correlation 検証 (3 信号同一 trace_id assert) が含まれている (NFR-U4-4)
- [ ] U7 への ValidConfig / AnyConfig 追加リクエスト (FD §6) が U4 Code Generation Phase で実装される
