// SPDX-License-Identifier: Apache-2.0

package k6output

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.k6.io/k6/metrics"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestNew_HappyPath(t *testing.T) {
	t.Parallel()

	out, err := New(newTestParams(t, "endpoint=localhost:4317,protocol=grpc,insecure=true"))
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if _, ok := out.(*Output); !ok {
		t.Fatalf("New() = %T, want *Output", out)
	}
}

func TestNew_InvalidArgs_ReturnsError(t *testing.T) {
	t.Parallel()

	_, err := New(newTestParams(t, "protocol=udp"))
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("New() error = %v, want wrapped *ConfigError", err)
	}
	if cfgErr.Kind != ConfigErrorKindInvalidProtocol {
		t.Fatalf("ConfigError.Kind = %q, want invalid_protocol", cfgErr.Kind)
	}
}

func TestDescription_ContainsEndpoint(t *testing.T) {
	t.Parallel()

	o := &Output{params: Params{Endpoint: "collector.example.com:4317"}}
	if got := o.Description(); !strings.Contains(got, "collector.example.com:4317") {
		t.Fatalf("Description() = %q, want endpoint", got)
	}
}

func TestDescription_NilOutput(t *testing.T) {
	t.Parallel()

	var o *Output
	if got := o.Description(); got == "" {
		t.Fatal("nil Description() returned empty string")
	}
}

func TestStart_Idempotent(t *testing.T) {
	t.Parallel()

	o := newTestOutput(t, "endpoint=localhost:4317,insecure=true,timeout=1ms,batchTimeout=1h")
	if err := o.Start(); err != nil {
		t.Fatalf("Start() first error = %v, want nil", err)
	}
	if err := o.Start(); err != nil {
		t.Fatalf("Start() second error = %v, want nil", err)
	}
	if err := o.Stop(); err != nil {
		t.Fatalf("Stop() error = %v, want nil", err)
	}
}

func TestStart_PipelineFailure_ReturnsError(t *testing.T) {
	t.Parallel()

	o := newTestOutput(t, "endpoint=localhost:4317,timeout=-1s")
	err := o.Start()
	if err == nil {
		t.Fatal("Start() error = nil, want pipeline init error")
	}
	if !strings.Contains(err.Error(), "k6output: pipeline init") {
		t.Fatalf("Start() error = %v, want pipeline init wrapper", err)
	}
}

func TestAddMetricSamples_BeforeStart_NoOp(t *testing.T) {
	t.Parallel()

	o := newTestOutput(t, "")
	o.AddMetricSamples(nil)
}

func TestAddMetricSamples_AfterStop_NoOp(t *testing.T) {
	t.Parallel()

	o := newTestOutput(t, "")
	if err := o.Stop(); err != nil {
		t.Fatalf("Stop() error = %v, want nil", err)
	}
	o.AddMetricSamples(nil)
}

func TestAddMetricSamples_ContextDone_NoOp(t *testing.T) {
	t.Parallel()

	o := &Output{queue: make(chan metrics.SampleContainer, 1)}
	o.ctx, o.cancelFn = context.WithCancel(context.Background())
	o.cancelFn()
	o.AddMetricSamples([]metrics.SampleContainer{testK6Sample(t, "iterations", metrics.Counter, 1, nil)})
	if got := len(o.queue); got != 0 {
		t.Fatalf("queue len = %d, want 0", got)
	}
}

func TestTryPush_DropsOldestWhenFull(t *testing.T) {
	t.Parallel()

	logger, logs := recordingLogger()
	o := &Output{
		queue:  make(chan metrics.SampleContainer, 1),
		logger: logger,
	}
	oldest := testK6Sample(t, "iterations", metrics.Counter, 1, nil)
	newest := testK6Sample(t, "iterations", metrics.Counter, 2, nil)
	o.queue <- oldest
	if ok := o.tryPush(newest); !ok {
		t.Fatal("tryPush() = false, want true")
	}
	if got := o.drops.Load(); got != 1 {
		t.Fatalf("drops = %d, want 1", got)
	}
	got := <-o.queue
	if value := got.GetSamples()[0].Value; value != 2 {
		t.Fatalf("queued sample value = %v, want newest value 2", value)
	}
	if len(*logs) == 0 {
		t.Fatal("queue overflow produced no warning log")
	}
}

func TestStop_Idempotent(t *testing.T) {
	t.Parallel()

	o := newTestOutput(t, "")
	if err := o.Stop(); err != nil {
		t.Fatalf("Stop() first error = %v, want nil", err)
	}
	if err := o.Stop(); err != nil {
		t.Fatalf("Stop() second error = %v, want nil", err)
	}
}

func TestStop_AlwaysReturnsNil(t *testing.T) {
	t.Parallel()

	logger, logs := recordingLogger()
	o := &Output{
		pipeline: &failingPipeline{err: errors.New("shutdown failed")},
		logger:   logger,
	}
	if err := o.Stop(); err != nil {
		t.Fatalf("Stop() error = %v, want nil", err)
	}
	if len(*logs) == 0 || !strings.Contains((*logs)[0], "Shutdown") {
		t.Fatalf("logs = %#v, want shutdown warning", *logs)
	}
}

func TestLookupOrBuildInstrument_LazyUnknownTypes(t *testing.T) {
	t.Parallel()

	o, _ := newManualTestOutput(t)
	tests := []struct {
		name   string
		typ    metrics.MetricType
		unit   string
		wantOK bool
	}{
		{name: "custom_counter", typ: metrics.Counter, unit: "{count}", wantOK: true},
		{name: "custom_rate", typ: metrics.Rate, unit: "{event}", wantOK: true},
		{name: "custom_trend", typ: metrics.Trend, unit: "ms", wantOK: true},
		{name: "custom_gauge", typ: metrics.Gauge, unit: "{vu}", wantOK: true},
		{name: "custom_bad", typ: metrics.MetricType(-1), unit: "", wantOK: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := o.lookupOrBuildInstrument(tt.name, tt.typ, tt.unit)
			if (got != nil) != tt.wantOK {
				t.Fatalf("lookupOrBuildInstrument() = %T, wantOK %v", got, tt.wantOK)
			}
		})
	}
	if !stringsHasTotalSuffix("k6.requests.total") {
		t.Fatal("stringsHasTotalSuffix() = false, want true")
	}
	if stringsHasTotalSuffix("k6.requests") {
		t.Fatal("stringsHasTotalSuffix(non-total) = true, want false")
	}
}

func TestEmitSample_RateAndNilMetric(t *testing.T) {
	t.Parallel()

	o, reader := newManualTestOutput(t)
	o.emitSample(metrics.Sample{})
	o.emitSample(testK6Sample(t, "http_req_failed", metrics.Rate, 0, nil))
	o.emitSample(testK6Sample(t, "http_req_failed", metrics.Rate, 1, nil))

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("ManualReader.Collect() error = %v", err)
	}
	got, ok := sumMetricValue(rm, "k6.http.request.failed.total")
	if !ok {
		t.Fatal("k6.http.request.failed.total not found")
	}
	if got != 1 {
		t.Fatalf("rate counter sum = %v, want 1", got)
	}
}

type failingPipeline struct {
	err error
}

func (p *failingPipeline) MetricExporter() sdkmetric.Exporter {
	return nil
}

func (p *failingPipeline) Shutdown(context.Context) error {
	return p.err
}

var _ sharedPipeline = (*failingPipeline)(nil)
