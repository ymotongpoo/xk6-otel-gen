# Performance Test Instructions

本書は xk6-otel-gen project の **benchmark target と regression detection** を集約する。

参照: 各 unit の NFR-R `nfr-requirements.md` Performance section + `Achieved` numbers in 各 code-generation-summary.md。

---

## 1. Benchmark Inventory

| Unit | Benchmark | Target | Achieved | Strict / Soft |
|---|---|---|---|---|
| U1 | `BenchmarkParse` | ≤ 10 ms / draw | (per CG summary) | strict |
| U2 | `BenchmarkBuildPlan_Typical` | < 1 ms / op | 15.46 ns/op (64,683x under) | strict |
| U2 | `BenchmarkExecute_PureOverhead` | < 50 µs / step | 2,065 ns/step (24x under) | strict |
| U2 | `BenchmarkListJourneys` | < 10 µs / op | 66.24 ns/op | informational |
| U3 | `BenchmarkBeginSpan_HTTP_Server` | < 10 µs / op | 7,256 ns/op | strict |
| U3 | `BenchmarkRecordMetric_HTTP_Server` | < 5 µs / op | 1,531 ns/op | strict |
| U3 | `BenchmarkEmitLog` | < 10 µs / op | 1,049 ns/op | strict |
| U3 | `BenchmarkBuildResource` | < 50 µs / op | 4,658 ns/op | strict |
| U4 | `BenchmarkNew` | < 100 ms / op | 6,783,849 ns/op (~6.8 ms) | strict |
| U5 | `BenchmarkNewModuleInstance` | < 5 ms / op | 5,514 ns/op (~1000x under) | strict |
| U5 | `BenchmarkLoad` | < 50 ms / op (guidance) | 60,102 ns/op | guidance |
| U5 | `BenchmarkConfigure` | < 500 µs / op (guidance) | 3,566 ns/op | guidance |
| U6 | `BenchmarkAddMetricSamples` | < 1 µs / sample | 84.41 ns/op (0 allocs) | strict |
| U6 | `BenchmarkFlushLoop` | < 5 µs / sample | 953.4 ns/op | strict |
| U6 | `BenchmarkTagSetCache_Hit` | informational | 479 ns/op | informational |
| U6 | `BenchmarkInstrumentLookup` | informational | 29.61 ns/op (0 allocs) | informational |

> **NOTE**: "strict" は CI で regression を blocking、"informational" は monitoring 用 reference、"guidance" は user-relaxed target (NFR-R Q1=C / Q4=C 等)。

---

## 2. Running Benchmarks

### 2.1 All benchmarks

```bash
go test -bench=. -benchmem -count=1 ./...
```

### 2.2 Per-unit

```bash
go test -bench=. -benchmem ./topology/...
go test -bench=. -benchmem ./journey/...
go test -bench=. -benchmem ./synth/...
go test -bench=. -benchmem ./exporter/...
go test -bench=. -benchmem ./k6otelgen/...
go test -bench=. -benchmem ./k6output/...
```

### 2.3 Specific benchmark

```bash
go test -bench=BenchmarkAddMetricSamples -benchmem ./k6output/
go test -bench=BenchmarkBuildPlan_Typical -benchmem ./journey/
```

### 2.4 Comparison runs (regression detection)

```bash
# Baseline (e.g., main branch)
git checkout main
go test -bench=. -benchmem -count=10 ./... > /tmp/bench-main.txt

# Feature branch
git checkout feature/x
go test -bench=. -benchmem -count=10 ./... > /tmp/bench-feature.txt

# Compare with benchstat
go install golang.org/x/perf/cmd/benchstat@latest
benchstat /tmp/bench-main.txt /tmp/bench-feature.txt
```

---

## 3. Regression Detection Strategy

### 3.1 Strict benchmarks (CI blocking)

PR で strict benchmark が **NFR target を 5% 以上 regress** したら CI fail:

| Threshold | Action |
|---|---|
| 0-5% degradation | informational warning |
| > 5% degradation | CI fail |
| Improvement | informational |

実装 (CI script):
```bash
benchstat -delta-test=none baseline.txt new.txt | grep -E "^[A-Z].*[+]([5-9]|[0-9][0-9])\.[0-9]+%" && exit 1
exit 0
```

### 3.2 Informational benchmarks

regression detection なし、bench output を artifact として upload して trend graph に蓄積 (将来 enhancement)。

### 3.3 Guidance benchmarks (soft)

user-relaxed target (NFR-R Q1=C 等)。CI で fail させない、`go test -bench` 出力を review log として残す。

---

## 4. CI Performance Workflow (推奨)

`.github/workflows/bench.yml`:

