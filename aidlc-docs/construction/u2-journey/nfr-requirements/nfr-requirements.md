# U2 journey — Non-Functional Requirements

本書は U2 (`journey/`) の **測定可能な非機能要件** を確定する。各 NFR は ID 付き、検証手段とテスト可能な閾値を持つ。

参照:
- FD: `aidlc-docs/construction/u2-journey/functional-design/`
- Plan + Answers: `aidlc-docs/construction/plans/u2-journey-nfr-r-plan.md`
- Tech stack: `tech-stack-decisions.md` (本ディレクトリ内)

---

## 1. Applicable NFR (本 unit で扱う)

### NFR-U2-1: API Stability (Q12=A)

| 項目 | 内容 |
|---|---|
| 要件 | post-v1 公開後 SemVer 1.0.0 厳守 |
| 公開 API | `NewEngine`, `(*Engine).BuildPlan`, `(*Engine).Execute`, `(*Engine).ListJourneys`, `Plan`, `Node`, `Outcome`, `PlanError`, `ExecuteError`, `AllowedErrorTypes` |
| 互換性ルール | Outcome 構造体への field 追加は backward-compatible (Go の struct literal で field name 必須にできれば minor で OK) |
| 検証 | `apidiff` 等の API check tool (CI) |

### NFR-U2-2: Engine Lifecycle (Q7=A)

| 項目 | 内容 |
|---|---|
| 構築 | `NewEngine(schema, overlay, syn)` で全 journey の Plan を **eager build** |
| 構築失敗 | journey に cycle 検出 / unknown reference 等 → `NewEngine` が panic (programmer error / Schema は事前 Validate 済前提) |
| Plan cache | `Engine.plans map[string]*Plan` で保持、construct 後 read-only |
| Execute 失敗 | step-level failure は Outcome に閉じ込め、Execute 戻り値 error は programmer error のみ |
| 検証 | `engine_test.go::TestNewEngine_BuildsAllPlans`, `TestNewEngine_UnknownJourneyPanics` |

### NFR-U2-3: Concurrency (Q4=A + Q5=A)

| 項目 | 内容 |
|---|---|
| 要件 | 複数 VU goroutine から同じ `*Engine` を並行 Execute 可能、race-free |
| 検証 | `go test -race -count=1 ./journey/...`、`TestExecute_ParallelVUs` 等 |
| 内部 state | Plan / overlay / schema は immutable、`*rand.Rand` のみ可変 |
| Random source 戦略 | per-Engine 単一 `*rand.Rand` + `sync.Mutex` (NFR Design で per-VU instance への切り替えを bench 結果次第で検討) |

### NFR-U2-4: Context Cancellation (Q5=A)

| 項目 | 内容 |
|---|---|
| 要件 | `ctx.Done()` を受けたら **10 ms 以内** に Execute が return |
| 中断時の outcome | sleep 中の step: `Outcome.Success=false, ErrorType="context_canceled"` |
| Span close 保証 | `defer finishFn(...)` で必ず close、open span を残さない |
| 後続 step の扱い | cascade として scan、全て `Cascaded=true` outcome を produce (実行はしない、span だけ emit して trace 視認性確保) |
| 検証 | `executor_test.go::TestExecute_CtxCancel_StopsWithin10ms`、span open状態の double-check |

### NFR-U2-5: Panic Recovery (Q6=A)

| 項目 | 内容 |
|---|---|
| 要件 | Engine 内部 / synth 呼び出し / topology 操作のいずれの panic も `Execute` 外には伝播しない |
| 中断時の outcome | 現 step の Outcome を `Success=false, ErrorType="internal_error"` で埋める |
| 戻り値 | `Execute` は `*ExecuteError{Kind: "internal", Inner: <recovered>}` を返す (panic 化せず error として通知) |
| Span close 保証 | `defer recover { defer finishFn(internalErrOutcome) }` パターン |
| 検証 | `executor_test.go::TestExecute_PanicInSynth_RecoversAndReturns` |

### NFR-U2-6: Performance (Q1=A + Q2=A)

| Operation | Target | 検証 |
|---|---|---|
| `BuildPlan(name)` | < 1 ms (typical journey: 深さ ≤ 5, ops ≤ 20) | `BenchmarkBuildPlan_Typical` |
| `Execute` per-step **pure overhead** (synth + Sleep を除く) | < 50 µs / step (p99) | `BenchmarkExecute_PureOverhead` (mock synth + zero Sleep) |
| `ListJourneys()` | < 10 µs (sort 済 map keys を返すのみ) | `BenchmarkListJourneys` |

