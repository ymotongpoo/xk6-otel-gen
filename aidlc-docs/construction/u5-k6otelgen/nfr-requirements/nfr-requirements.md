# U5 k6otelgen — Non-Functional Requirements

本書は U5 (`k6otelgen/`) の **測定可能な非機能要件** を確定する。各 NFR は ID 付き、検証手段とテスト可能な閾値を持つ。

> **NOTE on Performance budgets** (per Q1=B, Q2=A relaxed, Q4=C): U5 の latency target はおおむね「期待値 / monitoring 用 reference」として位置づけ、CI で blocking する閾値ではない。U5 は thin frontend layer なので、k6 iteration の実体的なボトルネックは U2/U3/U4 にある。明示閾値が緩めでも全体 NFR に影響しない。

参照:
- FD: `aidlc-docs/construction/u5-k6otelgen/functional-design/`
- Plan + Answers: `aidlc-docs/construction/plans/u5-k6otelgen-nfr-r-plan.md`
- Tech stack: `tech-stack-decisions.md` (本ディレクトリ内)

---

## 1. Applicable NFR (本 unit で扱う)

### NFR-U5-1: API Stability (Q12=A)

| 項目 | 内容 |
|---|---|
| 要件 | JS API surface は post-v1 公開後 SemVer 厳守 |
| JS public surface | `configure(opts)`, `load(path)`, `stats()`, `journeys()` (top-level); `handle.runJourney(name)`, `handle.journeys()` |
| opts decode rules | `business-rules.md` §4 の 10-field mapping は契約、field 追加は minor、field 削除/型変更は major |
| Stats field names | `tracesExported`, `tracesFailed`, ..., `logsFailed` の 6 field は契約 |
| Go public surface | `New()`, `(*RootModule).NewModuleInstance(vu)`, `(*ModuleInstance).Exports()` の k6 SDK contract のみが strict public; その他 method (`Load`, `Configure`, `Stats`, `Journeys`, `RunJourney`) は testability のため public だが SemVer Go-API も維持 |
| 検証 | `apidiff` 等 (Go side) + JS API integration test (sobek 経由) |

### NFR-U5-2: ConfigError.Kind SemVer (Q7=A)

| 項目 | 内容 |
|---|---|
| Kind enum 値 | `"already_loaded" | "already_configured" | "not_loaded" | "path_mismatch" | "file_not_found" | "parse_error" | "validate_error"` の 7 値 |
| 互換性ルール | 新 Kind 追加 → minor bump、既存 Kind の意味変更/削除 → major bump |
| JS-side 露出 | error.message 内に `[<Kind>]` 文字列を含めるなど grep 可能な形式で (NFR Design で具体的に確定) |
| 検証 | unit test で各 Kind が正しい場面で生成されることを確認 |

### NFR-U5-3: Process Singleton Lifecycle (Q1=B, Q4=C)

| 項目 | 内容 |
|---|---|
| Init phase 動作 | `New()` は zero-init のみ、heavy work なし。明示 latency target は **設定しない** (Q1=B、k6 init で 1 回のみ) |
| Load 動作 | sync.Once 保護、同 path で idempotent、別 path で `*ConfigError{Kind: "already_loaded"}` |
| Configure 動作 | sync.Once 保護、2 回目で `*ConfigError{Kind: "already_configured"}` |
| 検証 | `module_test.go::TestSingleton_LoadIdempotent`, `TestSingleton_ConfigureSingleShot` |

### NFR-U5-4: Per-VU Lifecycle (Q3=A)

| 項目 | 内容 |
|---|---|
| `NewModuleInstance(vu)` latency | < 5 ms (per VU、VU=1000 で total < 5 s、k6 startup の許容範囲) |
| Per-VU memory | < 200 KB (NFR-U5-7) |
| Random seed | per-VU 独立 (NFR Design で `time.UnixNano() XOR vu.VUID` 等の戦略を確定) |
| 検証 | `BenchmarkNewModuleInstance`、`go test -race` で VU 並列に instance 構築 |

### NFR-U5-5: Concurrency (Q3=A + Q4=C)

| 項目 | 内容 |
|---|---|
| 要件 | 複数 VU goroutine からの `Exports()` 経由 JS API 呼び出しが race-free |
| 検証 | `go test -race -count=1 ./k6otelgen/...`、`TestParallel_VUs_NoRace` |
| 内部 state | RootModule の write は sync.Once 保護、ModuleInstance は per-VU 独立、TopologyHandle は per-VU bound |

### NFR-U5-6: Performance (soft targets per Q1=B, Q2=A relaxed, Q4=C)

各 latency は **monitoring target** であり、CI blocking 閾値ではない。bench で大幅 regression が観測された場合のみ調査:

