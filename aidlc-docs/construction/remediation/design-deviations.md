# Round 2 Approved Design Deviations

Date: 2026-06-11

Decision-maker: user

This file records approved requirement/design deviations identified during the
Round 2 requirements-gap review. Original FD/NFR documents are left unchanged.

## 1. Single-span-per-hop model

Requirement/design reference:

- FR-6.1 / FR-5.1 telemetry model implications
- `aidlc-docs/construction/u3-synth/functional-design/business-logic-model.md:156`
- `aidlc-docs/construction/u3-synth/functional-design/business-logic-model.md:157`

Implementation evidence:

- `journey/plan.go:127` builds journey nodes from `Edge.To` only.
- `synth/synthesizer.go:272` through `synth/synthesizer.go:291` infers SERVER
  direction for application-to-application hops.

Decision:

Keep the current single-span-per-hop model instead of adding caller-side CLIENT
spans for every application-to-application edge.

Consequences:

- Span count remains lower and journey execution stays cheaper.
- OTel Collector service-graph connectors that require paired client/server
  spans may not derive application-to-application edges from those spans.
- Parent-child trace derivation still represents the topology.
- Future option: add caller-side client spans behind an explicit opt-in.

## 2. No probabilistic call firing

Requirement/design reference:

- FR-5.1 mentions conditional dependency firing in the requirements examples.

Implementation evidence:

- Weighted multi-journey selection is implemented via `journey.Engine.PickJourney`
  and the JS `handle.runRandomJourney()` API.
- Topology call edges do not define a YAML `probability` field.

Decision:

Treat conditional dependency firing as satisfied by weighted multi-journey
selection for this release. Per-call probability remains a future extension, not
a current implementation gap.

Consequences:

- The YAML schema remains unchanged in Round 2.
- Users model optional paths by defining multiple journeys with different
  weights.

## 3. k6 compatibility CI matrix deferred

Requirement/design reference:

- NFR-3.1 k6 version compatibility.
- Build and Test stage notes defer CI workflow artifacts and compatibility
  matrix generation to post-stage work.

Implementation evidence:

- Current verification uses local `go build`, `go test`, race tests, linting,
  schema generation, and optional `xk6 build` when available.

Decision:

Defer the k6 version compatibility CI matrix together with the other CI workflow
artifacts left as post-stage work.

Consequences:

- No CI workflow YAML is introduced in Round 2.
- Local and package-level checks remain the acceptance mechanism for this
  remediation pass.
