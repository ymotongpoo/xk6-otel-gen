// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"pgregory.net/rapid"
)

func TestParse_MapKeyConsistency(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := generators.ValidSchema().Draw(t, "schema")
		for id, svc := range s.Services {
			if svc.Name != id {
				t.Fatalf("service key %s mismatches name %s", id, svc.Name)
			}
			for name, op := range svc.Operations {
				if op.Service != svc {
					t.Fatalf("operation %s.%s has wrong service back-pointer", id, name)
				}
				if op.Service.Operations[op.Name] != op {
					t.Fatalf("operation %s.%s is not reachable by its own name", id, name)
				}
			}
		}
	})
}
