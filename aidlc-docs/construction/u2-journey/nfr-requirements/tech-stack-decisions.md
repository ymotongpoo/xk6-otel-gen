# U2 journey — Tech Stack Decisions

本書は U2 (`journey/`) が依存するパッケージ・採用された代替案・却下された案を確定する。

---

## 1. 依存モジュール (Production code)

### 1.1 採用一覧 (Go module)

| Module | Version | 用途 | 必要性 |
|---|---|---|---|
| `context` (stdlib) | Go 1.25 | ctx propagation, Done() cancel | 必須 |
| `sync` (stdlib) | Go 1.25 | WaitGroup, Mutex | 必須 |
| `time` (stdlib) | Go 1.25 | Sleep / time.After / time.Now / time.Duration | 必須 |
| `math/rand/v2` (stdlib) | Go 1.22+ | fault probability / replica idx (`rand.IntN`, `rand.Float64`) | 必須 |
| `github.com/ymotongpoo/xk6-otel-gen/topology` | (local) | Schema / Service / Edge / Operation / Journey / FaultOverlay / RecoveryPolicy 型参照 | 必須 |
| `github.com/ymotongpoo/xk6-otel-gen/synth` | (local) | Synthesizer interface, SpanInput / MetricInput / LogInput / Outcome / FinishSpanFunc | 必須 |

### 1.2 採用しないモジュール (production)

- `golang.org/x/sync/errgroup` — sync.WaitGroup で十分、外部依存追加を回避
- `go.opentelemetry.io/otel/*` — synth interface 経由で抽象化、U2 は OTel SDK を直接知らない
- `github.com/ymotongpoo/xk6-otel-gen/exporter` — synth 経由で provider 注入、U2 は U4 を import しない
- `math/rand` (v1) — `math/rand/v2` を採用 (modern API、`rand.IntN` の signature が直感的)

### 1.3 検証

`go list -deps ./journey/...` の出力に `topology`, `synth`, stdlib 以外の external module が含まれないことを CI で sanity check。

---

## 2. テスト依存 (Test-only)

| Module | Version | 用途 |
|---|---|---|
| `pgregory.net/rapid` | latest stable | PBT (TP-U2-1〜5) |
| `github.com/stretchr/testify` | latest stable | assertion (`require.*`, `assert.*`) |
| `github.com/ymotongpoo/xk6-otel-gen/testutil/generators` | (local) | ValidSchema / ValidService / Plan generators / etc. |

### 2.1 Mock synth strategy (Q8=A)

`journey/helpers_test.go` に自前 mock struct を定義:

```go
type mockSynth struct {
    mu       sync.Mutex
    spans    []spanCall
    metrics  []metricCall
    logs     []logCall
}

func (m *mockSynth) BeginSpan(ctx context.Context, in synth.SpanInput) (context.Context, synth.FinishSpanFunc) {
    m.mu.Lock()
    m.spans = append(m.spans, spanCall{in: in})
    m.mu.Unlock()
    return ctx, func(o synth.Outcome) { /* record finish */ }
}
// RecordMetric / EmitLog 同様
```

理由:
- U3 NewDefault を test で構築すると OTel SDK の test utility (tracetest) を経由する必要があり、test がやや重い
- Engine の振る舞いを test するには synth call の args を確認できれば十分
- mock は thread-safe (Mutex 保護)

---

## 3. Integration test 依存

`journey/integration/` で使用:
- Docker Engine + `docker compose`
- `otel/opentelemetry-collector-contrib:<pinned-tag>` (U3 / U4 と同 tag)
- `exporter` (U4) — 実 Pipeline 構築
- `synth` (U3) — 実 Synthesizer

`-tags=integration` build tag で default `go test` から除外。

---

## 4. Version 戦略

### 4.1 Go toolchain

- `go.mod`: `go 1.25`
- U1 / U3 / U4 / U7 と整合

### 4.2 stdlib `math/rand/v2`

Go 1.22+ で導入。今後 Go 標準として fix される予定なので `v2` を採用。`v1` の `math/rand` は global state の rand source 共有問題があり、`v2` で改善されている。

### 4.3 transitive dependency

- `go mod tidy` で管理、`replace` directive は使わない
- vendor なし、`go.sum` で integrity 確保

### 4.4 dependabot

`go.opentelemetry.io/otel/*` は U3/U4 経由で間接依存、direct dependabot は U2 では不要。

---

## 5. 代替案 (Rejected)

### 5.1 `golang.org/x/sync/errgroup` (rejected)

- 案: parallel branch 実行に errgroup を使う (context 連動 cancel + error 集約)
- 却下理由: 外部依存追加。sync.WaitGroup + 各 child goroutine 内で ctx チェックする方法で同等の機能を実現可能。child の "error" は Outcome として記録するので errgroup の error aggregation は不要

### 5.2 Worker pool (rejected)

