// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// fixedFactory routes every service to one shared tracer/logger provider,
// ignoring the per-service resource. Used by benchmarks that don't assert on
// resource identity.
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

const sustainedBudgetDuration = 3 * time.Second

func BenchmarkE2EAstroshopJourney(b *testing.B) {
	engine, cleanup := newAstroshopBenchEngine(b)
	defer cleanup()

	ctx := context.Background()
	var executed atomic.Int64
	start := time.Now()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := executePickedJourney(ctx, engine); err != nil {
				b.Fatalf("execute journey: %v", err)
			}
			executed.Add(1)
		}
	})
	elapsed := time.Since(start)
	b.ReportMetric(float64(executed.Load())/elapsed.Seconds(), "journeys/sec")
}

func TestSustained1kRPSBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sustained benchmark guard in short mode")
	}
	if raceEnabled {
		t.Skip("skipping sustained benchmark guard under the race detector")
	}

	engine, cleanup := newAstroshopBenchEngine(t)
	defer cleanup()

	ctx := context.Background()
	deadline := time.Now().Add(sustainedBudgetDuration)
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}

	var executed atomic.Int64
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	start := time.Now()
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				if err := executePickedJourney(ctx, engine); err != nil {
					errCh <- err
					return
				}
				executed.Add(1)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("execute journey: %v", err)
		}
	}

	elapsed := time.Since(start)
	journeysPerSec := float64(executed.Load()) / elapsed.Seconds()
	t.Logf("astroshop e2e throughput: %.1f journeys/sec over %s", journeysPerSec, elapsed)
	if journeysPerSec < 1000 {
		t.Fatalf("journeys/sec = %.1f, want >= 1000", journeysPerSec)
	}
}

func newAstroshopBenchEngine(tb testing.TB) (*Engine, func()) {
	tb.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "..", "examples", "astroshop", "topology.yaml")
	schema, err := topology.ParseFile(path)
	if err != nil {
		tb.Fatalf("ParseFile(%s) error = %v", path, err)
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewNoopExporter()))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(noopLogExporter{})))
	engine := NewEngineWithSeed(schema, schema.ApplyFaults(), synth.NewDefault(fixedFactory{tp: tp, lp: lp}, mp), 42)
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
		_ = mp.Shutdown(ctx)
		_ = lp.Shutdown(ctx)
	}
	return engine, cleanup
}

func executePickedJourney(ctx context.Context, engine *Engine) error {
	name := engine.PickJourney()
	if name == "" {
		return fmt.Errorf("no journey selected")
	}
	plan, err := engine.BuildPlan(name)
	if err != nil {
		return err
	}
	return engine.Execute(ctx, plan)
}

type noopLogExporter struct{}

func (noopLogExporter) Export(context.Context, []sdklog.Record) error {
	return nil
}

func (noopLogExporter) Shutdown(context.Context) error {
	return nil
}

func (noopLogExporter) ForceFlush(context.Context) error {
	return nil
}
