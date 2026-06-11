# Unit Test Instructions

本書は xk6-otel-gen project の **unit test 全 target と coverage 要件** を集約する。

参照: 各 unit の NFR-R `nfr-requirements.md` の DoD section。

---

## 1. Test Tool Prerequisites

| Tool | 用途 |
|---|---|
| Go 1.25+ stdlib `testing` | 基本 test framework |
| `pgregory.net/rapid` | PBT (property-based testing) |
| `github.com/stretchr/testify` | assertion helpers |
| OTel SDK test utilities (`tracetest`, `metricdata`, `logtest`) | in-memory exporter for synth tests |
| `go.k6.io/k6/js/modulestest` | k6 JS runtime mock for k6otelgen tests |

---

## 2. Quick Reference: Run All Unit Tests

```bash
# Full repo race-clean test run
go test -race -count=1 ./...

# With coverage profile
go test -race -count=1 -cover ./...

# Per-package coverage
go test -cover ./topology/...
go test -cover ./journey/...
go test -cover ./synth/...
go test -cover ./exporter/...
go test -cover ./k6otelgen/...
go test -cover ./k6output/...
go test -cover ./testutil/...
go test -cover ./cmd/...
```

---

## 3. Per-Unit Coverage Targets

| Unit | Package | Coverage target | Achieved |
|---|---|---|---|
| U1 | `topology/` | ≥ 80% | (per code-generation-summary) |
| U2 | `journey/` | ≥ 80% | 80.9% |
| U3 | `synth/` | ≥ 80% | 84.0% |
| U4 | `exporter/` | ≥ 80% | 82.5% |
| U5 | `k6otelgen/` | ≥ 80% | 82.2% |
| U6 | `k6output/` | ≥ 80% | 86.6% |
| U7 | `testutil/generators/` | ≥ 80% (PBT generator はそれ自体が test 補助) | (内部 test) |
| U8 | `cmd/xk6-otel-gen-schema/` | ≥ 70% (CLI 簡素) | 86.4% |

---

## 4. PBT (Property-Based Testing)

`pgregory.net/rapid` を使った property test。各 unit で TP-Uх-N で識別:

| Unit | TP IDs | 概要 |
|---|---|---|
| U1 | TP-U1-1..8 | Parse↔Marshal round-trip、Validate idempotency、ApplyFaults invariants 等 |
| U2 | TP-U2-1..5 | BuildPlan idempotency、cascade conditional、error.type allowed set、time monotonicity |
| U3 | TP-U3-1..4 | BuildResource idempotency、allowed attribute keys、histogram insertion、error.type required on failure |
| U4 | TP-U4-1..4 | Config merge override wins / idempotent、OTLP protobuf round-trip、Stats monotonicity |
| U5 | TP-U5-1..3 | Load idempotency、Configure merge、RunJourney ctx passed |
| U6 | TP-U6-1..3 | AddMetricSamples robustness、Counter monotonicity、tag round-trip |
| U7 | 各 generator の `Valid*`/`Any*` を他 unit が利用 | — |
| U8 | N/A | — |

実行:
```bash
go test -race ./topology/... ./journey/... ./synth/... ./exporter/... ./k6otelgen/... ./k6output/... -run Property
```

rapid default 100 iterations、必要なら `-rapid.iterations=N` で調整。

---

## 5. Race Detector

**すべての unit test を `-race` で実行する**:

```bash
go test -race -count=1 ./...
```

- `-count=1` は test result cache を bypass (CI で fresh run を保証)
- `-race` は data race を検出 — concurrency-heavy な journey/Engine、synth/Synthesizer、exporter/Pipeline、k6output/Output で特に重要

---

## 6. Parallel Test

すべての unit test は `t.Parallel()` を呼ぶ (各 unit NFR-R で確定):
- shared-immutable state (Engine / Schema / Pipeline) は安全
- shared mutable state (`exporter.ResetShared`) を必要とする test は **意図的に non-parallel** で隔離

例外:
- `k6otelgen.TestConfigure_Merge_Property`: `t.Setenv` 使用で sequential 強制
- `exporter.shared_test.go` の一部: GetShared/ResetShared に依存、`t.Parallel()` なし

---

## 7. Test Helpers Location

各 unit の test helper:

| Unit | Helper file | 内容 |
|---|---|---|
| U1 | `topology/helpers_test.go` | YAML fixture builder |
| U2 | `journey/helpers_test.go` | mockSynth + test schema builder + outcome assertions |
| U3 | `synth/helpers_test.go` | newTestProviders + mock log recorder |
| U4 | `exporter/*_test.go` 各所 | mockSpanExporter / mockMetricExporter / mockLogExporter |
| U5 | `k6otelgen/helpers_test.go` | newTestRuntime (modulestest) + mockSynth |
| U6 | `k6output/helpers_test.go` | newTestParams + newTestOutput |
| U7 | `testutil/generators/*_test.go` | self-test for generators |
| U8 | (なし) | cmd の test は self-contained |

---

## 8. CI Unit Test Workflow Snippet (推奨)

`.github/workflows/test.yml` (Build and Test stage で配置):

```yaml
name: Unit Tests

on:
  pull_request:
  push:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
          cache: true
      - name: Run unit tests with race detector and coverage
        run: go test -race -count=1 -coverprofile=coverage.out ./...
      - name: Coverage report
        run: go tool cover -func=coverage.out
      - name: Upload coverage
        uses: actions/upload-artifact@v4
        with:
          name: coverage
          path: coverage.out
```

---

## 9. Coverage Verification

### 9.1 Per-package threshold check

```bash
# All packages must meet their target
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

### 9.2 CI gate

PR 上で各 package が target coverage を下回ったら CI fail。具体 cutoff:

| Package | Threshold |
|---|---|
| `./topology/...` | 80% |
| `./journey/...` | 80% |
| `./synth/...` | 80% |
| `./exporter/...` | 80% |
| `./k6otelgen/...` | 80% |
| `./k6output/...` | 80% |
| `./cmd/...` | 70% |

実装: `gocovsh` / `gocov-html` / 自前 awk スクリプト等で per-package threshold を check。

---

## 10. Common Issues

| Issue | Solution |
|---|---|
| `go test -race` で false positive | sync.Pool 等の lock-free pattern で発生しうる、実 race ではないことを確認 |
| `rapid` shrink で `too many shrinks` | property の counter-example が複雑すぎる、generator の `Filter` を絞る |
| `t.Setenv` で parallel test fail | sequential 化を確認 |
| `modulestest` runtime init 失敗 | k6 SDK version mismatch、`go.mod` の `go.k6.io/k6` を確認 |
| OTel SDK in-memory exporter not flushing | `ForceFlush` を test 内で明示呼び出し |
