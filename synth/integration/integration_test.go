// SPDX-License-Identifier: Apache-2.0

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestIntegration_SynthToCollector_ThreeSignals_Correlated(t *testing.T) {
	endpoint, cleanup := StartCollector(t)
	defer cleanup()

	p := BuildPipeline(t, exporter.Config{
		Endpoint:     endpoint,
		Insecure:     true,
		Timeout:      5 * time.Second,
		BatchSize:    1,
		BatchTimeout: 100 * time.Millisecond,
		MaxQueueSize: 8,
	})

	syn := synth.NewDefault(p.TracerProvider(), p.MeterProvider(), p.LoggerProvider())
	svc := &topology.Service{Name: "frontend", Kind: topology.KindApplication, Replicas: 1}
	start := time.Now()
	ctx, finish := syn.BeginSpan(context.Background(), synth.SpanInput{
		Service:     svc,
		Operation:   "GET /checkout",
		StartTime:   start,
		InstanceIdx: 0,
	})
	spanCtx := trace.SpanContextFromContext(ctx)
	traceID := spanCtx.TraceID().String()
	spanID := spanCtx.SpanID().String()

	syn.RecordMetric(ctx, synth.MetricInput{
		Service:     svc,
		Operation:   "GET /checkout",
		Latency:     25 * time.Millisecond,
		Outcome:     synth.Outcome{Success: true, StatusCode: 200},
		InstanceIdx: 0,
	})
	syn.EmitLog(ctx, synth.LogInput{
		Service: svc,
		Body:    "frontend checkout",
		Attributes: map[string]any{
			"trace_id": traceID,
			"span_id":  spanID,
		},
	})
	finish(synth.Outcome{Success: true, StatusCode: 200, EndTime: start.Add(25 * time.Millisecond)})

	flushCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	forceFlush(t, flushCtx, p)
	if err := p.Shutdown(flushCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	traces := string(ReadCollectorTraces(t))
	metrics := string(ReadCollectorMetrics(t))
	logs := string(ReadCollectorLogs(t))
	assertContains(t, "traces", traces, traceID)
	assertContains(t, "traces", traces, spanID)
	assertContains(t, "metrics", metrics, "http.server.request.duration")
	assertContains(t, "metrics", metrics, traceID)
	assertContains(t, "metrics", metrics, spanID)
	assertContains(t, "logs", logs, traceID)
	assertContains(t, "logs", logs, spanID)
	assertContains(t, "logs", logs, "frontend checkout")
}

func forceFlush(t *testing.T, ctx context.Context, p *exporter.Pipeline) {
	t.Helper()

	if provider, ok := p.TracerProvider().(*sdktrace.TracerProvider); ok {
		if err := provider.ForceFlush(ctx); err != nil {
			t.Fatalf("trace ForceFlush() error = %v", err)
		}
	}
	if provider, ok := p.MeterProvider().(*sdkmetric.MeterProvider); ok {
		if err := provider.ForceFlush(ctx); err != nil {
			t.Fatalf("metric ForceFlush() error = %v", err)
		}
	}
	if provider, ok := p.LoggerProvider().(*sdklog.LoggerProvider); ok {
		if err := provider.ForceFlush(ctx); err != nil {
			t.Fatalf("log ForceFlush() error = %v", err)
		}
	}
}

func assertContains(t *testing.T, fileName, body, want string) {
	t.Helper()

	if !strings.Contains(body, want) {
		t.Fatalf("%s output does not contain %q:\n%s", fileName, want, body)
	}
}