| Operation | Soft target | 備考 |
|---|---|---|
| `init()` / `New()` | no target | Q1=B (one-shot, zero-init) |
| `configure(opts)` | < 500 µs (guidance) | opts decode + merge |
| `load(path)` | < 50 ms (guidance) | YAML Parse + Validate + ApplyFaults |
| `getOrBuildPipeline()` (lazy 初回) | < 100 ms | NFR-U4-6 と整合 |
| `NewModuleInstance(vu)` | < 5 ms (target) | per-VU 構築、Q3=A |
| `runJourney(name)` overhead (excl. Execute) | "数 ms まで許容" (Q4=C) | 厳密 target なし、k6 user の許容範囲内 |
| `stats()` / `journeys()` | < 10 µs (guidance) | simple snapshot |

### NFR-U5-7: Memory Usage (Q5=A)

| 項目 | 内容 |
|---|---|
| Per-VU `*ModuleInstance` overhead | < 200 KB (target) |
| Process-singleton `*RootModule` + shared Pipeline | < 10 MB (target、NFR-U4 と整合) |
| Total VU=1000 想定 | < 200 MB k6otelgen 起因 |
| 検証 | `BenchmarkNewModuleInstance` で `b.ReportAllocs()` 確認 |

### NFR-U5-8: Observability (Q6=A)

| 項目 | 内容 |
|---|---|
| 要件 | U5 自身は **self-metric を持たない** |
| 理由 | Pipeline.Stats は U4 で、k6 native metrics は U6 で、U5 は thin frontend |
| 検証 | `k6otelgen/*.go` に `atomic.*` counter import がないことを code review で確認 |

### NFR-U5-9: Documentation

| 項目 | 内容 |
|---|---|
| GoDoc 網羅性 | 全 Go exported identifier に doc comment |
| JS API documentation | `doc.go` 内に JS-side usage example を埋め込む (setup / iteration / teardown の典型 pattern) |
| `--out` warning | `doc.go` の冒頭で **`k6 run --out otel-gen=...`** 推奨を明示 (NFR-U5-11 と連動) |
| Example function | 2 件 (Q10=A): `ExampleNew`, `ExampleRootModule_NewModuleInstance` |

### NFR-U5-10: Testability (Q9=A + Q10=A + Q11=A)

| 項目 | 内容 |
|---|---|
| Coverage target | ≥ 80% (`go test -cover ./k6otelgen/...`) |
| JS runtime mock | `go.k6.io/k6/js/modulestest.NewRuntime(t)` を使用 (NFR Design で specific helper 構築) |
| PBT 適用 | TP-U5-1〜3 を `pbt_test.go` で実装 |
| `t.Parallel()` | unit test で適用 (sync.Once 系は test 内 fresh RootModule で隔離) |
| Integration test | `k6otelgen/integration/` + `-tags=integration` — 実 k6 binary を build + 実 OTel Collector で end-to-end |
| `--out otel-gen=...` 依存 | Integration test は必ず `--out` を使用 (Pipeline shutdown 経路の verify 含む) |

### NFR-U5-11: Pipeline Shutdown Dependency (FD §11)

| 項目 | 内容 |
|---|---|
| 要件 | U5 は Pipeline.Shutdown を呼ばない。`exporter.GetShared` で取得した Pipeline は U6 (k6output) の `Output.Stop()` がライフサイクル管理 |
| User-facing risk | k6 を `--out otel-gen=...` なしで run すると Shutdown が呼ばれず未送信 batch が lost する可能性 |
| Mitigation | `doc.go` + JS-side error 文言で警告、Integration test は必ず `--out` 使用 |
| 検証 | manual review of `k6otelgen/*.go` に `pipeline.Shutdown(...)` 呼び出しがないこと |

### NFR-U5-12: Filesystem Access (Q8=A)

| 項目 | 内容 |
|---|---|
| 要件 | `load(path)` は k6 SDK の filesystem sandbox に従う。U5 内部で path traversal 拒否等の追加 check はしない |
| 理由 | k6 自体が `--allow-list` / `--blacklist-ip` 等で制限を提供、U5 内重複は冗長 |
| 検証 | `module_test.go::TestLoad_RelativePath_OK`, `TestLoad_AbsolutePath_OK` |

### NFR-U5-13: Compatibility

| 項目 | 内容 |
|---|---|
| Go version | Go 1.25+ (U1〜U4 と整合) |
| k6 SDK | latest stable (NFR Design + tech-stack で pin version 確定) |
| sobek (JS runtime) | k6 推移依存に従う (direct import 不要、k6 が引っ張る) |
| backwards compatibility | post-v1 SemVer 厳守 (NFR-U5-1) |

### NFR-U5-14: PBT (Property-Based Testing) Compliance

Extension "Property-Based Testing — Full enforcement" 適合性:

