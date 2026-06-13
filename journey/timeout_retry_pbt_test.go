// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func TestExecute_EdgeTimeoutInvariant_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		latency := time.Duration(rapid.IntRange(0, 500).Draw(rt, "latency_ms")) * time.Millisecond
		timeout := time.Duration(rapid.IntRange(1, 500).Draw(rt, "timeout_ms")) * time.Millisecond
		schema, edge := timeoutRetrySchema(latency, timeout)
		engine, plan, mock := newExecutablePlanForSchemaRapid(rt, schema)

		if err := engine.Execute(context.Background(), plan); err != nil {
			rt.Fatalf("Execute() error = %v", err)
		}
		span := requireSpanForEdgeRapid(rt, mock.snapshotSpans(), edge)
		duration := span.Outcome.EndTime.Sub(span.Input.StartTime)
		if duration > timeout {
			rt.Fatalf("duration = %s exceeds timeout %s", duration, timeout)
		}
		wantFailure := latency > timeout
		if gotFailure := !span.Outcome.Success; gotFailure != wantFailure {
			rt.Fatalf("failure = %v, want %v for latency=%s timeout=%s", gotFailure, wantFailure, latency, timeout)
		}
	})
}

func TestRetryBackoffArithmetic_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		base := time.Duration(rapid.IntRange(0, 500).Draw(rt, "base_ms")) * time.Millisecond
		retries := rapid.IntRange(1, 8).Draw(rt, "retries")
		policy := rapid.SampledFrom([]topology.BackoffPolicy{
			topology.BackoffConstant,
			topology.BackoffLinear,
			topology.BackoffExponential,
		}).Draw(rt, "policy")
		edge := &topology.Edge{RetryBackoff: policy, RetryBaseDelay: base}
		effectiveBase := base
		if effectiveBase == 0 {
			effectiveBase = topology.DefaultRetryBaseDelay
		}

		var prev time.Duration
		var total time.Duration
		for attempt := 1; attempt <= retries; attempt++ {
			delay := retryBackoffDelay(edge, attempt)
			total += delay
			if attempt > 1 && (policy == topology.BackoffLinear || policy == topology.BackoffExponential) && delay < prev {
				rt.Fatalf("delay[%d] = %s decreased below %s", attempt, delay, prev)
			}
			prev = delay
		}

		var want time.Duration
		for attempt := 1; attempt <= retries; attempt++ {
			switch policy {
			case topology.BackoffConstant:
				want += effectiveBase
			case topology.BackoffLinear:
				want += time.Duration(attempt) * effectiveBase
			default:
				want += time.Duration(1<<(attempt-1)) * effectiveBase
			}
		}
		if total != want {
			rt.Fatalf("total backoff = %s, want %s", total, want)
		}
	})
}

func newExecutablePlanForSchemaRapid(t *rapid.T, schema *topology.Schema) (*Engine, *Plan, *mockSynth) {
	t.Helper()

	mock := newMockSynth()
	engine := NewEngineWithSeed(schema, schema.ApplyFaults(), mock, 1)
	plan, err := engine.BuildPlan("flow")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	return engine, plan, mock
}

func requireSpanForEdgeRapid(t *rapid.T, spans []spanCall, edge *topology.Edge) spanCall {
	t.Helper()

	matches := spansForEdge(spans, edge)
	if len(matches) == 0 {
		t.Fatalf("no span found for edge %p in %+v", edge, spans)
	}
	return matches[0]
}
