# U6 k6output — Non-Functional Requirements

本書は U6 (`k6output/`) の **測定可能な非機能要件** を確定する。各 NFR は ID 付き、検証手段とテスト可能な閾値を持つ。

> **NOTE on Performance budgets** (per Q1=C, Q4=C user-relaxed): U6 の Start / Stop latency 目標はおおむね「期待値 / monitoring 用 reference」として位置づけ、CI で blocking する閾値ではない。Per-sample overhead (Q2/Q3) のみ厳密 target を持つ — k6 hot path で観測されるため。

参照:
- FD: `aidlc-docs/construction/u6-k6output/functional-design/`
- Plan + Answers: `aidlc-docs/construction/plans/u6-k6output-nfr-r-plan.md`
- Tech stack: `tech-stack-decisions.md` (本ディレクトリ内)

---

## 1. Applicable NFR (本 unit で扱う)

### NFR-U6-1: API Stability (Q8=A)

| 項目 | 内容 |
|---|---|
| 要件 | `--out otel-gen=<args>` の args syntax は post-v1 公開後 SemVer 厳守 |
| Args key 追加 | minor bump |
| 既存 key の意味変更 / 削除 | major bump |
| Go-side API | `func New(params output.Params) (output.Output, error)` のみ strict public (k6 SDK contract) |
| 検証 | unit test で各 args key の decode + warn-ignore for unknown |

### NFR-U6-2: Output Lifecycle

| 項目 | 内容 |
|---|---|
| `New(params)` | args parse のみ、heavy init なし。失敗時 *ConfigError 返却 → k6 が `--out` 設定エラーとして拒否 |
| `Start()` | Pipeline (lazy via GetShared) + runner MeterProvider + instrument + flush goroutine 起動。失敗時 error 返却 → k6 run abort (Q8=A) |
| `AddMetricSamples` | queue push (non-blocking)、Start 前 / Stop 後の呼び出しは no-op |
| `Stop()` | sync.Once で 1 回のみ。flush drain → Pipeline.Shutdown(ctx)。**戻り値は常に nil** (k6 lifecycle 保護) |
| 検証 | `output_test.go::TestLifecycle_*` |

### NFR-U6-3: Performance — Per-sample overhead (Q2=A + Q3=A)

| Operation | Target | 性質 |
|---|---|---|
| `AddMetricSamples` per-sample queue push | **< 1 µs / sample** | strict target (CI bench で監視) |
| flush goroutine per-sample emit (lookup + Record) | **< 5 µs / sample** | strict target (CI bench で監視) |

k6 typical throughput (1000 req/sec × 5 samples/req = 5000 samples/sec) で:
- AddMetricSamples: 5 ms/sec を消費 (< 0.5% CPU)
- flush emit: 25 ms/sec を消費 (< 2.5% CPU)

→ k6 そのものの実行に殆ど影響を与えない。

### NFR-U6-4: Performance — Lifecycle latency (Q1=C + Q4=C, soft targets)

| Operation | Soft target | 性質 |
|---|---|---|
| `New(params)` | < 100 µs (guidance) | args parse のみ |
| `Start()` | < 100 ms (guidance, Q1=C "目安でよい") | Pipeline 構築含む |
| `Stop()` | < 30 sec (guidance, Q4=C "目安でよい") | Pipeline.Shutdown timeout 上限 |

CI で blocking 閾値ではない、bench で regression がない範囲で OK。

### NFR-U6-5: Memory Footprint (Q5=X formula-based)

固定 cap ではなく **入力規模に依存した formula で見積もる**:

```text
Memory(U6) ≈ Base + (queueCapacity × ~10KB) + (N_instruments × ~1KB) + (N_attributeSets × ~100B)

where:
  Base               ≈ 1 KB (Output struct + runner Resource + small fields)
  queueCapacity      = `--out otel-gen=queueSize=N` で調整可能、default 100
  N_instruments      ≈ ~50 (k6 の standard metric 数)
  N_attributeSets    = sample 内の unique tag combinations 数 (k6 script 依存)
```

### 5.1 典型ワークロード

| Workload | N_attributeSets | Memory estimate |
|---|---|---|
| Low cardinality (1 URL pattern) | 10-50 | ~1.5 MB |
| Medium cardinality (10 URL patterns × 5 status × 4 methods) | ~200 | ~2 MB |
| High cardinality (1000 unique URLs) | ~1000 | ~2 MB |
| Pathological (no `name` tag normalization, 100k unique URLs) | ~100,000 | ~12 MB |

