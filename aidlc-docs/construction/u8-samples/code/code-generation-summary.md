# U8 Samples & Distribution — Code Generation Summary

## File List

| Path | Lines |
|---|---:|
| `.claude/skills/sync-astroshop/SKILL.md` | 67 |
| `cmd/xk6-otel-gen-schema/main.go` | 53 |
| `cmd/xk6-otel-gen-schema/main_test.go` | 137 |
| `test/examples/examples_test.go` | 50 |
| `examples/minimal/topology.yaml` | 77 |
| `examples/minimal/script.js` | 25 |
| `examples/minimal/otel-collector-config.yaml` | 37 |
| `examples/minimal/README.md` | 92 |
| `examples/minimal/k8s/*` | 682 |
| `examples/astroshop/topology.yaml` | 359 |
| `examples/astroshop/script.js` | 41 |
| `examples/astroshop/otel-collector-config.yaml` | 37 |
| `examples/astroshop/README.md` | 95 |
| `examples/astroshop/k8s/*` | 728 |
| `README.md` | 302 |
| `LICENSE` | 201 |
| `.github/dependabot.yml` | updated |
| `.lychee.toml` | 17 |
| `.goheader.txt` | 1 |
| `.golangci.yml` | 10 |

Total U8 deliverable lines under `cmd/`, `examples/`, `test/examples/`, and `.claude/skills/sync-astroshop/`: 2480.

## Verification Results

| Check | Result |
|---|---|
| `go build ./...` | pass |
| `go vet ./cmd/... ./test/examples/...` | pass |
| `go test -race -count=1 ./...` | pass |
| `go test -cover ./cmd/...` | pass, 86.4% statements |
| `golangci-lint run ./cmd/...` | pass |
| `golangci-lint run` | pass |
| `go test ./test/examples/...` | pass |
| `kustomize build examples/minimal/k8s/ > /tmp/minimal.yaml` | pass |
| `kubectl apply --dry-run=client -f /tmp/minimal.yaml` | pass |
| `kustomize build examples/astroshop/k8s/ > /tmp/astroshop.yaml` | pass |
| `kubectl apply --dry-run=client -f /tmp/astroshop.yaml` | pass |
| `xk6 build --with .` | skipped; `xk6` not installed locally |
| `lychee --config .lychee.toml README.md 'examples/**/README.md'` | skipped; `lychee` not installed locally |
| Source `TODO(agent):` markers | none |

## Deviations From Plan

- `topology.ExportJSONSchema()` exists in the current U1 code as `(*topology.Schema).ExportJSONSchema()`. The U8 CLI uses that existing method to keep U8 additive and avoid a U1 coordination patch.
- The kustomize built into `kubectl` rejects `../otel-collector-config.yaml` because of load restrictions. Each example keeps the root `otel-collector-config.yaml` for users and includes a same-content `k8s/otel-collector-config.yaml` as the local ConfigMap source.
- Image tags were refreshed at implementation time: Collector contrib `0.154.0`, Tempo `3.0.0`, Loki `3.7.2`, Prometheus `v3.12.0`, Grafana `13.0.2`, and OTel Demo snapshot `2.2.0`.

## Recent Commits

| Commit | Message |
|---|---|
| `585a90b` | `chore(license): backfill SPDX headers across all packages` |
| `d258602` | `build(ci): add dependabot, lychee, goheader configs` |
| `c92c68e` | `docs(skill): add sync-astroshop AI maintenance skill` |
| `3e66589` | `docs(project): add full project README and Apache-2.0 LICENSE` |
| `75b5de7` | `feat(examples): add astroshop 18-service example modeled after OTel Demo` |
| `3b66e3c` | `feat(examples): add minimal 3-tier example with LGTM-lite k8s stack` |
| `f180f87` | `test(examples): add topology validation test for example yamls` |
| `84da832` | `feat(cmd): add xk6-otel-gen-schema CLI for JSON Schema export` |
| `a377008` | `build(samples): scaffold U8 directories` |

No benchmark command was required for U8; this unit has no benchmark target.
