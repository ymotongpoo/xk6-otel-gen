// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func TestValidate_Idempotent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := generators.ValidSchema().Draw(t, "schema")
		err1 := topology.Validate(s)
		err2 := topology.Validate(s)
		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("Validate idempotency mismatch: err1=%v err2=%v", err1, err2)
		}
	})
}
