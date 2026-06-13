// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"bytes"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"gopkg.in/yaml.v3"
	"pgregory.net/rapid"
)

func TestParse_RoundTrip(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := generators.ValidSchema().Draw(t, "schema")
		yamlBytes, err := yaml.Marshal(s)
		if err != nil {
			t.Fatalf("yaml.Marshal() error = %v", err)
		}
		s2, err := topology.Parse(bytes.NewReader(yamlBytes))
		if err != nil {
			t.Fatalf("Parse() error = %v\nYAML:\n%s", err, yamlBytes)
		}
		if !topology.Equal(s, s2) {
			t.Fatalf("round-trip lost or altered fields\nYAML:\n%s", yamlBytes)
		}
	})
}
