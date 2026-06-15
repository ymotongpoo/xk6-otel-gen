// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"fmt"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

// ValidStackSample returns one weighted stack trace sample.
func ValidStackSample() *rapid.Generator[topology.StackSample] {
	return rapid.Custom(func(t *rapid.T) topology.StackSample {
		frameCount := rapid.IntRange(1, 5).Draw(t, "frame_count")
		frames := make([]string, frameCount)
		for i := range frames {
			frames[i] = rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9_]{0,15}`).Draw(t, fmt.Sprintf("frame_%d", i))
		}
		return topology.StackSample{
			Frames: frames,
			Weight: rapid.Float64Range(0, 1000).Draw(t, "weight"),
		}
	})
}

// ValidProfileSpec returns a topology-valid enabled profile declaration.
func ValidProfileSpec() *rapid.Generator[*topology.ProfileSpec] {
	return rapid.Custom(func(t *rapid.T) *topology.ProfileSpec {
		baselineCount := rapid.IntRange(1, 3).Draw(t, "baseline_count")
		spec := &topology.ProfileSpec{
			Enabled:    true,
			SampleRate: rapid.IntRange(1, 200).Draw(t, "sample_rate"),
			Baseline:   make([]topology.StackSample, 0, baselineCount),
		}
		for i := 0; i < baselineCount; i++ {
			spec.Baseline = append(spec.Baseline, ValidStackSample().Draw(t, fmt.Sprintf("baseline_%d", i)))
		}
		if rapid.Bool().Draw(t, "has_when_fault") {
			incidentCount := rapid.IntRange(1, 3).Draw(t, "incident_count")
			spec.Incident = make([]topology.StackSample, 0, incidentCount)
			for i := 0; i < incidentCount; i++ {
				spec.Incident = append(spec.Incident, ValidStackSample().Draw(t, fmt.Sprintf("incident_%d", i)))
			}
			spec.WhenFault = &topology.ProfileFaultLink{
				Kind: ValidMetricFaultKind().Draw(t, "fault_kind"),
			}
		}
		return spec
	})
}
