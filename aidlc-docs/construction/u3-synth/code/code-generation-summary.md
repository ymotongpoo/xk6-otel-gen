# U3 synth — Code Generation Summary

## Files

| Area | Files | Lines |
|---|---:|---:|
| `synth/` production | 7 | 851 |
| `synth/` unit/property/example/bench tests | 8 | 1,442 |
| `synth/integration/` | 3 | 256 |
| `synth/testdata/` | 3 | 39 |
| `testutil/generators/` synth additions | 2 | 473 |

Total counted lines: 3,126.

## Verification Results

| Check | Result |
|---|---|
| `go build ./...` | PASS |
| `go vet ./synth/...` | PASS |
| `go test -race -count=1 ./...` | PASS |
| `go test -cover ./synth/...` | PASS — 84.0% |
| `golangci-lint run ./synth/...` | PASS |
| `go test -tags=integration ./synth/integration/...` | PASS |
| Source `TODO(agent)` markers in `synth/`, `testutil/generators/`, `topology/`, `exporter/` | none |

## Benchmarks

Command: `go test -bench=. -benchmem ./synth/...`

| Benchmark | Result | Budget |
|---|---:|---:|
| `BenchmarkBuildResource` | 4,658 ns/op, 1,224 B/op, 8 allocs/op | < 50,000 ns/op |
| `BenchmarkBeginSpan_HTTP_Server` | 7,256 ns/op, 4,699 B/op, 24 allocs/op | < 10,000 ns/op |
| `BenchmarkRecordMetric_HTTP_Server` | 1,531 ns/op, 896 B/op, 10 allocs/op | < 5,000 ns/op |
| `BenchmarkEmitLog` | 1,049 ns/op, 448 B/op, 1 alloc/op | < 10,000 ns/op |

## Deviations

- Phase 0 semconv verification used `go list go.opentelemetry.io/otel/semconv/v1.27.0` instead of grepping `go.sum`, because semconv is a package path within the existing `go.opentelemetry.io/otel` module.
- U3 implementation maps the plan's older `EdgeKind` wording to the actual `topology.Protocol` field already implemented by U1.
- Semconv v1.27.0 uses `DBOperationNameKey` and `MessagingOperationNameKey`; tests and allowed-key checks use those actual constants.
- `synth/pbt_test.go` is an external `synth_test` package after Phase 11 to avoid an import cycle once `testutil/generators` imports `synth`.
- Collector output files under `synth/testdata/otel-logs/` are ignored from `synth/testdata/.gitignore` because integration tests remove and recreate that directory.

## Recent Commits

```text
97fb53a docs(synth): reconcile FD with NFR-D — drop errors.go from file layout
d472676 test(synth): add integration test harness with 3-signal correlation
1d630a9 feat(testutil): add synth IO generators for U3 PBT
8c83661 test(synth): add benchmarks for per-call latency targets
3f9d232 test(synth): add PBT for BuildResource idempotency and allowed attribute keys
c9cdf2c docs(synth): add package documentation and Example functions
e6a0a4d feat(synth): add EmitLog with default Body fallback and service.name auto-attribute
e966851 feat(synth): add RecordMetric with hybrid static+dynamic attribute strategy
3a7c03f feat(synth): add BeginSpan with FinishSpanFunc double-call protection and active_requests tracking
3a241b9 feat(synth): add defaultSynthesizer skeleton with eager instrument creation
e2e9e9c feat(synth): add attribute policy mapping and static/dynamic builders
1537b5e feat(synth): add Resource builder with deterministic service.instance.id
cbf4873 feat(synth): add Synthesizer interface and IO types
df5213c build(synth): add uuid dependency for U3
cbbe718 docs(synth): add code generation plan for U3
```
