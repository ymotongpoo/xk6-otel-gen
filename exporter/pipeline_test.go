// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestNew_Success(t *testing.T) {
	t.Parallel()

	p, err := New(Config{
		Endpoint:     "localhost:4317",
		Insecure:     true,
		Timeout:      500 * time.Millisecond,
		BatchSize:    8,
		BatchTimeout: 10 * time.Millisecond,
		MaxQueueSize: 16,
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if p.TracerProvider() == nil || p.MeterProvider() == nil || p.LoggerProvider() == nil {
		t.Fatalf("providers must be non-nil: trace=%v metric=%v log=%v", p.TracerProvider(), p.MeterProvider(), p.LoggerProvider())
	}
	_ = p.Shutdown(context.Background())
}

func TestPipeline_SamplerTraceIDRatioZero_DropsTracesOnly(t *testing.T) {
	t.Parallel()

	traceExp := &fakeSpanExporter{}
	metricExp := &fakeMetricExporter{}
	logExp := &fakeLogExporter{}
	cfg := validPipelineConfig()
	cfg.Sampler = "traceidratio"
	cfg.SamplerArg = 0
	cfg.SamplerArgSet = true
	p := newPipelineFromExporters(cfg, sdkresource.Empty(), &pipelineStats{}, traceExp, metricExp, logExp)

	ctx := context.Background()
	_, span := p.TracerProvider().Tracer("test").Start(ctx, "dropped")
	span.End()
	counter, err := p.MeterProvider().Meter("test").Int64Counter("test.counter", metric.WithUnit("{count}"))
	if err != nil {
		t.Fatalf("Int64Counter() error = %v", err)
	}
	counter.Add(ctx, 1)
	record := log.Record{}
	record.SetBody(log.StringValue("still logged"))
	p.LoggerProvider().Logger("test").Emit(ctx, record)

	if err := p.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if traceExp.exports != 0 {
		t.Fatalf("trace exports = %d, want 0", traceExp.exports)
	}
	if metricExp.exports == 0 {
		t.Fatal("metric exports = 0, want metrics to flow")
	}
	if logExp.exports == 0 {
		t.Fatal("log exports = 0, want logs to flow")
	}
}

func TestPipeline_MetricExporter_NotNil(t *testing.T) {
	t.Parallel()

	p, err := New(validPipelineConfig())
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if got := p.MetricExporter(); got == nil {
		t.Fatal("MetricExporter() = nil, want non-nil")
	}
	_ = p.Shutdown(context.Background())
}

func TestPipeline_MetricExporter_SameAsInternal(t *testing.T) {
	t.Parallel()

	metricInner := &fakeMetricExporter{}
	p := newPipelineFromExporters(validPipelineConfig(), sdkresource.Empty(), &pipelineStats{}, &fakeSpanExporter{}, metricInner, &fakeLogExporter{})

	got := p.MetricExporter()
	if got != p.metricExp {
		t.Fatalf("MetricExporter() = %v, want internal metric exporter %v", got, p.metricExp)
	}
	if got != metricInner {
		t.Fatalf("MetricExporter() = %v, want constructor exporter %v", got, metricInner)
	}
}

func TestNew_ValidationError(t *testing.T) {
	t.Parallel()

	_, err := New(Config{Endpoint: "localhost:4317", Timeout: -time.Second})
	if err == nil {
		t.Fatal("New() error = nil, want PipelineError")
	}
	var pipeErr *PipelineError
	if !errors.As(err, &pipeErr) {
		t.Fatalf("New() error type = %T, want *PipelineError", err)
	}
	if pipeErr.Stage != "validate" {
		t.Fatalf("PipelineError.Stage = %q, want validate", pipeErr.Stage)
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("New() error = %v, want wrapped ConfigError", err)
	}
}

func TestNew_InvalidSamplerEnvErrorIncludesRawValueAndAllowedSet(t *testing.T) {
	withOTLPEnv(t, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "localhost:4317",
		"OTEL_EXPORTER_OTLP_INSECURE": "true",
		"OTEL_TRACES_SAMPLER":         "parentbased_always_on",
		"OTEL_TRACES_SAMPLER_ARG":     "0.25",
	})

	_, err := New(ConfigFromEnv())
	if err == nil {
		t.Fatal("New() error = nil, want invalid sampler error")
	}
	got := err.Error()
	for _, want := range []string{"parentbased_always_on", "OTEL_TRACES_SAMPLER", "always_on", "always_off", "traceidratio"} {
		if !strings.Contains(got, want) {
			t.Fatalf("New() error = %q, want substring %q", got, want)
		}
	}
}