Engine インスタンスメモリ: < 1 MB (典型 topology: 20 services, 50 edges, 5 journeys)。

#### Performance の観点

Hot path 上の処理:
1. `executeNode` 呼び出し overhead (関数 dispatch + Plan tree pointer traversal)
2. Fault overlay lookup (現状 map lookup × 4 種、< 1 µs/lookup 期待)
3. `rand.IntN` / `rand.Float64` 呼び出し (per-Engine mutex 競合がボトルネックになる場合 NFR Design で per-VU rand に切り替え)
4. Outcome struct allocation (< 200 B / step 期待)
5. Synth call (U3 で 7-10 µs 計測済、本 unit の budget には含めない)

### NFR-U2-7: Observability (Q3=A)

| 項目 | 内容 |
|---|---|
| 要件 | Engine 自身は **self-metric を持たない** |
| 理由 | journey 実行による span/metric/log は synth 経由で emit 済、Engine の cascade/fault hit 回数は test で確認 |
| 検証 | `journey/*.go` に `atomic.*` import / package-level mutable counter が存在しないことを code review で確認 |
| 将来 | k6 高並列で debug 必要時、Engine.Stats を opt-in で追加検討 |

### NFR-U2-8: Documentation

| 項目 | 内容 |
|---|---|
| GoDoc 網羅性 | 全 exported identifier に doc comment 必須、`revive` で CI enforce |
| Example function | 3 件 (Q10=A): `ExampleNewEngine`, `ExampleEngine_BuildPlan`, `ExampleEngine_Execute` |
| Package overview | `doc.go` に Journey lifecycle (NewEngine → BuildPlan → Execute) + cascade / recovery / fault の semantic 説明 |
| Outcome field の semantic | 各 field に GoDoc で「いつ true / non-nil になるか」を明記 |

### NFR-U2-9: Testability (Q8=A + Q9=A + Q11=A)

| 項目 | 内容 |
|---|---|
| Coverage target | ≥ 80% (`go test -cover ./journey/...`) |
| Mock synth | `helpers_test.go` で自前 mock struct (synth.Synthesizer interface を簡素に実装、call log を atomic-safe で保持) |
| PBT 適用 | TP-U2-1〜5 (FD §10) を `pbt_test.go` で実装 (`pgregory.net/rapid`) |
| `t.Parallel()` | 全 unit test で適用 (Engine は shared-immutable なので安全) |
| Integration test | `journey/integration/` + `-tags=integration` — 実 U4 Pipeline + 実 U3 Synthesizer + 実 topology YAML で end-to-end correlation 確認、cascade パターン (`OnExhausted=propagate` + child failure) の trace 構造を verify |
| Bench | `BenchmarkBuildPlan` / `BenchmarkExecute_PureOverhead` を NFR-U2-6 閾値で CI regression check |

### NFR-U2-10: Compatibility

| 項目 | 内容 |
|---|---|
| Go version | Go 1.25+ (U3/U4 と整合) |
| `math/rand/v2` | Go 1.22+ の `rand/v2` を採用 (modern API、生成器の interface 改善) |
| OTel Go SDK | 直接 import しない (synth interface 経由)、互換は U3 / U4 で担保 |
| topology / synth 依存 | latest commit を direct import (replace directive 不使用) |
| backwards compatibility | post-v1 SemVer 厳守 (NFR-U2-1) |

### NFR-U2-11: PBT (Property-Based Testing) Compliance

Extension "Property-Based Testing — Full enforcement" 適合性:

| PBT Rule | 適用状況 |
|---|---|
| PBT-01 (Property Identification) | ✅ FD で 5 properties 文書化 (TP-U2-1..5) |
| PBT-02 (Round-trip Tests) | N/A (U2 は signal を emit するだけ、round-trip 対象なし) |
| PBT-03 (Invariant Tests) | ✅ TP-U2-2 (all ops visited), TP-U2-3 (cascade conditional), TP-U2-4 (error.type allowed), TP-U2-5 (time monotonicity) |
| PBT-04 (Idempotency Tests) | ✅ TP-U2-1 (BuildPlan) |
| PBT-05 (Metamorphic Tests) | N/A (Engine は journey 構造を変えずに実行するのみ、metamorphic relation を定義しにくい) |
| PBT-06 (Stateful Tests) | ⚠️ Cascade flow / Recovery flow は stateful PBT 候補。実装時に難易度判断 |
| PBT-07 (Generator Quality) | ✅ U7 generators (FD §6 で 3 pairs request 済) |
| PBT-08 (Shrinking Strategy) | ✅ rapid 標準 shrinking |
| PBT-09 (Test Performance) | ✅ rapid default 100 iterations、CI 5 min budget 内 |

