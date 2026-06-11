// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

// EdgeOption mutates edge-generation parameters.
type EdgeOption interface {
	applyEdgeOption(*edgeOptions)
}

type edgeOptionFunc func(*edgeOptions)

func (f edgeOptionFunc) applyEdgeOption(o *edgeOptions) {
	f(o)
}

type protocolOption struct {
	edge   *topology.Protocol
	config *exporter.Protocol
}

func (o protocolOption) applyEdgeOption(edgeOpts *edgeOptions) {
	if o.edge != nil {
		edgeOpts.protocol = o.edge
	}
}

type edgeOptions struct {
	protocol        *topology.Protocol
	p50             *time.Duration
	p95             *time.Duration
	errorRate       *float64
	onFailure       *topology.RecoveryPolicy
	withoutRecovery bool
}

func applyEdgeOptions(opts []EdgeOption) edgeOptions {
	o := edgeOptions{}
	for _, opt := range opts {
		opt.applyEdgeOption(&o)
	}
	return o
}

type protocolValue interface {
	topology.Protocol | exporter.Protocol
}

// WithProtocol fixes the generated protocol for edge or exporter config generators.
func WithProtocol[P protocolValue](p P) protocolOption {
	switch value := any(p).(type) {
	case topology.Protocol:
		return protocolOption{edge: &value}
	case exporter.Protocol:
		return protocolOption{config: &value}
	default:
		return protocolOption{}
	}
}

// WithLatency fixes the generated edge latency pair.
func WithLatency(p50, p95 time.Duration) EdgeOption {
	return edgeOptionFunc(func(o *edgeOptions) {
		o.p50 = &p50
		o.p95 = &p95
	})
}

// WithErrorRate fixes the generated edge error rate.
func WithErrorRate(r float64) EdgeOption {
	return edgeOptionFunc(func(o *edgeOptions) {
		o.errorRate = &r
	})
}

// WithOnFailure fixes the generated edge recovery policy.
func WithOnFailure(rp *topology.RecoveryPolicy) EdgeOption {
	return edgeOptionFunc(func(o *edgeOptions) {
		o.onFailure = rp
	})
}

// WithoutRecovery prevents ValidEdge from synthesizing a nested recovery policy.
func WithoutRecovery() EdgeOption {
	return edgeOptionFunc(func(o *edgeOptions) {
		o.withoutRecovery = true
	})
}

// ValidEdge returns a directed edge between from and to with valid domain ranges.
func ValidEdge(from, to *topology.Operation, opts ...EdgeOption) *rapid.Generator[*topology.Edge] {
	o := applyEdgeOptions(opts)
	return rapid.Custom(func(t *rapid.T) *topology.Edge {
		latency := ValidLatencyPair().Draw(t, "latency")
		p50 := latency.P50
		p95 := latency.P95
		if o.p50 != nil {
			p50 = *o.p50
		}
		if o.p95 != nil {
			p95 = *o.p95
		}
		if p95 < p50 {
			p95 = p50
		}

		protocol := ValidProtocol().Draw(t, "protocol")
		if o.protocol != nil {
			protocol = *o.protocol
		}
		errorRate := ValidErrorRate().Draw(t, "error_rate")
		if o.errorRate != nil {
			errorRate = *o.errorRate
		}

		edge := &topology.Edge{
			From:         from,
			To:           to,
			Protocol:     protocol,
			Latency:      topology.LatencyDist{Distribution: validLatencyDistribution(t, "distribution"), P50: p50, P95: p95},
			ErrorRate:    errorRate,
			Timeout:      ValidTimeoutDuration().Draw(t, "timeout"),
			Retries:      rapid.IntRange(0, 10).Draw(t, "retries"),
			RetryBackoff: validBackoffPolicy(t, "retry_backoff"),
		}
		if o.onFailure != nil {
			edge.OnFailure = o.onFailure
		} else if !o.withoutRecovery && rapid.Float64Range(0, 1).Draw(t, "recovery_roll") < 0.3 && from != nil && to != nil {
			fallbackTargets := []*topology.Operation{to}
			edge.OnFailure = ValidRecoveryPolicy(from, fallbackTargets).Draw(t, "on_failure")
		}
		return edge
	})
}

// AnyEdge returns an edge that may violate domain or pointer invariants.
func AnyEdge(from, to *topology.Operation, opts ...EdgeOption) *rapid.Generator[*topology.Edge] {
	return rapid.Custom(func(t *rapid.T) *topology.Edge {
		edge := ValidEdge(from, to, opts...).Draw(t, "valid_edge")
		switch rapid.IntRange(0, 5).Draw(t, "edge_mutation") {
		case 0:
			edge.From = nil
		case 1:
			edge.To = nil
		case 2:
			edge.ErrorRate = AnyErrorRate().Draw(t, "any_error_rate")
		case 3:
			pair := AnyLatencyPair().Draw(t, "any_latency")
			edge.Latency.P50 = pair.P50
			edge.Latency.P95 = pair.P95
		case 4:
			edge.Timeout = AnyTimeoutDuration().Draw(t, "any_timeout")
		case 5:
			if edge.OnFailure != nil && len(edge.OnFailure.Fallback) > 0 {
				edge.OnFailure.Fallback[0].From = &topology.Operation{Name: "stale-owner", Service: &topology.Service{Name: "stale"}}
			}
		}
		return edge
	})
}

func validLatencyDistribution(t *rapid.T, label string) string {
	return rapid.SampledFrom([]string{"constant", "lognormal", "normal", "exponential"}).Draw(t, label)
}