func TestNew_ExporterFailure_CleansUp(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("metric builder failed")
	traceInner := &fakeSpanExporter{}
	_, err := newWithExporterBuilders(validPipelineConfig(),
		func(context.Context, Config, *tls.Config, *pipelineStats) (sdktrace.SpanExporter, error) {
			return traceInner, nil
		},
		func(context.Context, Config, *tls.Config, *pipelineStats) (sdkmetric.Exporter, error) {
			return nil, sentinel
		},
		func(context.Context, Config, *tls.Config, *pipelineStats) (sdklog.Exporter, error) {
			t.Fatal("log builder should not be called after metric builder failure")
			return nil, nil
		},
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("newWithExporterBuilders() error = %v, want %v", err, sentinel)
	}
	var pipeErr *PipelineError
	if !errors.As(err, &pipeErr) || pipeErr.Stage != "metric_exporter" {
		t.Fatalf("newWithExporterBuilders() error = %v, want metric_exporter PipelineError", err)
	}
	if traceInner.shutdown != 1 {
		t.Fatalf("trace shutdown count = %d, want 1", traceInner.shutdown)
	}
}

func TestPipeline_Shutdown_Idempotent(t *testing.T) {
	t.Parallel()

	traceInner := &fakeSpanExporter{}
	metricInner := &fakeMetricExporter{}
	logInner := &fakeLogExporter{}
	p := newPipelineFromExporters(validPipelineConfig(), sdkresource.Empty(), &pipelineStats{}, traceInner, metricInner, logInner)

	first := p.Shutdown(context.Background())
	second := p.Shutdown(context.Background())
	if first != second {
		t.Fatalf("second Shutdown() error = %v, want same value as first %v", second, first)
	}
	if traceInner.shutdown != 1 || metricInner.shutdown != 1 || logInner.shutdown != 1 {
		t.Fatalf("shutdown counts = trace:%d metric:%d log:%d, want all 1", traceInner.shutdown, metricInner.shutdown, logInner.shutdown)
	}
}

func TestPipeline_ForceFlush_DeliversSpansWithoutClosing(t *testing.T) {
	t.Parallel()

	traceInner := &fakeSpanExporter{}
	metricInner := &fakeMetricExporter{}
	logInner := &fakeLogExporter{}
	p := newPipelineFromExporters(validPipelineConfig(), sdkresource.Empty(), &pipelineStats{}, traceInner, metricInner, logInner)
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })

	// A span that has ended sits in the batch processor's queue until the
	// batch timeout or an explicit flush. ForceFlush must drain it now.
	_, span := p.TracerProvider().Tracer("test").Start(context.Background(), "root")
	span.End()

	if err := p.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() error = %v", err)
	}
	if traceInner.exports == 0 {
		t.Fatalf("ForceFlush() did not export queued spans (exports = 0)")
	}
	if traceInner.shutdown != 0 {
		t.Fatalf("ForceFlush() closed the exporter (shutdown = %d, want 0)", traceInner.shutdown)
	}
}

func TestPipeline_Stats_DelegatesToSnapshot(t *testing.T) {
	t.Parallel()

	stats := &pipelineStats{}
	stats.tracesExported.Add(1)
	stats.metricsFailed.Add(2)
	stats.logsExported.Add(3)
	p := &Pipeline{stats: stats}

	got := p.Stats()
	want := Stats{TracesExported: 1, MetricsFailed: 2, LogsExported: 3}
	if got != want {
		t.Fatalf("Stats() = %#v, want %#v", got, want)
	}
}

func validPipelineConfig() Config {
	return Config{
		Protocol:     ProtocolGRPC,
		Endpoint:     "localhost:4317",
		Insecure:     true,
		Timeout:      500 * time.Millisecond,
		BatchSize:    8,
		BatchTimeout: 10 * time.Millisecond,
		MaxQueueSize: 16,
	}
}
