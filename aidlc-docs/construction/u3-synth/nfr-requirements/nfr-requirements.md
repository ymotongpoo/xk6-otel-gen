# U3 synth — Non-Functional Requirements

本書は U3 (`synth/`) の **測定可能な非機能要件** を確定する。各 NFR は ID 付き、検証手段とテスト可能な閾値を持つ。

参照:
- FD: `aidlc-docs/construction/u3-synth/functional-design/`
- Plan + Answers: `aidlc-docs/construction/plans/u3-synth-nfr-r-plan.md`
- Tech stack: `tech-stack-decisions.md` (本ディレクトリ内)

---

## 1. Applicable NFR (本 unit で扱う)

### NFR-U3-1: API Stability (Q12=A)

| 項目 | 内容 |
|---|---|
| 要件 | post-v1 公開後は SemVer 1.0.0 厳守 |
| 検証 | `go vet ./synth/...` + 公開 API surface の API check tool (`apidiff` 等) |
| 閾値 | breaking change → major bump、addition → minor、bug fix → patch |
| 公開 API | `Synthesizer` interface, `SpanInput`, `MetricInput`, `LogInput`, `Outcome`, `FinishSpanFunc`, `NewDefault`, `BuildResource` |

### NFR-U3-2: Construction Lifecycle

| 項目 | 内容 |
|---|---|
| 要件 | `NewDefault(tp, mp, lp)` は 1 回の呼び出しで完全な Synthesizer を返す。再構築は新規 instance で行う |
| 検証 | `synth_test.go::TestNewDefault_*` で確認 |
| 副要件 | 構築後の Synthesizer は immutable (provider 参照のみ保持) |

### NFR-U3-3: Concurrency (Q1=A の thread-safe + Stateless)

| 項目 | 内容 |
|---|---|
| 要件 | `Synthesizer` は複数 goroutine から同時呼び出し可能 |
| 検証 | `go test -race -count=1 ./synth/...` パス、`TestSynthesizer_ConcurrentBeginSpan` 等で並行性 explicit テスト |
| 不変条件 | (1) `defaultSynthesizer` 構造体に可変フィールドなし、(2) OTel SDK の Tracer/Meter/Logger 公式 thread-safety に依存 |

### NFR-U3-4: Error Contract (Q4=A + Q5=A + Q6=A)

| 入力 | 振る舞い | 検証 |
|---|---|---|
| `NewDefault` の引数いずれか nil | panic with descriptive message | `TestNewDefault_NilPanic` (recover で確認) |
| `BeginSpan` の `in.Service == nil` | panic | `TestBeginSpan_InvalidInputPanic` |
| `BeginSpan` の `in.Operation == ""` | panic | 同上 |
| `BeginSpan` の `in.InstanceIdx < 0 \|\| >= svc.Replicas` | panic | 同上 |
| `RecordMetric` の同様の不正 input | panic | 同上 |
| `EmitLog` の `in.Service == nil` | panic | 同上 |
| `FinishSpanFunc` 2 回目以降の呼び出し | no-op (silent ignore) + `-race` build で panic | `TestFinishSpan_DoubleCall_NoOp` + `TestFinishSpan_DoubleCall_RacePanic` |
| `BuildResource` の `svc == nil` | panic | `TestBuildResource_NilPanic` |
| `BuildResource` の `instanceIdx < 0` | panic | 同上 |

すべて programmer error として **fail-fast**。production code が caller 責任を遵守する前提。

### NFR-U3-5: Resource Building Determinism

| 項目 | 内容 |
|---|---|
| 要件 | `BuildResource(svc, idx)` は同じ入力で常に同じ attribute set を返す (TP-U3-1 Idempotency) |
| 検証 | PBT TP-U3-1、`rapid.Check` で 100+ iteration |
| `service.instance.id` | UUID v5 (SHA-1 namespace-based)、namespace は package-local 固定 UUID |
| 性能 | < 50 µs / call (UUID v5 計算込み、Q1=A) |

### NFR-U3-6: Performance (Q1=A + Q7=A)

| Operation | p99 latency target | アロケーション目安 | 測定 |
|---|---|---|---|
| `BeginSpan` (full path) | < 10 µs | < 256 B (SDK 側 span allocation を除く) | `BenchmarkBeginSpan` |
| `RecordMetric` | < 5 µs | < 64 B | `BenchmarkRecordMetric` |
| `EmitLog` | < 10 µs | < 128 B | `BenchmarkEmitLog` |
| `BuildResource` | < 50 µs | < 1 KB | `BenchmarkBuildResource` |
| `FinishSpanFunc` | < 5 µs | < 64 B | `BenchmarkFinishSpan` |