→ 通常用途で memory は問題にならない。pathological case は Q13=A 通り Collector 側で制御 (cardinalitylimit processor)。

### 5.2 検証

`BenchmarkMemoryFootprint` で 各 cardinality レベルの allocation を bench。formula と一致するかを確認。

### NFR-U6-6: Queue Configuration (Q7=A + user clarification)

| 項目 | 内容 |
|---|---|
| Default queue size | 100 |
| Configurable | `--out otel-gen=queueSize=N` で調整可能 |
| Range | [10, 10000] (NFR Design で validate) |
| Queue full handling | drop oldest sample + warn log (per second rate-limited) |
| Drop tracking | internal `drops_total` counter (debug-time inspection、self-metric として expose しない per Q6=A) |
| 影響範囲 | U6 内部 memory のみ。他 unit (U2/U3/U4/U5) に影響なし。Pipeline shutdown timing にも影響なし (flush ticker は 1s 固定) |

### NFR-U6-7: Concurrency

| 項目 | 内容 |
|---|---|
| 要件 | Start() / AddMetricSamples / Stop() の concurrent 呼び出しが race-free |
| 検証 | `go test -race -count=1 ./k6output/...`、queue channel + sync.Once で保護 |
| Flush goroutine と Stop の race | sync.Once + cancel ctx + done channel で deterministic shutdown |

### NFR-U6-8: Observability (Q6=A)

| 項目 | 内容 |
|---|---|
| 要件 | `*Output` 自身の self-metric は持たない (U2/U3/U5 と同方針) |
| Pipeline.Stats 経由 | U4 の Stats で送信側を観測 |
| internal drops counter | atomic counter としては持つが exported API なし (debug log のみ) |

### NFR-U6-9: Documentation

| 項目 | 内容 |
|---|---|
| GoDoc 網羅性 | 全 Go exported identifier に doc comment |
| Example function | 1 件 (Q10=A): `ExampleNew` |
| `--out` usage example | `doc.go` の package comment に shell example を含む |
| Args reference | `--out otel-gen=<args>` の各 key を doc.go の table で documentation |

### NFR-U6-10: Testability (Q9=A + Q11=A)

| 項目 | 内容 |
|---|---|
| Coverage target | ≥ 80% (`go test -cover ./k6output/...`) |
| Unit test mock | k6 SDK の `output.Params` を test 内で構築、`output.Output` interface implementation を直接呼ぶ |
| PBT 適用 | TP-U6-1〜3 を `pbt_test.go` で実装 |
| `t.Parallel()` | 全 unit test で適用 (各 test で独立 *Output instance) |
| Integration test | `k6output/integration/` + `-tags=integration` — xk6 で k6 binary build + 実 `--out otel-gen=...` で run + Collector で k6.* metric 受信確認 |

### NFR-U6-11: Compatibility

| 項目 | 内容 |
|---|---|
| Go version | Go 1.25+ (U1-U5 と整合) |
| k6 output SDK | latest stable (U5 と同 version pin) |
| OTel Go SDK | 直接 import (`go.opentelemetry.io/otel/sdk/metric`) — 独自 MeterProvider 構築のため |
| backwards compatibility | post-v1 SemVer 厳守 (NFR-U6-1) |

### NFR-U6-12: PBT (Property-Based Testing) Compliance

Extension "Property-Based Testing — Full enforcement" 適合性:

| PBT Rule | 適用状況 |
|---|---|
| PBT-01 (Property Identification) | ✅ FD で 3 properties 文書化 (TP-U6-1..3) |
| PBT-02 (Round-trip Tests) | N/A (signal を emit するだけ) |
| PBT-03 (Invariant Tests) | ✅ TP-U6-1 robustness, TP-U6-2 monotonic, TP-U6-3 tag round-trip |
| PBT-04 (Idempotency Tests) | N/A (k6 SDK lifecycle で driving、Output は state machine) |
| PBT-05 (Metamorphic Tests) | N/A |
| PBT-06 (Stateful Tests) | ⚠️ TP-U6-1 は state transition を扱うので stateful 寄り、`rapid.Run` で実装可だが scope 外 |
| PBT-07 (Generator Quality) | ✅ U7 generators (`ValidK6Sample`, `ValidOutputParams`) |
| PBT-08 (Shrinking Strategy) | ✅ rapid 標準 |
| PBT-09 (Test Performance) | ✅ rapid default 100 iterations |