| PBT Rule | 適用状況 |
|---|---|
| PBT-01 (Property Identification) | ✅ FD で 3 properties 文書化 (TP-U5-1..3) |
| PBT-02 (Round-trip Tests) | N/A (U5 は JS frontend、round-trip 対象なし) |
| PBT-03 (Invariant Tests) | ✅ TP-U5-2 (Configure merge), TP-U5-3 (RunJourney ctx) |
| PBT-04 (Idempotency Tests) | ✅ TP-U5-1 (Load same path → same handle) |
| PBT-05 (Metamorphic Tests) | N/A |
| PBT-06 (Stateful Tests) | N/A (k6 lifecycle test は integration で扱う) |
| PBT-07 (Generator Quality) | ✅ U7 generators (`ValidConfigureOpts`, `ValidLoadPath` 等) |
| PBT-08 (Shrinking Strategy) | ✅ rapid 標準 |
| PBT-09 (Test Performance) | ✅ rapid default 100 iterations |

---

## 2. Non-Applicable NFR (Q13=A)

| カテゴリ | 理由 |
|---|---|
| Persistence | N/A — `load()` は YAML を 1 回読み込むのみ、state は in-memory |
| Authentication / Authorization | N/A — library として exported |
| Encryption at rest | N/A — 永続化なし |
| Encryption in transit | N/A — U4 Pipeline (TLS) が担保 |
| Internationalization (i18n) | N/A — error.message は英語、log も英語 |
| Accessibility (a11y) | N/A — UI なし |
| GDPR / SOC2 / PCI | N/A — 合成データのみ |
| Production monitoring SLO | N/A — library |
| Disaster recovery | N/A — stateless across runs |
| Multi-region | N/A — process-local |
| Backup / Restore | N/A — stateless |
| Capacity planning | N/A — k6 / U5 user の責務 |

---

## 3. Project NFR トレーサビリティ

| Project NFR | U5 で対応する項目 |
|---|---|
| R-NFR-001 (Performance) | NFR-U5-6 (soft target で運用、blocker にしない) |
| R-NFR-002 (Reliability) | NFR-U5-3 singleton lifecycle, NFR-U5-11 shutdown delegation warning |
| R-NFR-003 (Observability) | NFR-U5-8 (no self-metric, delegate to U4 / U6) |
| R-NFR-004 (Maintainability: PBT) | NFR-U5-14 |
| R-NFR-005 (Compatibility) | NFR-U5-13 |

---

## 4. Definition of Done (DoD)

U5 の Code Generation 完了条件:

- [ ] `go build ./k6otelgen/...` succeeds
- [ ] `go vet ./k6otelgen/...` clean
- [ ] `go test -race -count=1 ./k6otelgen/...` passes
- [ ] `go test -cover ./k6otelgen/...` shows ≥ 80%
- [ ] PBT TP-U5-1〜3 pass
- [ ] All Go exported identifiers have GoDoc
- [ ] 2 Example functions present + passing
- [ ] U7 generators (`ValidConfigureOpts/AnyConfigureOpts`, `ValidLoadPath/AnyLoadPath`) added
- [ ] `golangci-lint run ./k6otelgen/...` passes
- [ ] Integration test (`-tags=integration ./k6otelgen/integration/...`) passes — 実 k6 binary build (xk6 経由) + Docker Collector で 1 journey 実行、Collector JSON で trace 受信を確認
- [ ] `doc.go` に `--out otel-gen=...` 推奨 warning を明示

---

## 5. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| k6 SDK の major bump で modules.VU 等の signature 変更 | NFR Design で k6 SDK version pin、bump 時は U5 修正 phase を別 PR で |
| `--out` 未指定で Pipeline.Shutdown 不実行 | `doc.go` 明示 warning + integration test の usage example |
| sobek の value 変換で precision loss (number → int64) | NFR Design で型 check を慎重に、test で large number / negative / NaN を扱う |
| 大規模 topology (services > 100) で load latency 超過 | guidance を超えても blocking しない (Q2=A relaxed) |
| Per-VU instance 構築 latency が VU=1000+ で k6 startup を遅延 | NFR-U5-4 < 5 ms target を bench で監視 |

---

## 6. 関連他 unit への要求

| 依頼先 | 内容 |
|---|---|
| U2 (journey) | `Engine.Execute(ctx, plan) error` (済)、`Engine.BuildPlan(name)` cache (済) |
| U3 (synth) | `NewDefault(tp, mp, lp) Synthesizer` (済) |
| U4 (exporter) | `Config / GetShared / Pipeline.TracerProvider() / MeterProvider() / LoggerProvider() / Stats()` (済) |
| U1 (topology) | `Parse(yaml) / Schema.Validate() / Schema.ApplyFaults()` (済) |
| U7 (testutil) | FD §7 の 2 pairs generator 追加 |
| U6 (k6output) | `Output.Stop()` で `exporter.GetShared` の Pipeline を取得して `Shutdown(ctx)` を呼ぶ責務 (U6 unit で確定) |
