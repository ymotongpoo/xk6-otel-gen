// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"context"
	"testing"
	"time"

	otellog "go.opentelemetry.io/otel/log"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

// TestEmitLog_CarriesActiveSpanContext asserts that a log emitted inside an
// active span carries the SAME trace_id/span_id as the span. This is the basis
// of Grafana's trace->logs correlation.
func TestEmitLog_CarriesActiveSpanContext(t *testing.T) {
	tp, mp, lp, spanExporter, _, recorder := newTestProviders(t)
	syn := NewDefault(singleProviderFactory{tp: tp, lp: lp}, mp, nil)

	svc := makeSpanService("frontend", topology.KindApplication)
	start := time.Now()

	// Mirror journey.finishAndEmitAt ordering: BeginSpan, finish (End), then
	// EmitLog with the span context.
	ctx, finish := syn.BeginSpan(context.Background(), SpanInput{
		Service:     svc,
		Operation:   "GET /",
		StartTime:   start,
		InstanceIdx: 0,
	})
	finish(Outcome{Success: true, StatusCode: 200, EndTime: start.Add(100 * time.Millisecond)})
	syn.EmitLog(ctx, LogInput{
		Service:   svc,
		Severity:  otellog.SeverityInfo,
		Body:      "GET / success",
		Timestamp: start.Add(100 * time.Millisecond),
	})

	spans := spanExporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	span := spans[0]
	rec := requireSingleLog(t, recorder)

	if !rec.TraceID().IsValid() {
		t.Fatalf("log has no trace_id (%s); Grafana cannot correlate trace->logs", rec.TraceID())
	}
	if rec.TraceID() != span.SpanContext.TraceID() {
		t.Errorf("log trace_id %s != span trace_id %s", rec.TraceID(), span.SpanContext.TraceID())
	}
	if rec.SpanID() != span.SpanContext.SpanID() {
		t.Errorf("log span_id %s != span span_id %s", rec.SpanID(), span.SpanContext.SpanID())
	}
	if !rec.TraceFlags().IsSampled() {
		t.Errorf("log trace flags not sampled (%v)", rec.TraceFlags())
	}
}

// TestEmitLog_UsesSuppliedTimestamp guards the trace->logs time-correlation
// fix: when LogInput.Timestamp is set, the record must use that simulated time
// (so it lands inside the corresponding span's window) rather than wall-clock
// now(). A deep child span's simulated start is far in the future relative to
// the real clock; the log must follow the span, not the wall clock.
func TestEmitLog_UsesSuppliedTimestamp(t *testing.T) {
	tp, mp, lp, spanExporter, _, recorder := newTestProviders(t)
	syn := NewDefault(singleProviderFactory{tp: tp, lp: lp}, mp, nil)

	svc := makeSpanService("checkout", topology.KindApplication)

	simulatedOffset := 30 * time.Second
	spanStart := time.Now().Add(simulatedOffset)
	spanEnd := spanStart.Add(200 * time.Millisecond)

	ctx, finish := syn.BeginSpan(context.Background(), SpanInput{
		Service:     svc,
		Operation:   "POST /checkout",
		StartTime:   spanStart,
		InstanceIdx: 0,
	})
	finish(Outcome{Success: true, StatusCode: 200, EndTime: spanEnd})
	syn.EmitLog(ctx, LogInput{
		Service:   svc,
		Severity:  otellog.SeverityInfo,
		Body:      "checkout ok",
		Timestamp: spanEnd,
	})

	span := spanExporter.GetSpans()[0]
	rec := recorder.Records()[0]

	if !rec.Timestamp().Equal(spanEnd) {
		t.Errorf("record Timestamp = %s, want supplied %s", rec.Timestamp(), spanEnd)
	}
	if !rec.ObservedTimestamp().Equal(spanEnd) {
		t.Errorf("record ObservedTimestamp = %s, want supplied %s", rec.ObservedTimestamp(), spanEnd)
	}
	if ts := rec.Timestamp(); ts.Before(span.StartTime) || ts.After(span.EndTime) {
		t.Errorf("log timestamp %s outside span window [%s, %s]; time-windowed trace->logs search would miss it",
			ts.Format(time.RFC3339Nano),
			span.StartTime.Format(time.RFC3339Nano),
			span.EndTime.Format(time.RFC3339Nano))
	}
}

// TestEmitLog_ZeroTimestampFallsBackToNow ensures the backward-compatible
// fallback path still stamps a non-zero time when no simulated time is given.
func TestEmitLog_ZeroTimestampFallsBackToNow(t *testing.T) {
	tp, mp, lp, _, _, recorder := newTestProviders(t)
	syn := NewDefault(singleProviderFactory{tp: tp, lp: lp}, mp, nil)

	before := time.Now()
	syn.EmitLog(context.Background(), LogInput{
		Service:  makeSpanService("frontend", topology.KindApplication),
		Severity: otellog.SeverityInfo,
		Body:     "no timestamp supplied",
	})
	after := time.Now()

	rec := requireSingleLog(t, recorder)
	ts := rec.Timestamp()
	if ts.Before(before) || ts.After(after) {
		t.Errorf("fallback Timestamp %s not within [%s, %s]", ts, before, after)
	}
	if rec.ObservedTimestamp().IsZero() {
		t.Error("fallback ObservedTimestamp is zero")
	}
}
