// SPDX-License-Identifier: Apache-2.0

package generators

import "testing"

func BenchmarkValidSchemaDraw(b *testing.B) {
	gen := ValidSchema()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gen.Example()
	}
}