```yaml
name: Benchmarks

on:
  pull_request:
  schedule:
    - cron: '0 6 * * 0'  # weekly trend

jobs:
  bench:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # need full history for baseline
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
          cache: true
      - name: Install benchstat
        run: go install golang.org/x/perf/cmd/benchstat@latest

      - name: Bench feature branch
        run: go test -bench=. -benchmem -count=5 -run=^$ ./... | tee bench-feature.txt

      - name: Checkout baseline
        run: git fetch origin main && git worktree add /tmp/baseline origin/main

      - name: Bench baseline
        run: |
          cd /tmp/baseline
          go test -bench=. -benchmem -count=5 -run=^$ ./... | tee /tmp/bench-baseline.txt

      - name: Compare
        run: benchstat /tmp/bench-baseline.txt bench-feature.txt | tee bench-compare.txt

      - name: Upload artifacts
        uses: actions/upload-artifact@v4
        with:
          name: bench-results
          path: |
            bench-feature.txt
            bench-compare.txt

      # Regression gate
      - name: Fail on strict regression
        run: |
          # Custom script to parse benchstat output, fail if strict benchmarks > 5% regression
          ./scripts/check-bench-regression.sh bench-compare.txt
```

> **NOTE**: `scripts/check-bench-regression.sh` は Build and Test stage で別途実装。strict benchmark list と threshold を encode。

---

## 5. Benchmark Best Practices

### 5.1 b.ReportAllocs()

すべての benchmark で `b.ReportAllocs()` を呼び、`allocs/op` を report。Zero-allocation hot path (U6 AddMetricSamples 等) を継続的に verify。

### 5.2 b.ResetTimer() / b.StopTimer()

setup 時間を計測から除外:
```go
func BenchmarkXxx(b *testing.B) {
    // setup (heavy)
    p := setupPipeline()
    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        p.Execute(...)
    }
}
```

### 5.3 Parallel benchmarks (`b.RunParallel`)

concurrent hot path (`exporter.Pipeline.Stats`, `synth.Synthesizer.RecordMetric`) は parallel bench で contention を verify:

```go
func BenchmarkConcurrent(b *testing.B) {
    p := setupPipeline()
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            p.Stats()
        }
    })
}
```

### 5.4 Avoid go-test-cache for benchmarks

```bash
go test -bench=. -count=1 ...  # -count=1 で cache bypass
```

---

## 6. Performance Soft Targets (NFR-R user-relaxed)

特定 unit で user が soft target (best-effort) を明示:

| Unit | Operation | NFR-R reference |
|---|---|---|
| U5 | `New()` / `NewModuleInstance` lifecycle | Q1=B "no target" |
| U5 | `Load` / `Configure` | Q2=A "guidance" |
| U6 | `Start()` | Q1=C "best-effort" |
| U6 | `Stop()` | Q4=C "best-effort" |
| U8 | `xk6-otel-gen-schema` | Q1=C "no target" |

これらは CI で **bench を実行はするが regression gate なし**、output を artifact 保存。

---

## 7. Memory Profiling

heap allocation hot spot 解析:

```bash
go test -bench=BenchmarkRecordMetric -benchmem -memprofile=mem.out ./synth/
go tool pprof -text -cum mem.out | head -30
go tool pprof -web mem.out  # interactive browser
```

primary suspects:
- `attribute.NewSet` allocation (U3 / U6 で staticSetCache が必要な理由)
- `*Span` allocation (SDK 側、本 unit からは control 困難)
- `Outcome` struct (value type、stack に乗ることが望ましい)

---

## 8. CPU Profiling

CPU bottleneck 解析:

```bash
go test -bench=BenchmarkExecute_PureOverhead -cpuprofile=cpu.out ./journey/
go tool pprof -text -cum cpu.out | head -30
```

primary suspects:
- mutex contention in `*Engine.rand` (NFR-D で per-Engine vs per-VU で議論済)
- `sync.Map.Load` overhead in tag cache / instrument cache
- `time.After` channel creation on every step

---

## 9. Common Issues

| Issue | Solution |
|---|---|
| Benchmark results noisy (CV > 10%) | `-count=10` 以上で repeat、`benchstat` で median 計算 |
| `-race` で bench が極端に slow | bench は `-race` なしで実行 (race test と分離) |
| CI worker spec によって result が変動 | self-hosted runner で固定 spec、または baseline も同 worker で再実行 |
| sync.Pool overhead が見えない | `-benchmem` で allocs/op を追跡、pool 効果は GC 統計で要 verify |
| Initial run vs cached run の差 | `-count=1` で cache bypass + warm-up run を bench 内で実施 |