---

## 2. Non-Applicable NFR (Q13=A)

| カテゴリ | 理由 |
|---|---|
| Persistence | N/A — U2 は stateless、永続化対象なし |
| Authentication / Authorization | N/A — library として exported、auth boundary を持たない |
| Encryption at rest / in transit | N/A — 永続化なし、in-transit は U4 Pipeline (TLS) が担保 |
| Internationalization (i18n) | N/A — library、localized string なし |
| Accessibility (a11y) | N/A — UI なし |
| GDPR / SOC2 / PCI | N/A — 合成データのみ |
| Production monitoring SLO | N/A — service ではなく library |
| Disaster recovery | N/A — stateless |
| Capacity planning | N/A — library、caller (k6 + U5) の責務 |
| Multi-region / Multi-AZ | N/A — process-local |
| Backup / Restore | N/A — stateless |

---

## 3. Project NFR トレーサビリティ

| Project NFR | U2 で対応する項目 |
|---|---|
| R-NFR-001 (Performance: load test ツール vibe を阻害しない) | NFR-U2-6 per-step overhead < 50µs |
| R-NFR-002 (Reliability: 不正入力で k6 を crash させない) | NFR-U2-5 panic recovery、NFR-U2-4 ctx cancel |
| R-NFR-003 (Observability: telemetry 自身が観測可能) | NFR-U2-7 (no self-metric, delegate to synth/U4) |
| R-NFR-004 (Maintainability: PBT 全面適用) | NFR-U2-11 PBT compliance |
| R-NFR-005 (Compatibility: OTel SDK 最新追従) | NFR-U2-10 (synth/U4 経由で間接的に追従) |

---

## 4. Definition of Done (DoD)

U2 の Code Generation 完了条件:

- [ ] `go build ./journey/...` succeeds
- [ ] `go vet ./journey/...` clean
- [ ] `go test -race -count=1 ./journey/...` passes
- [ ] `go test -cover ./journey/...` shows ≥ 80%
- [ ] `BenchmarkBuildPlan < 1ms / op`, `BenchmarkExecute_PureOverhead < 50µs / step`
- [ ] PBT properties TP-U2-1〜5 all pass
- [ ] All exported identifiers have GoDoc
- [ ] 3 Example functions present + passing
- [ ] U7 generators (`ValidPlan` / `AnyPlan` / `ValidNode` / `AnyNode` / `ValidEngineOutcome` / `AnyEngineOutcome`) added to `testutil/generators/`
- [ ] `golangci-lint run ./journey/...` passes
- [ ] Integration test passes against Docker Collector with cascade pattern verification
- [ ] No package-level mutable state in `journey/` (excluding test files)

---

## 5. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| `*rand.Rand` mutex 競合が k6 高並列で bottleneck | NFR Design で bench 後に per-VU rand instance への切り替え検討 |
| Plan 構造が巨大な topology で memory hungry | Plan immutable & cached → 1 Plan / journey で済む、深さ制限を validate に追加検討 |
| Cascade の trace 表現が分かりにくい | span attribute `synth.cascaded=true` を明示、doc.go で例示 |
| Recovery flow の test combinatorics 爆発 | PBT で代表 sequence、unit test で各 OnExhausted mode を個別 |
| Fault overlay の lookup API 変更 (U1) | U1 FaultOverlay API は変更しない (FD で固定済), 変更時は API stability 議論 |

---

## 6. 関連他 unit への要求

| 依頼先 | 内容 |
|---|---|
| U1 (topology) | `FaultOverlay.LookupCrash` / `LookupDisconnect` / `LookupErrorRate` / `LookupLatencyInflation` の API 明確化 (FD で言及、NFR Design で正確な signature を確認) |
| U3 (synth) | Synthesizer interface (既出)、`SpanInput.InstanceIdx` / `MetricInput.InstanceIdx` を Engine が埋める前提 (U3 FD §10 で約束済) |
| U7 (testutil) | FD §6 の 3 pairs generator 追加 (`ValidPlan/AnyPlan` 等) |
| U5 (k6 module) | `NewEngine(schema, overlay, synth)` を構築、k6 init phase で 1 回呼び出し、各 VU iteration で `Execute(ctx, plan)` を呼ぶ |
