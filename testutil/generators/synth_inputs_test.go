// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"pgregory.net/rapid"
)

func TestValidSpanInput_PassesValidation_Property(t *testing.T) {
	t.Parallel()

	syn := newGeneratorSynthesizer(t)
	rapid.Check(t, func(t *rapid.T) {
		in := ValidSpanInput().Draw(t, "in")
		if panicked(func() {
			_, finish := syn.BeginSpan(context.Background(), in)
			finish(synth.Outcome{Success: true, StatusCode: 200, EndTime: in.StartTime.Add(time.Millisecond)})
		}) {
			t.Fatalf("ValidSpanInput panicked: %#v", in)
		}
	})
}

func TestAnySpanInput_SometimesInvalid_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		invalid := false
		for i := 0; i < 100; i++ {
			in := AnySpanInput().Draw(t, fmt.Sprintf("in_%d", i))
			if !validSpanShape(in) {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnySpanInput produced no invalid values in 100 draws")
		}
	})
}

func TestValidMetricInput_PassesValidation_Property(t *testing.T) {
	t.Parallel()

	syn := newGeneratorSynthesizer(t)
	rapid.Check(t, func(t *rapid.T) {
		in := ValidMetricInput().Draw(t, "in")
		if panicked(func() {
			syn.RecordMetric(context.Background(), in)
		}) {
			t.Fatalf("ValidMetricInput panicked: %#v", in)
		}
	})
}

func TestAnyMetricInput_SometimesInvalid_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		invalid := false
		for i := 0; i < 100; i++ {
			in := AnyMetricInput().Draw(t, fmt.Sprintf("in_%d", i))
			if !validMetricShape(in) {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyMetricInput produced no invalid values in 100 draws")
		}
	})
}

func TestValidLogInput_PassesValidation_Property(t *testing.T) {
	t.Parallel()

	syn := newGeneratorSynthesizer(t)
	rapid.Check(t, func(t *rapid.T) {
		in := ValidLogInput().Draw(t, "in")
		if panicked(func() {
			syn.EmitLog(context.Background(), in)
		}) {
			t.Fatalf("ValidLogInput panicked: %#v", in)
		}
	})
}

func TestAnyLogInput_SometimesInvalid_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		invalid := false
		for i := 0; i < 100; i++ {
			in := AnyLogInput().Draw(t, fmt.Sprintf("in_%d", i))
			if in.Service == nil {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyLogInput produced no invalid values in 100 draws")
		}
	})
}

func TestValidOutcome_Invariants_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		out := ValidOutcome().Draw(t, "out")
		if !validOutcomeShape(out) {
			t.Fatalf("ValidOutcome produced invalid outcome: %#v", out)
		}
	})
}

func TestAnyOutcome_SometimesInvalid_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		invalid := false
		for i := 0; i < 100; i++ {
			out := AnyOutcome().Draw(t, fmt.Sprintf("out_%d", i))
			if !validOutcomeShape(out) {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyOutcome produced no invalid values in 100 draws")
		}
	})
}

func newGeneratorSynthesizer(t *testing.T) synth.Synthesizer {
	t.Helper()

	tp := sdktrace.NewTracerProvider()
	mp := sdkmetric.NewMeterProvider()
	lp := sdklog.NewLoggerProvider()
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
		_ = lp.Shutdown(context.Background())
	})
	return synth.NewDefault(fixedFactory{tp: tp, lp: lp}, mp)
}

// fixedFactory routes every service to one shared tracer/logger provider,
// ignoring the per-service resource.
type fixedFactory struct {
	tp oteltrace.TracerProvider
	lp otellog.LoggerProvider
}

func (f fixedFactory) TracerProviderForService(string, *sdkresource.Resource) oteltrace.TracerProvider {
	return f.tp
}

func (f fixedFactory) LoggerProviderForService(string, *sdkresource.Resource) otellog.LoggerProvider {
	return f.lp
}

func validSpanShape(in synth.SpanInput) bool {
	return in.Service != nil &&
		in.Operation != "" &&
		in.Service.Replicas > 0 &&
		in.InstanceIdx >= 0 &&
		in.InstanceIdx < in.Service.Replicas
}

func validMetricShape(in synth.MetricInput) bool {
	return in.Service != nil &&
		in.Operation != "" &&
		in.Service.Replicas > 0 &&
		in.InstanceIdx >= 0 &&
		in.InstanceIdx < in.Service.Replicas &&
		in.Latency >= 0
}

func validOutcomeShape(out synth.Outcome) bool {
	if out.StatusCode < 0 {
		return false
	}
	if !out.Success && out.ErrorType == "" {
		return false
	}
	if out.Success && out.ErrorType != "" {
		return false
	}
	return true
}

func panicked(fn func()) (didPanic bool) {
	defer func() {
		didPanic = recover() != nil
	}()
	fn()
	return false
}