Instrument は **eager 生成** (Q7=A) — `NewDefault` 内で全 9 Histogram/UpDownCounter (4 namespace × {client,server duration} + 1 active gauge) を作る。Hot path 上の `meter.Histogram(name)` 呼び出しを避ける。

#### Throughput (Q2=A)

明示的 throughput target は持たない (k6 ワークロードと CPU に依存)。per-call latency の確保で間接的に保証する。

### NFR-U3-7: Observability (Q3=A)

| 項目 | 内容 |
|---|---|
| 要件 | U3 自身は **self-metric を持たない** (`synth.Stats` 構造体なし) |
| 理由 | Provider 経由で emit された span/metric/log は U4 の `exporter.Stats` で観測可能、重複計装は冗長 |
| 検証 | `synth/*.go` に `atomic.*` import / package-level mutable state が存在しないことの code review |
| 将来再評価 | k6 high-concurrency でボトルネック発生時、limited self-instrumentation を追加検討 |

### NFR-U3-8: Semantic Conventions Conformance

| 項目 | 内容 |
|---|---|
| 採用 version | `go.opentelemetry.io/otel/semconv/v1.27.0` (FD §1, Q1=B) |
| 適用範囲 | HTTP / RPC / DB / Messaging × {Client, Server, Producer, Consumer} の主要 attribute key |
| 検証 | PBT TP-U3-2: 全 span attribute key が semconv constants + 既知 custom namespace (`synth.service.framework`) に含まれる |
| Bump プロトコル | `attributes.go` の import path を変更、grep+replace、TP-U3-2 の allowed key set 更新 |
| Project alignment | U4 (`exporter/`) と整合 — both units 同じ semconv version を import 可 (`u4-exporter/nfr-requirements/tech-stack-decisions.md` §1.2 ですでに許可済) |

### NFR-U3-9: Documentation

| 項目 | 内容 |
|---|---|
| GoDoc 網羅性 | 全 exported identifier に doc comment 必須。`revive` 等の lint で CI enforce |
| Example function | 3 件 (Q10=A): `ExampleNewDefault`, `ExampleBuildResource`, `ExampleSynthesizer_BeginSpan` |
| Package overview | `doc.go` に usage walkthrough (NewDefault → BeginSpan → finishFunc → Shutdown は U4 経由) |
| Semantic Conventions hint | `attributes.go` のコメントで参照 semconv version 明記 |

### NFR-U3-10: Testability

| 項目 | 内容 |
|---|---|
| Coverage target | ≥ 80% (`go test -cover ./synth/...`, Q9=A) |
| Mock provider | OTel SDK 公式 `tracetest`, `metricdata` package を使用 (Q8=A) |
| PBT 適用 | TP-U3-1〜4 を実装 (`pgregory.net/rapid` 使用) |
| `t.Parallel()` | 全 unit test で適用 (shared state なし) |
| Integration test | `synth/integration/` + `-tags=integration` (Q11=A) — U4 Pipeline と組み合わせ end-to-end correlation 検証 |
| Bench | `BenchmarkBeginSpan` / `BenchmarkRecordMetric` / `BenchmarkEmitLog` / `BenchmarkBuildResource` を NFR-U3-6 の閾値で CI regression check |

### NFR-U3-11: Compatibility

| 項目 | 内容 |
|---|---|
| Go version | Go 1.25+ (U4 / U1 と整合) |
| OTel Go SDK | latest stable (dependabot で自動 PR、`tech-stack-decisions.md` 参照) |
| semconv | `v1.27.0` (bump は明示的に行う、`attributes.go` 1 ファイルに局所化) |
| backwards compatibility | post-v1 SemVer 厳守、API addition は minor で OK |

### NFR-U3-12: PBT (Property-Based Testing) Compliance

Extension "Property-Based Testing — Full enforcement" 適合性:

| PBT Rule | 適用状況 |
|---|---|
| PBT-01 (Property Identification) | ✅ FD で 4 properties 文書化 (TP-U3-1..4) |
| PBT-02 (Round-trip Tests) | ⚠️ U3 では明示的な round-trip なし、U4 の TP-U4-3 が project レベルで担保 |
| PBT-03 (Invariant Tests) | ✅ TP-U3-2 (allowed keys)、TP-U3-3 (histogram insertion)、TP-U3-4 (error.type required) |
| PBT-04 (Idempotency Tests) | ✅ TP-U3-1 (BuildResource) |
| PBT-05 (Metamorphic Tests) | N/A (U3 のドメインは metamorphic 関係を持つ計算をしない) |
| PBT-06 (Stateful Tests) | ⚠️ `active_requests` UpDownCounter の +1/-1 平衡は stateful PBT で検証可能 — 実装時に追加検討 |
| PBT-07 (Generator Quality) | ✅ U7 generators (FD §6 で 8 funcs request 済) |
| PBT-08 (Shrinking Strategy) | ✅ rapid 標準 shrinking |
| PBT-09 (Test Performance) | ✅ rapid default 100 iterations、CI 5 min budget 内 |

