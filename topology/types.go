// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology

import "time"

// DefaultNamespace is the synthetic service.namespace value applied when a
// topology does not specify a namespace.
const DefaultNamespace = "xk6-otel-gen"

// DefaultRetryBaseDelay is the synthetic retry delay base used when an edge
// does not specify retry_base_delay.
const DefaultRetryBaseDelay = 100 * time.Millisecond

// ServiceID is a newtype for service name identifiers.
type ServiceID string

// Schema represents the parsed and resolved root of a topology YAML file.
// It is immutable after Parse returns; treat all fields as read-only.
type Schema struct {
	Namespace string                 `yaml:"namespace"`
	Services  map[ServiceID]*Service `yaml:"services"`
	Journeys  map[string]*Journey    `yaml:"journeys"`
	Faults    []FaultSpec            `yaml:"faults"`
}

// Service describes a microservice or dependency node in a topology.
type Service struct {
	Name       ServiceID             `yaml:"name"`
	Namespace  string                `yaml:"namespace"`
	Kind       ServiceKind           `yaml:"kind"`
	Replicas   int                   `yaml:"replicas"`
	Language   string                `yaml:"language"`
	Framework  string                `yaml:"framework"`
	Version    string                `yaml:"version"`
	Operations map[string]*Operation `yaml:"operations"`
}

// Operation is a callable unit at a Service.
type Operation struct {
	Name      string         `yaml:"name"`
	Service   *Service       `yaml:"service"`
	Calls     []*CallNode    `yaml:"calls"`
	LogEvents []LogEventSpec `yaml:"log_events,omitempty"`
	Metrics   []MetricSpec   `yaml:"metrics,omitempty"`
}

// MetricSpec is a declarative synthetic metric emitted when an operation
// completes (gated by Condition). When WhenFault is set, the value becomes
// baseline+Delta (or Value) only while a fault of the given kind is active on
// THIS operation's node. The metric must be declared on the operation that the
// fault targets; activeness is derived from that node's folded fault, so no
// additional randomness is drawn (deterministic).
type MetricSpec struct {
	Name       string           `yaml:"name"`
	Type       MetricType       `yaml:"type"`
	Unit       string           `yaml:"unit,omitempty"`
	Baseline   float64          `yaml:"baseline"`
	Condition  LogCondition     `yaml:"condition"`
	Attributes map[string]any   `yaml:"attributes,omitempty"`
	WhenFault  *MetricFaultLink `yaml:"when_fault,omitempty"`
}

// MetricFaultLink adjusts a metric value when a matching fault kind is active.
type MetricFaultLink struct {
	Kind     FaultKind `yaml:"kind"`
	Delta    float64   `yaml:"delta"`
	Value    float64   `yaml:"value"`
	HasValue bool      `yaml:"has_value"`
}

// LogEventSpec is a declarative structured log event emitted when an operation
// completes, gated by Condition on the operation outcome.
type LogEventSpec struct {
	Name       string         `yaml:"name"`
	Severity   LogSeverity    `yaml:"severity"`
	Condition  LogCondition   `yaml:"condition"`
	Body       string         `yaml:"body,omitempty"`
	Attributes map[string]any `yaml:"attributes,omitempty"`
}

// CallNode is either a single outgoing Edge or a Parallel group of CallNodes.
type CallNode struct {
	Edge     *Edge       `yaml:"edge"`
	Parallel []*CallNode `yaml:"parallel"`
}

// Edge is a directed call from one operation to another.
type Edge struct {
	From           *Operation      `yaml:"from"`
	To             *Operation      `yaml:"to"`
	Protocol       Protocol        `yaml:"protocol"`
	Latency        LatencyDist     `yaml:"latency"`
	ErrorRate      float64         `yaml:"error_rate"`
	Timeout        time.Duration   `yaml:"timeout"`
	Retries        int             `yaml:"retries"`
	RetryBackoff   BackoffPolicy   `yaml:"retry_backoff"`
	RetryBaseDelay time.Duration   `yaml:"retry_base_delay"`
	OnFailure      *RecoveryPolicy `yaml:"on_failure"`
}

// LatencyDist holds latency distribution parameters.
type LatencyDist struct {
	Distribution string        `yaml:"distribution"`
	P50          time.Duration `yaml:"p50"`
	P95          time.Duration `yaml:"p95"`
}

// RecoveryPolicy describes what happens when an edge call fails.
type RecoveryPolicy struct {
	Fallback        []*Edge         `yaml:"fallback"`
	OnExhausted     ExhaustedAction `yaml:"on_exhausted"`
	DefaultResponse map[string]any  `yaml:"default_response"`
}

// Journey describes a weighted sequence of operation invocations.
type Journey struct {
	Name   string  `yaml:"name"`
	Steps  []*Step `yaml:"steps"`
	Weight float64 `yaml:"weight"`
}

// Step is one operation invocation in a journey.
type Step struct {
	Op       *Operation `yaml:"op"`
	Parallel []*Step    `yaml:"parallel"`
}

// FaultTarget identifies what a fault is attached to.
type FaultTarget struct {
	Kind      TargetKind `yaml:"kind"`
	Service   *Service   `yaml:"service"`
	Operation *Operation `yaml:"operation"`
	Edge      *Edge      `yaml:"edge"`
}

// FaultSpec describes one fault injection rule.
type FaultSpec struct {
	Target   FaultTarget    `yaml:"target"`
	Kind     FaultKind      `yaml:"kind"`
	Severity SeverityParams `yaml:"severity"`
}

// SeverityParams holds fault severity fields used by different FaultKind values.
type SeverityParams struct {
	Probability float64       `yaml:"probability"`
	Multiplier  float64       `yaml:"multiplier"`
	Add         time.Duration `yaml:"add"`
	Value       float64       `yaml:"value"`
}

// FaultOverlay is an opaque computed fault lookup.
type FaultOverlay struct {
	nodeFaults      map[*Service][]FaultSpec
	operationFaults map[*Operation][]FaultSpec
	edgeFaults      map[*Edge][]FaultSpec
}
