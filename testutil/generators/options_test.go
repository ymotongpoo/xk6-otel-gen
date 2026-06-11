// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"testing"

	"pgregory.net/rapid"
)

func TestValidSchema_MaxServices(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(-10, 20).Draw(t, "n")
		schema := ValidSchema(MaxServices(n)).Draw(t, "schema")
		if len(schema.Services) > max(1, n) {
			t.Fatalf("got %d services, want <= %d", len(schema.Services), max(1, n))
		}
	})
}

func TestValidSchema_MaxOpsPerService(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(-10, 20).Draw(t, "n")
		schema := ValidSchema(MaxOpsPerService(n)).Draw(t, "schema")
		for _, svc := range schema.Services {
			if len(svc.Operations) > max(1, n) {
				t.Fatalf("service %s has %d operations, want <= %d", svc.Name, len(svc.Operations), max(1, n))
			}
		}
	})
}

func TestValidSchema_MaxCallsPerOp(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(-10, 20).Draw(t, "n")
		schema := ValidSchema(MaxCallsPerOp(n)).Draw(t, "schema")
		for _, op := range allOperations(schema) {
			if calls := countCallEdges(op.Calls); calls > max(0, n) {
				t.Fatalf("operation %s has %d calls, want <= %d", op.Name, calls, max(0, n))
			}
		}
	})
}

func TestMaxServices_ClampExample(t *testing.T) {
	t.Parallel()
	schema := ValidSchema(MaxServices(-5)).Example(1)
	if len(schema.Services) < 1 {
		t.Fatalf("got %d services, want at least 1", len(schema.Services))
	}
}

func TestBiasValid_ClampExample(t *testing.T) {
	t.Parallel()
	for seed := 0; seed < 20; seed++ {
		schema := AnySchema(BiasValid(2.0)).Example(seed)
		if problems := schemaProblems(schema); len(problems) > 0 {
			t.Fatalf("BiasValid(2.0) produced invalid schema: %v", problems)
		}
	}
}
