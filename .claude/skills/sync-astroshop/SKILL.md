---
name: sync-astroshop
description: Review open-telemetry/opentelemetry-demo updates against examples/astroshop/topology.yaml and produce a checklist-driven proposal covering service graph changes, journey changes, fault demos, version comments, README references, and validation commands.
---

# Sync Astroshop

Use this skill when asked to review or refresh the local astroshop example against the upstream OpenTelemetry Demo repository (`open-telemetry/opentelemetry-demo`).

## When to Use

- Annual or release-driven review of `examples/astroshop/topology.yaml`
- A user asks whether the astroshop example still matches upstream OTel Demo
- A user provides an OTel Demo tag or commit and asks for a local diff proposal

## Out of Scope

- Daily or monthly automatic synchronization
- One-for-one reproduction of OTel Demo Kubernetes, Helm, or source code
- Updating Go module dependencies or container image tags outside the astroshop example
- Introducing real services; this project remains a synthetic telemetry generator

## Steps

1. Survey upstream

   Inspect `open-telemetry/opentelemetry-demo` at the requested tag or commit. Capture services, key dependencies, primary user journeys, and notable new telemetry-relevant behavior.

2. Diff astroshop

   Compare upstream findings with `examples/astroshop/topology.yaml`, `examples/astroshop/README.md`, and `examples/astroshop/script.js`. Focus on service names, dependency nodes, operation graph shape, journey names, and the snapshot version comment.

3. Propose changes

   Produce a concise proposal before editing. Include checklist items for added/removed services, changed journeys, changed fault demonstrations, README snapshot text, and validation commands.

4. Apply checklist

   Apply only the approved checklist items. Keep the local example representative rather than exhaustive. Run:

   ```bash
   go test ./test/examples/...
   kustomize build examples/astroshop/k8s/ > /tmp/astroshop.yaml
   kubectl apply --dry-run=client -f /tmp/astroshop.yaml
   ```

## Anti-Patterns

- Do not block on minor upstream churn that has no topology or journey impact.
- Do not mirror upstream service internals when a single synthetic operation captures the behavior.
- Do not add external credentials, real payment flows, or production endpoints.
- Do not change `examples/minimal/` while syncing astroshop.

## Output

Return a Markdown summary suitable for a PR description:

```markdown
## Astroshop sync summary

- Upstream reference: open-telemetry/opentelemetry-demo <tag-or-commit>
- Local file: examples/astroshop/topology.yaml
- Services changed:
- Journeys changed:
- Fault demos changed:
- Validation:
```
