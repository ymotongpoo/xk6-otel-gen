package topology_test

import (
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestParse_ParallelStepsRecoveryAndFaultTargets(t *testing.T) {
	t.Parallel()

	const src = `
services:
  frontend:
    kind: application
    operations:
      - name: Root
        calls:
          - parallel:
              - to: { service: backend, operation: Fetch }
                protocol: http
                on_failure:
                  fallback:
                    - to: { service: cache, operation: Fetch }
                      protocol: http
                  on_exhausted: return_default
                  default_response: { stale: true }
              - to: { service: audit, operation: Append }
                protocol: messaging
  backend:
    kind: application
    operations:
      - name: Fetch
  cache:
    kind: cache
    operations:
      - name: Fetch
  audit:
    kind: queue
    operations:
      - name: Append
journeys:
  fanout:
    steps:
      - parallel:
          - service: frontend
            operation: Root
          - service: audit
            operation: Append
faults:
  - target: node:cache
    kind: crash
    severity: { probability: 0.1 }
  - target: operation:audit.Append
    kind: disconnect
    severity: { probability: 0.2 }
  - target: edge:frontend.Root->backend.Fetch
    kind: latency_inflation
    severity: { probability: 0.3, multiplier: 2 }
`
	s := mustParse(t, src)
	if got := len(s.Journeys["fanout"].Steps[0].Parallel); got != 2 {
		t.Fatalf("parallel steps = %d, want 2", got)
	}
	root := s.Services["frontend"].Operations["Root"]
	if got := len(root.Calls[0].Parallel); got != 2 {
		t.Fatalf("parallel calls = %d, want 2", got)
	}
	edge := root.Calls[0].Parallel[0].Edge
	if edge.OnFailure == nil || edge.OnFailure.OnExhausted != topology.ExhaustedReturnDefault {
		t.Fatalf("recovery policy not resolved: %+v", edge.OnFailure)
	}
	if len(s.Faults) != 3 {
		t.Fatalf("fault count = %d, want 3", len(s.Faults))
	}
	if s.Faults[0].Target.Service == nil || s.Faults[1].Target.Operation == nil || s.Faults[2].Target.Edge == nil {
		t.Fatalf("fault targets not fully resolved: %+v", s.Faults)
	}
}
