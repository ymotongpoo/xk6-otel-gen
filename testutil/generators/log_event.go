// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"fmt"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

// ValidLogEventName returns a non-empty log event name within topology limits.
func ValidLogEventName() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-z][a-z0-9_.]{0,39}`)
}

// ValidLogCondition returns a supported declarative log condition.
func ValidLogCondition() *rapid.Generator[topology.LogCondition] {
	return rapid.SampledFrom([]topology.LogCondition{
		topology.ConditionAlways,
		topology.ConditionOnSuccess,
		topology.ConditionOnError,
	})
}

// ValidLogSeverity returns a supported declarative log severity.
func ValidLogSeverity() *rapid.Generator[topology.LogSeverity] {
	return rapid.SampledFrom([]topology.LogSeverity{
		topology.SeverityTrace,
		topology.SeverityDebug,
		topology.SeverityInfo,
		topology.SeverityWarn,
		topology.SeverityError,
		topology.SeverityFatal,
	})
}

// ValidLogEventSpec returns a topology-valid declarative log event.
func ValidLogEventSpec() *rapid.Generator[topology.LogEventSpec] {
	return rapid.Custom(func(t *rapid.T) topology.LogEventSpec {
		ev := topology.LogEventSpec{
			Name:      ValidLogEventName().Draw(t, "name"),
			Severity:  ValidLogSeverity().Draw(t, "severity"),
			Condition: ValidLogCondition().Draw(t, "condition"),
		}
		if rapid.Bool().Draw(t, "has_body") {
			ev.Body = rapid.StringN(1, 40, 80).Draw(t, "body")
		}
		if rapid.Bool().Draw(t, "has_attributes") {
			count := rapid.IntRange(1, 3).Draw(t, "attr_count")
			ev.Attributes = make(map[string]any, count)
			for i := 0; i < count; i++ {
				key := fmt.Sprintf("attr_%d", i)
				ev.Attributes[key] = rapid.StringN(1, 8, 16).Draw(t, fmt.Sprintf("attr_%d", i))
			}
		}
		return ev
	})
}