### NFR-U6-13: Cardinality Strategy (Q13=A)

| 項目 | 内容 |
|---|---|
| 要件 | U6 内部で attribute set cardinality safeguard を持たない |
| User 責務 | k6 script で `tags` を制限する (e.g., `name` tag を URL pattern 化) |
| Operator 責務 | OTel Collector の processors (e.g., `cardinalitylimitprocessor`) で global cap を設定 |
| Doc.go warning | high-cardinality risk について明記 |

---

## 2. Non-Applicable NFR (Q12=A)

| カテゴリ | 理由 |
|---|---|
| Persistence | N/A — k6 lifecycle で state 持つのみ、永続化なし |
| Authentication / Authorization | N/A — library として exported |
| Encryption at rest | N/A — 永続化なし |
| Encryption in transit | N/A — U4 Pipeline (TLS) が担保 |
| Internationalization (i18n) | N/A — k6 metric name は固定英語 |
| Accessibility (a11y) | N/A — UI なし |
| GDPR / SOC2 / PCI | N/A — 合成データ + k6 自身の test execution metrics のみ |
| Production monitoring SLO | N/A — library |
| Disaster recovery | N/A — stateless across runs |
| Multi-region | N/A — process-local |
| Backup / Restore | N/A — stateless |
| Capacity planning | N/A — k6 user の責務、`queueSize` で調整可能 |

---

## 3. Project NFR トレーサビリティ

| Project NFR | U6 で対応する項目 |
|---|---|
| R-NFR-001 (Performance) | NFR-U6-3 strict per-sample target、NFR-U6-4 soft lifecycle target |
| R-NFR-002 (Reliability) | NFR-U6-2 lifecycle (Stop always returns nil)、NFR-U6-6 queue full graceful drop |
| R-NFR-003 (Observability) | NFR-U6-8 (no self-metric, delegate to U4 Stats) |
| R-NFR-004 (Maintainability: PBT) | NFR-U6-12 |
| R-NFR-005 (Compatibility) | NFR-U6-11 |

---

## 4. Definition of Done (DoD)

U6 の Code Generation 完了条件:

- [ ] `go build ./k6output/...` succeeds
- [ ] `go vet ./k6output/...` clean
- [ ] `go test -race -count=1 ./k6output/...` passes
- [ ] `go test -cover ./k6output/...` shows ≥ 80%
- [ ] `BenchmarkAddMetricSamples` < 1 µs / sample
- [ ] `BenchmarkFlushLoop` < 5 µs / sample
- [ ] PBT TP-U6-1..3 pass
- [ ] All Go exported identifiers have GoDoc
- [ ] 1 Example function present (`ExampleNew`)
- [ ] `--out` args documentation in doc.go
- [ ] U7 generators (`ValidK6Sample`, `AnyK6Sample`, `ValidOutputParams`, `AnyOutputParams`) added
- [ ] `golangci-lint run ./k6output/...` passes
- [ ] Integration test (`-tags=integration ./k6output/integration/...`) passes — xk6 build + Docker Collector で `k6.*` metric 受信確認
- [ ] **U4 patch (`Pipeline.MetricExporter()`)** が landed

---

## 5. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| 高 cardinality k6 script で attribute set explosion | NFR-U6-13 で user/operator 責務として明示、doc.go で警告 |
| queue 100 では burst で drop 多発 | `--out otel-gen=queueSize=N` で調整可能 (NFR-U6-6) |
| Pipeline.Shutdown が 30s timeout 内に終わらない | warn log + return nil で k6 lifecycle 保護 |
| flush loop が Stop 後も走る | sync.Once + cancel ctx + done channel で deterministic shutdown |
| k6 SDK が major bump で Output interface 変更 | dependabot で監視、breaking 時手動対応 |

---

## 6. 関連他 unit への要求

| 依頼先 | 内容 |
|---|---|
| U4 (exporter) | **NEW**: `Pipeline.MetricExporter() sdkmetric.Exporter` accessor 追加 (minor SemVer bump)。U6 が独自 MeterProvider + runner Resource を構築するため必要 |
| U7 (testutil) | FD §6.2 の 2 pairs generator 追加 |
| U5 (k6otelgen) | U5 integration test に U6 build dependency を反映 (現状 U5 が U6 absence を guard している) |
