package topology_test

import (
	"bytes"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"gopkg.in/yaml.v3"
	"pgregory.net/rapid"
)

func TestParse_NoNilPointers(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := generators.ValidSchema().Draw(t, "schema")
		yamlBytes, err := yaml.Marshal(s)
		if err != nil {
			t.Fatalf("yaml.Marshal() error = %v", err)
		}
		parsed, err := topology.Parse(bytes.NewReader(yamlBytes))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		assertNoNilSchemaPointers(t, parsed)
	})
}

func assertNoNilSchemaPointers(t *rapid.T, s *topology.Schema) {
	for id, svc := range s.Services {
		if svc == nil {
			t.Fatalf("service %s is nil", id)
			continue
		}
		for name, op := range svc.Operations {
			if op == nil {
				t.Fatalf("operation %s.%s is nil", id, name)
				continue
			}
			if op.Service == nil {
				t.Fatalf("operation %s.%s has nil Service", id, name)
			}
			assertNoNilCallPointers(t, op.Calls)
		}
	}
	for name, journey := range s.Journeys {
		if journey == nil {
			t.Fatalf("journey %s is nil", name)
			continue
		}
		assertNoNilStepPointers(t, journey.Steps)
	}
	for i, fault := range s.Faults {
		switch fault.Target.Kind {
		case topology.TargetNode:
			if fault.Target.Service == nil {
				t.Fatalf("fault %d has nil service target", i)
			}
		case topology.TargetOperation:
			if fault.Target.Operation == nil {
				t.Fatalf("fault %d has nil operation target", i)
			}
		case topology.TargetEdge:
			if fault.Target.Edge == nil {
				t.Fatalf("fault %d has nil edge target", i)
			}
		}
	}
}

func assertNoNilCallPointers(t *rapid.T, nodes []*topology.CallNode) {
	for _, node := range nodes {
		if node == nil {
			t.Fatal("call node is nil")
			continue
		}
		if node.Edge != nil {
			if node.Edge.From == nil || node.Edge.To == nil {
				t.Fatalf("edge has nil endpoint: %+v", node.Edge)
			}
			if node.Edge.OnFailure != nil {
				for _, fallback := range node.Edge.OnFailure.Fallback {
					if fallback == nil || fallback.From == nil || fallback.To == nil {
						t.Fatalf("fallback edge has nil pointer: %+v", fallback)
					}
				}
			}
		}
		assertNoNilCallPointers(t, node.Parallel)
	}
}

func assertNoNilStepPointers(t *rapid.T, steps []*topology.Step) {
	for _, step := range steps {
		if step == nil {
			t.Fatal("step is nil")
			continue
		}
		if step.Op == nil && len(step.Parallel) == 0 {
			t.Fatal("step has neither Op nor Parallel")
		}
		assertNoNilStepPointers(t, step.Parallel)
	}
}
