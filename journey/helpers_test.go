// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

type mockSynth struct {
	mu            sync.Mutex
	spans         []spanCall
	metrics       []metricCall
	customMetrics []customMetricCall
	logs          []logCall
	profiles      []profileCall
}

type spanCall struct {
	Input    synth.SpanInput
	Outcome  synth.Outcome
	Finished bool
}

type metricCall struct {
	Input synth.MetricInput
}

type customMetricCall struct {
	Input synth.CustomMetricInput
}

type logCall struct {
	Input synth.LogInput
}

type profileCall struct {
	Input synth.ProfileInput
}

func newMockSynth() *mockSynth {
	return &mockSynth{}
}

func (m *mockSynth) BeginSpan(ctx context.Context, in synth.SpanInput) (context.Context, synth.FinishSpanFunc) {
	m.mu.Lock()
	idx := len(m.spans)
	m.spans = append(m.spans, spanCall{Input: in})
	m.mu.Unlock()

	parentSC := trace.SpanContextFromContext(ctx)
	traceID := parentSC.TraceID()
	if !parentSC.IsValid() {
		traceID = trace.TraceID{1}
	}
	spanID := trace.SpanID{}
	spanID[7] = byte(idx + 1)
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))

	return ctx, func(out synth.Outcome) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.spans[idx].Outcome = out
		m.spans[idx].Finished = true
	}
}

func (m *mockSynth) RecordMetric(_ context.Context, in synth.MetricInput) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = append(m.metrics, metricCall{Input: in})
}

func (m *mockSynth) EmitLog(_ context.Context, in synth.LogInput) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, logCall{Input: in})
}

func (m *mockSynth) RecordCustom(_ context.Context, in synth.CustomMetricInput) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customMetrics = append(m.customMetrics, customMetricCall{Input: in})
}

func (m *mockSynth) EmitProfile(_ context.Context, in synth.ProfileInput) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profiles = append(m.profiles, profileCall{Input: in})
}

func (m *mockSynth) snapshotSpans() []spanCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]spanCall, len(m.spans))
	copy(out, m.spans)
	return out
}

func (m *mockSynth) snapshotMetrics() []metricCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]metricCall, len(m.metrics))
	copy(out, m.metrics)
	return out
}

func (m *mockSynth) snapshotLogs() []logCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]logCall, len(m.logs))
	copy(out, m.logs)
	return out
}

func (m *mockSynth) snapshotCustomMetrics() []customMetricCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]customMetricCall, len(m.customMetrics))
	copy(out, m.customMetrics)
	return out
}

func (m *mockSynth) snapshotProfiles() []profileCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]profileCall, len(m.profiles))
	copy(out, m.profiles)
	return out
}

type schemaOption func(*topology.Schema)

func newTestSchema(t *testing.T, opts ...schemaOption) *topology.Schema {
	t.Helper()
	schema := newPlanTestSchema()
	for _, opt := range opts {
		opt(schema)
	}
	return schema
}

func newTestOverlay(t *testing.T, schema *topology.Schema, faults ...topology.FaultSpec) *topology.FaultOverlay {
	t.Helper()
	schema.Faults = faults
	return schema.ApplyFaults()
}

func assertOutcomeMatches(t *testing.T, got, want Outcome) {
	t.Helper()
	if got.Success != want.Success {
		t.Fatalf("Outcome.Success = %v, want %v", got.Success, want.Success)
	}
	if got.StatusCode != want.StatusCode {
		t.Fatalf("Outcome.StatusCode = %d, want %d", got.StatusCode, want.StatusCode)
	}
	if got.ErrorType != want.ErrorType {
		t.Fatalf("Outcome.ErrorType = %q, want %q", got.ErrorType, want.ErrorType)
	}
	if got.Cascaded != want.Cascaded {
		t.Fatalf("Outcome.Cascaded = %v, want %v", got.Cascaded, want.Cascaded)
	}
}

type panicBeginSynth struct{}

func (panicBeginSynth) BeginSpan(context.Context, synth.SpanInput) (context.Context, synth.FinishSpanFunc) {
	panic("boom")
}

func (panicBeginSynth) RecordMetric(context.Context, synth.MetricInput) {}

func (panicBeginSynth) EmitLog(context.Context, synth.LogInput) {}

func (panicBeginSynth) RecordCustom(context.Context, synth.CustomMetricInput) {}

func (panicBeginSynth) EmitProfile(context.Context, synth.ProfileInput) {}
