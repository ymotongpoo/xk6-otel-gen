// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology

import "time"

type rawSchema struct {
	Namespace string                 `yaml:"namespace,omitempty"`
	Services  map[string]*rawService `yaml:"services"`
	Journeys  map[string]*rawJourney `yaml:"journeys"`
	Faults    []*rawFault            `yaml:"faults,omitempty"`
}

type rawService struct {
	Namespace  string          `yaml:"namespace,omitempty"`
	Kind       string          `yaml:"kind"`
	Replicas   *int            `yaml:"replicas,omitempty"`
	Language   string          `yaml:"language,omitempty"`
	Framework  string          `yaml:"framework,omitempty"`
	Version    string          `yaml:"version,omitempty"`
	Metrics    []*rawMetric    `yaml:"metrics,omitempty"`
	Operations []*rawOperation `yaml:"operations"`
}

type rawOperation struct {
	Name         string            `yaml:"name"`
	Calls        []*rawCallNode    `yaml:"calls,omitempty"`
	LogEvents    []*rawLogEvent    `yaml:"log_events,omitempty"`
	Metrics      []*rawMetric      `yaml:"metrics,omitempty"`
	StateUpdates []*rawStateUpdate `yaml:"state_updates,omitempty"`
	Profile      *rawProfile       `yaml:"profile,omitempty"`
}

type rawProfile struct {
	Enabled    bool             `yaml:"enabled"`
	SampleRate *int             `yaml:"sample_rate,omitempty"`
	Baseline   []*rawStack      `yaml:"baseline"`
	Incident   []*rawStack      `yaml:"incident,omitempty"`
	WhenFault  *rawProfileFault `yaml:"when_fault,omitempty"`
}

type rawStack struct {
	Frames []string `yaml:"frames"`
	Weight float64  `yaml:"weight"`
}

type rawProfileFault struct {
	Kind string `yaml:"kind"`
}

type rawMetric struct {
	Name       string              `yaml:"name"`
	Type       string              `yaml:"type"`
	Unit       string              `yaml:"unit,omitempty"`
	Baseline   *float64            `yaml:"baseline,omitempty"`
	Condition  string              `yaml:"condition,omitempty"`
	Attributes map[string]any      `yaml:"attributes,omitempty"`
	WhenFault  *rawMetricFaultLink `yaml:"when_fault,omitempty"`
	Source     *rawMetricSource    `yaml:"source,omitempty"`
}

type rawMetricFaultLink struct {
	Kind  string   `yaml:"kind"`
	Delta *float64 `yaml:"delta,omitempty"`
	Value *float64 `yaml:"value,omitempty"`
}

type rawMetricSource struct {
	Accumulator string `yaml:"accumulator"`
	Minus       string `yaml:"minus,omitempty"`
}

type rawStateUpdate struct {
	Key       string              `yaml:"key"`
	Delta     *float64            `yaml:"delta,omitempty"`
	Condition string              `yaml:"condition,omitempty"`
	WhenFault *rawMetricFaultLink `yaml:"when_fault,omitempty"`
}

type rawLogEvent struct {
	Name       string         `yaml:"name"`
	Severity   string         `yaml:"severity,omitempty"`
	Condition  string         `yaml:"condition,omitempty"`
	Body       string         `yaml:"body,omitempty"`
	Attributes map[string]any `yaml:"attributes,omitempty"`
}

type rawCallNode struct {
	To             *rawCallTarget     `yaml:"to,omitempty"`
	Parallel       []*rawCallNode     `yaml:"parallel,omitempty"`
	Protocol       string             `yaml:"protocol,omitempty"`
	Latency        *rawLatencyDist    `yaml:"latency,omitempty"`
	ErrorRate      *float64           `yaml:"error_rate,omitempty"`
	Timeout        *time.Duration     `yaml:"timeout,omitempty"`
	Retries        *int               `yaml:"retries,omitempty"`
	RetryBackoff   string             `yaml:"retry_backoff,omitempty"`
	RetryBaseDelay *time.Duration     `yaml:"retry_base_delay,omitempty"`
	OnFailure      *rawRecoveryPolicy `yaml:"on_failure,omitempty"`
}

type rawCallTarget struct {
	Service   string `yaml:"service"`
	Operation string `yaml:"operation"`
}

type rawJourney struct {
	Steps  []*rawStep `yaml:"steps"`
	Weight *float64   `yaml:"weight,omitempty"`
}

type rawStep struct {
	Service   string     `yaml:"service,omitempty"`
	Operation string     `yaml:"operation,omitempty"`
	Parallel  []*rawStep `yaml:"parallel,omitempty"`
}

type rawFault struct {
	Target   string       `yaml:"target"`
	Kind     string       `yaml:"kind"`
	Severity *rawSeverity `yaml:"severity,omitempty"`
}

type rawRecoveryPolicy struct {
	Fallback        []*rawCallNode `yaml:"fallback,omitempty"`
	OnExhausted     string         `yaml:"on_exhausted,omitempty"`
	DefaultResponse map[string]any `yaml:"default_response,omitempty"`
}

type rawLatencyDist struct {
	Distribution string         `yaml:"distribution,omitempty"`
	P50          *time.Duration `yaml:"p50,omitempty"`
	P95          *time.Duration `yaml:"p95,omitempty"`
}

type rawSeverity struct {
	Probability *float64       `yaml:"probability,omitempty"`
	Multiplier  *float64       `yaml:"multiplier,omitempty"`
	Add         *time.Duration `yaml:"add,omitempty"`
	Value       *float64       `yaml:"value,omitempty"`
}
