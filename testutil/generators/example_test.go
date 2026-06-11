// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func ExampleValidSchema() {
	rapid.Check(&testing.T{}, func(t *rapid.T) {
		_ = ValidSchema(
			MaxServices(3),
			MaxOpsPerService(2),
		).Draw(t, "schema")
	})
}

func ExampleValidService() {
	rapid.Check(&testing.T{}, func(t *rapid.T) {
		_ = ValidService(WithKind(topology.KindDatabase)).Draw(t, "service")
	})
}

func ExampleAnySchema() {
	rapid.Check(&testing.T{}, func(t *rapid.T) {
		_ = AnySchema(BiasValid(0.0)).Draw(t, "schema")
	})
}
