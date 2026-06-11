// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func BenchmarkParse(b *testing.B) {
	yamlBytes, err := os.ReadFile("testdata/typical.yaml")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := topology.Parse(bytes.NewReader(yamlBytes)); err != nil {
			b.Fatal(err)
		}
	}
}
