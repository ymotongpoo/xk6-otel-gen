// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

// emitMessagingProducerSpan synthesizes the PRODUCER (publish) span on the
// sender side of a messaging edge and returns its SpanContext so the consumer
// span can link back to it. It draws no randomness (InstanceIdx 0, fixed
// publish latency) to keep the seeded RNG stream stable.
func (e *engineImpl) emitMessagingProducerSpan(ctx context.Context, edge *topology.Edge, start time.Time) trace.SpanContext {
	if edge.From == nil || edge.From.Service == nil {
		return trace.SpanContext{}
	}
	pubLatency := messagingPublishLatency(edge)
	pctx, finish := e.synth.BeginSpan(ctx, synth.SpanInput{
		Service:     edge.From.Service,
		Edge:        edge,
		Operation:   edge.From.Name,
		StartTime:   start,
		InstanceIdx: 0,
	})
	// Symmetric to the consumer's messaging.receive.duration: record the
	// publish duration for the producer side.
	e.synth.RecordMetric(pctx, synth.MetricInput{
		Service:     edge.From.Service,
		Edge:        edge,
		Operation:   edge.From.Name,
		Latency:     pubLatency,
		Outcome:     synth.Outcome{Success: true, StatusCode: 200, EndTime: start.Add(pubLatency)},
		InstanceIdx: 0,
	})
	finish(synth.Outcome{Success: true, StatusCode: 200, EndTime: start.Add(pubLatency)})
	return trace.SpanContextFromContext(pctx)
}

// messagingPublishLatency returns a deterministic publish latency (no RNG) so
// that adding producer spans does not perturb the seeded latency stream.
func messagingPublishLatency(edge *topology.Edge) time.Duration {
	if edge != nil && edge.Latency.P50 > 0 {
		return edge.Latency.P50 / 4
	}
	return defaultEntryLatency
}
