// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"fmt"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

// ValidMetricName returns a non-empty metric name within topology limits.
func ValidMetricName() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-z][a-z0-9_.]{0,39}`)
}

// ValidMetricType returns a supported declarative metric type.
func ValidMetricType() *rapid.Generator[topology.MetricType] {
	return rapid.SampledFrom([]topology.MetricType{
		topology.MetricCounter,
		topology.MetricGauge,
		topology.MetricHistogram,
	})
}

// ValidMetricFaultKind returns a supported fault kind for metric linkage.
func ValidMetricFaultKind() *rapid.Generator[topology.FaultKind] {
	return rapid.SampledFrom([]topology.FaultKind{
		topology.FaultLatencyInflation,
		topology.FaultErrorRateOverride,
		topology.FaultDisconnect,
		topology.FaultCrash,
	})
}

// ValidMetricSpec returns a topology-valid declarative metric.
func ValidMetricSpec() *rapid.Generator[topology.MetricSpec] {
	return rapid.Custom(func(t *rapid.T) topology.MetricSpec {
		spec := topology.MetricSpec{
			Name:      ValidMetricName().Draw(t, "name"),
			Type:      ValidMetricType().Draw(t, "type"),
			Baseline:  rapid.Float64Range(-1000, 1000).Draw(t, "baseline"),
			Condition: ValidLogCondition().Draw(t, "condition"),
		}
		if rapid.Bool().Draw(t, "has_unit") {
			spec.Unit = rapid.SampledFrom([]string{"{request}", "{usd}", "s", "1"}).Draw(t, "unit")
		}
		if rapid.Bool().Draw(t, "has_attributes") {
			count := rapid.IntRange(1, 3).Draw(t, "attr_count")
			spec.Attributes = make(map[string]any, count)
			for i := 0; i < count; i++ {
				key := fmt.Sprintf("attr_%d", i)
				spec.Attributes[key] = rapid.StringN(1, 8, 16).Draw(t, fmt.Sprintf("attr_%d", i))
			}
		}
		if rapid.Bool().Draw(t, "has_when_fault") {
			link := &topology.MetricFaultLink{
				Kind: ValidMetricFaultKind().Draw(t, "fault_kind"),
			}
			if rapid.Bool().Draw(t, "use_value_override") {
				link.HasValue = true
				link.Value = rapid.Float64Range(-1000, 1000).Draw(t, "fault_value")
			} else {
				link.Delta = rapid.Float64Range(-1000, 1000).Draw(t, "fault_delta")
			}
			spec.WhenFault = link
		}
		return spec
	})
}
