package k6output

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
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