- 案: 固定 N worker goroutine で全 Parallel branch を処理
- 却下理由: 大量 fan-out (Parallel branch > 100) は本ツールの想定 use case 外。シンプルな goroutine-per-branch で十分。将来 fan-out が極端な topology が来たら NFR Design で worker pool 検討

### 5.3 `math/rand` v1 (rejected)

- 案: stdlib の `math/rand` (v1) を使う
- 却下理由: `rand/v2` が modern (`rand.IntN` の signature 改善、ChaCha8 algorithm 等)、Go 1.25 で v2 が標準的な選択肢

### 5.4 SDK concrete 型を直接 import (rejected)

- 案: Engine が `*sdktrace.TracerProvider` 等を直接受け取る
- 却下理由: 抽象化が崩れる、U3 (synth) を経由する設計を尊重

### 5.5 Generics 化した Engine (rejected)

- 案: `Engine[T any]` で signal 型を generics に
- 却下理由: メリット不明、Synthesizer interface で十分

### 5.6 Outcome 値の channel return (rejected)

- 案: Execute が `<-chan Outcome` を返し、JS 側で streaming 受信
- 却下理由: k6 JS API として overcomplicated、Outcome は signal として U4 経由で観測すれば十分

### 5.7 Plan を YAML として serialize (rejected)

- 案: BuildPlan の結果を YAML で persist して再利用
- 却下理由: Plan は topology から決定論的に build できる、persist する必要性がない

---

## 6. CI / Lint 統合

### 6.1 必須 CI ジョブ

| ジョブ | コマンド | DoD blocking? |
|---|---|---|
| Build | `go build ./journey/...` | Yes |
| Unit test (race) | `go test -race -count=1 ./journey/...` | Yes |
| Coverage | `go test -cover ./journey/...` ≥ 80% | Yes |
| Bench (regression) | `go test -bench=. -benchmem ./journey/...` vs baseline | Yes |
| Lint | `golangci-lint run ./journey/...` | Yes |
| `go vet` | `go vet ./journey/...` | Yes |
| Integration (Docker) | `go test -tags=integration ./journey/integration/...` | nightly + manual trigger |

### 6.2 lint rules

`.golangci.yml` で project 共通設定 (U3/U4 と同じ):
- `revive` (GoDoc 網羅)
- `govet`
- `staticcheck`
- `errcheck`
- `unused`

---

## 7. Cross-unit dependency summary

```text
U2 (journey) imports:
  - context, sync, time, math/rand/v2 (stdlib)
  - github.com/ymotongpoo/xk6-otel-gen/topology
  - github.com/ymotongpoo/xk6-otel-gen/synth

U2 does NOT import:
  - exporter/ (U4)         — synth 経由
  - go.opentelemetry.io/otel/*  — synth 経由
  - errgroup, worker pool 等  — sync.WaitGroup で十分

U2 is imported by:
  - k6otelgen/ (U5)        — NewEngine / BuildPlan / Execute を呼ぶ
  - testutil/generators/   — ValidPlan / ValidNode / ValidEngineOutcome
```

---

## 8. Migration / Upgrade Notes

### 8.1 math/rand/v2 from v1

U1/U3/U4 がまだ `math/rand` (v1) を使っている場合、U2 で `v2` を新規採用する。`v1` と `v2` は co-exist 可能 (両方 import OK)。将来 project 全体を `v2` に統一する場合、`go fix` 系ツールで auto-migrate 可能。

### 8.2 synth.Synthesizer interface 変更時

- U3 が `Synthesizer` interface を変更する場合 (新規 method 追加等)、U2 の mockSynth と Engine 実装も対応
- interface に新 method が増えた場合、Engine.execute の 呼び出し site も更新
- backwards compatibility は U3 NFR-U3-1 で保証

### 8.3 topology FaultOverlay API 変更時

- U1 (topology) が FaultOverlay.LookupX API を変更する場合、Engine.fault.go の lookup 呼び出し site も更新
- U2 FD §6 で言及した lookup API (LookupCrash / LookupDisconnect / LookupErrorRate / LookupLatencyInflation) は U1 実装と coordinate

---

## 9. Open questions for Future revisit

| 質問 | 想定 trigger |
|---|---|
| `*rand.Rand` mutex 競合 in high-concurrency | NFR-U2-6 bench で latency p99 が target を超えた場合 |
| Stateful PBT (PBT-06) for cascade / recovery sequence | TP-U2-1〜5 で十分か実装時に再評価 |
| `error_rate_override` の ErrorType field を topology YAML に追加 | error.type taxonomy を user-extensible にしたい場合 |
| Replica weighted distribution | per-step uniform random の代わりに weighted を要望されたら |
| Sticky session strategy | sticky session 模擬の要望が出たら |
| `latency_inflation` の jitter (random delta) | NFR Design で標準偏差 / 分布 noise 追加検討 |
