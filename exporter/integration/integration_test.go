//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestIntegration_ThreeSignals_Correlated(t *testing.T) {
	endpoint, cleanup := StartCollector(t)
	defer cleanup()

	p, err := exporter.New(exporter.Config{
		Endpoint:     endpoint,
		Insecure:     true,
		Timeout:      5 * time.Second,
		BatchSize:    1,
		BatchTimeout: 100 * time.Millisecond,
		MaxQueueSize: 8,
		ResourceOverrides: map[string]string{
			"service.name":      "u4-integration",
			"service.namespace": "xk6-otel-gen",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	if err != nil {
		t.Fatalf("parse trace ID: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("1112131415161718")
	if err != nil {
		t.Fatalf("parse span ID: %v", err)
	}
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	ctx = trace.ContextWithSpanContext(ctx, parent)

	ctx, span := p.TracerProvider().Tracer("u4-integration").Start(ctx, "integration.operation")
	active := span.SpanContext()
	traceIDHex := active.TraceID().String()
	spanIDHex := active.SpanID().String()

	meter := p.MeterProvider().Meter("u4-integration")
	counter, err := meter.Int64Counter("integration.requests")
	if err != nil {
		t.Fatalf("Int64Counter() error = %v", err)
	}
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("trace_id", traceIDHex),
		attribute.String("span_id", spanIDHex),
		attribute.String("signal", "metric"),
	))

	record := otellog.Record{}
	record.SetTimestamp(time.Now())
	record.SetObservedTimestamp(time.Now())
	record.SetSeverity(otellog.SeverityInfo)
	record.SetBody(otellog.StringValue("integration log"))
	record.AddAttributes(
		otellog.String("trace_id", traceIDHex),
		otellog.String("span_id", spanIDHex),
		otellog.String("signal", "log"),
	)
	p.LoggerProvider().Logger("u4-integration").Emit(ctx, record)
	span.End()

	flushCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	forceFlush(t, flushCtx, p)
	if err := p.Shutdown(flushCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	traces := string(ReadCollectorTraces(t))
	metrics := string(ReadCollectorMetrics(t))
	logs := string(ReadCollectorLogs(t))
	for _, output := range []struct {
		name string
		body string
	}{
		{name: "traces", body: traces},
		{name: "metrics", body: metrics},
		{name: "logs", body: logs},
	} {
		assertContains(t, output.name, output.body, traceIDHex)
		assertContains(t, output.name, output.body, spanIDHex)
		assertContains(t, output.name, output.body, "u4-integration")
		assertContains(t, output.name, output.body, "xk6-otel-gen")
	}
	assertContains(t, "metrics", metrics, "integration.requests")
	assertContains(t, "logs", logs, "integration log")
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
