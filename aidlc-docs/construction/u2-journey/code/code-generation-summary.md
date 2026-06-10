# U2 (journey) — Code Generation Summary

## Files

| File | Lines |
|---|---:|
| `journey/bench_test.go` | 118 |
| `journey/doc.go` | 28 |
| `journey/doc_test.go` | 67 |
| `journey/engine.go` | 66 |
| `journey/engine_test.go` | 108 |
| `journey/errors.go` | 86 |
| `journey/errors_test.go` | 104 |
| `journey/executor.go` | 338 |
| `journey/executor_test.go` | 251 |
| `journey/fault.go` | 159 |
| `journey/fault_test.go` | 195 |
| `journey/helpers_test.go` | 128 |
| `journey/integration/README.md` | 11 |
| `journey/integration/helpers.go` | 215 |
| `journey/integration/integration_test.go` | 134 |
| `journey/pbt_test.go` | 335 |
| `journey/plan.go` | 152 |
| `journey/plan_test.go` | 169 |
| `journey/recovery.go` | 57 |
| `journey/recovery_test.go` | 198 |
| `journey/replica.go` | 26 |
| `journey/replica_test.go` | 62 |
| `journey/testdata/collector-config.yaml` | 27 |
| `journey/testdata/docker-compose.yaml` | 11 |
| `testutil/generators/journey_inputs.go` | 144 |
| `testutil/generators/journey_inputs_test.go` | 113 |

`journey/` total: 3045 lines. U7 journey generator additions: 257 lines.

## Verification

| Check | Result |
|---|---|
| `go build ./...` | pass |
| `go vet ./journey/...` | pass |
| `go test -race -count=1 ./...` | pass |
| `go test -cover ./journey/...` | pass, 80.9% |
| `golangci-lint run ./journey/...` | pass |
| `go test -tags=integration ./journey/integration/...` | pass, 7.029s |
| Source `TODO(agent)` markers in `journey/`, `synth/`, `testutil/generators/`, `exporter/`, `topology/` | none |

Benchmarks from `go test -bench=. -benchmem ./journey/...`:

```text
BenchmarkBuildPlan_Typical-4       78834302   15.46 ns/op       0 B/op    0 allocs/op
BenchmarkExecute_PureOverhead-4       36734   30978 ns/op    2065 ns/step 30893 B/op 90 allocs/op
BenchmarkListJourneys-4            17997741   66.24 ns/op      80 B/op    1 allocs/op
```

Budgets met: BuildPlan < 1ms/op, Execute pure overhead < 50us/step.

## Deviations and Adaptations

- Go cannot express a constant slice, so `journey.AllowedErrorTypes` is an exported read-only-by-convention slice with the specified 16 values.
- U1 `topology.SeverityParams` uses `Value`, `Add`, and `Multiplier` rather than `Rate`, `Delay`, or `ErrorType`. U2 maps `Value` to error-rate override, uses `Add` plus a multiplier-derived latency component for latency inflation, and infers the default error type from the node protocol/service kind.
- Multi-step journeys are represented with a virtual sequential root (`Service == nil`, `Children != nil`). Single-step journeys keep the concrete operation as the root.
- U3 was coordinated per Phase 7: `synth.Outcome.Cascaded` was added and `FinishSpanFunc` emits `synth.cascaded=true`.
- Collector file exporter output is append-oriented, so integration tests wait for expected trace content before asserting correlation.

## Recent Commits

```text
afa070e fix(journey): clean final verification lint findings
b7865fe test(journey): add integration test harness with cascade and recovery verification
9078767 test(journey): add benchmarks for BuildPlan and Execute pure overhead
0a1d9fd test(journey): add PBT for TP-U2-1..5
e421417 feat(testutil): add journey IO generators for U2 PBT
8f0e984 docs(journey): add package documentation and Example functions
ec298b1 feat(journey): add recovery flow with OnExhausted modes and cascade span emission
c1637e8 feat(synth): add Outcome.Cascaded marker for journey engine integration
88744e1 feat(journey): add Executor with sequential/parallel dispatch, ctx cancel, and panic recovery
d7dc3b5 feat(journey): add FaultOverlay adapter with precedence folding and latency sampling
61ed2aa feat(journey): add per-Engine random source with mutex protection
2f4720a feat(journey): add Plan/Node types and BuildPlan DFS algorithm
15406db feat(journey): add Engine struct with NewEngine and ListJourneys
bed227b feat(journey): add typed error hierarchy and AllowedErrorTypes constants
d391953 build(journey): scaffold package for U2
2483d6c docs(plan): record U2 FD plan answers (14× A)
eb8c0d3 docs(journey): add code generation plan for U2
f23fe8a docs(journey): add NFR design patterns and logical components for U2
7111053 docs(journey): add NFR requirements and tech stack decisions for U2
b585d7e docs(journey): add functional design for U2 with cascade and recovery contracts
```
