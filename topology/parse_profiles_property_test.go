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

func TestParseRoundTrip_Profile_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		schema := generators.ValidSchema(
			generators.MaxServices(4),
			generators.MaxOpsPerService(2),
			generators.MaxCallsPerOp(1),
			generators.MaxFaults(0),
		).Draw(rt, "schema")
		for _, svc := range schema.Services {
			for _, op := range svc.Operations {
				if op.Profile != nil && op.Profile.Enabled {
					if len(op.Profile.Baseline) == 0 {
						rt.Fatal("generated enabled profile missing baseline")
					}
					if op.Profile.SampleRate <= 0 {
						rt.Fatalf("SampleRate = %d", op.Profile.SampleRate)
					}
				}
			}
		}
		data, err := yaml.Marshal(schema)
		if err != nil {
			rt.Fatalf("MarshalYAML() error = %v", err)
		}
		got, err := topology.Parse(bytes.NewReader(data))
		if err != nil {
			rt.Fatalf("Parse() error = %v", err)
		}
		if !topology.Equal(schema, got) {
			rt.Fatal("roundtrip not equal for profile-bearing schema")
		}
	})
}