---

## 2. Non-Applicable NFR (Q13=A)

| カテゴリ | 理由 |
|---|---|
| Persistence (DB / file storage) | N/A — U3 は stateless、永続化対象なし |
| Authentication / Authorization | N/A — library として exported、auth boundary を持たない |
| Encryption at rest | N/A — 永続化なし |
| Encryption in transit | N/A — U4 Pipeline (TLS via OTel SDK) で担保 |
| Internationalization (i18n) | N/A — library、localized string なし。log Body は英語固定 |
| Accessibility (a11y) | N/A — UI なし |
| GDPR / SOC2 / PCI | N/A — 合成データのみ、real PII を扱わない |
| Production monitoring SLO | N/A — service ではなく library |
| Disaster recovery | N/A — stateless |
| Capacity planning | N/A — service ではなく library。caller (k6 + U5/U6) が responsible |
| Multi-region / Multi-AZ | N/A — process-local |
| Backup / Restore | N/A — stateless |

---

## 3. Project NFR トレーサビリティ

`aidlc-docs/inception/requirements/requirements.md` の Project レベル NFR への対応:

| Project NFR | U3 で対応する項目 |
|---|---|
| R-NFR-001 (Performance: load test ツール vibe を阻害しない) | NFR-U3-6 per-call latency budget |
| R-NFR-002 (Reliability: 不正入力で k6 を crash させない) | NFR-U3-4 (programmer error は panic、ただし入力経路 = U2 経由なので E2E では入力 validate がかかる) |
| R-NFR-003 (Observability: telemetry 自身が観測可能) | NFR-U3-7 (U4 Stats に委譲) |
| R-NFR-004 (Maintainability: PBT 全面適用) | NFR-U3-12 PBT compliance |
| R-NFR-005 (Compatibility: OTel SDK 最新追従) | NFR-U3-11 |

---

## 4. Definition of Done (DoD)

U3 の Code Generation 完了条件:

- [ ] `go build ./synth/...` succeeds
- [ ] `go vet ./synth/...` clean
- [ ] `go test -race -count=1 ./synth/...` passes
- [ ] `go test -cover ./synth/...` shows ≥ 80%
- [ ] `BenchmarkBeginSpan` < 10 µs / op, `BenchmarkRecordMetric` < 5 µs / op, `BenchmarkEmitLog` < 10 µs / op, `BenchmarkBuildResource` < 50 µs / op
- [ ] PBT properties TP-U3-1〜4 all pass
- [ ] All exported identifiers have GoDoc
- [ ] 3 Example functions present and passing
- [ ] U7 generators (`ValidSpanInput` / `AnySpanInput` / `ValidMetricInput` / `AnyMetricInput` / `ValidLogInput` / `AnyLogInput` / `ValidOutcome` / `AnyOutcome` + optional `ValidErrorType`) added to `testutil/generators/`
- [ ] `golangci-lint run ./synth/...` passes
- [ ] Integration test (`-tags=integration ./synth/integration/...`) passes against Docker Collector with 3-signal trace_id correlation
- [ ] No package-level mutable state in `synth/` (excluding test files)

---

## 5. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| OTel SDK の breaking change (semconv attribute name 変更等) | semconv version pinned、bump 時は `attributes.go` 1 ファイル + TP-U3-2 update |
| Histogram bucket boundary が k6 load test 帯域に合わない | NFR Design で再評価、SDK default bucket を採用してから bench で確認 |
| Multi-replica InstanceIdx の Engine 側未実装 | U2 Journey Engine FD で `SpanInput.InstanceIdx` を含むことを明示 (Application Design §C3 修正済) |
| `process.runtime.name` の semantic 解釈不一致 | NFR Design で対応の確定 (`svc.Language` → `process.runtime.name`)、U3 unit doc に明記 |
| Histogram per-call allocation がボトルネック | OTel SDK の `RecordOption` 再利用 + `attribute.Set` cache (NFR Design で検討) |

---

## 6. 関連他 unit への要求

| 依頼先 | 内容 |
|---|---|
| U2 (journey) | `SpanInput.InstanceIdx` を埋める責務 (Engine が replica 選択)、`Outcome.ErrorType` semconv 準拠 string を渡す責務 |
| U4 (exporter) | Provider accessor (`TracerProvider()`, `MeterProvider()`, `LoggerProvider()`) を提供 (済) |
| U7 (testutil) | FD §6 の 8〜9 個 generator 追加 (Q12=C; ValidErrorType は実装時判断) |
| U5 (k6 module) | `synth.NewDefault(tp, mp, lp)` を構築 (U4 から取得した Provider を注入) |
