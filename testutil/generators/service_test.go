package generators

import (
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func TestValidService_BackPointers(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		svc := ValidService().Draw(t, "service")
		for name, op := range svc.Operations {
			if op.Service != svc {
				t.Fatalf("operation %s has Service %p, want %p", name, op.Service, svc)
			}
		}
	})
}

func TestValidService_NamePattern(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		svc := ValidService().Draw(t, "service")
		if !validServiceIDRegexp().MatchString(string(svc.Name)) {
			t.Fatalf("service name %q does not match valid pattern", svc.Name)
		}
	})
}

func TestValidService_OperationsNonEmpty(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		svc := ValidService().Draw(t, "service")
		if len(svc.Operations) == 0 {
			t.Fatal("ValidService produced no operations")
		}
	})
}

func TestValidService_WithKind(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		svc := ValidService(WithKind(topology.KindDatabase)).Draw(t, "service")
		if svc.Kind != topology.KindDatabase {
			t.Fatalf("kind = %v, want %v", svc.Kind, topology.KindDatabase)
		}
	})
}
