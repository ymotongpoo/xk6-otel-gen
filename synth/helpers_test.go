// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"context"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// singleProviderFactory routes every service to one shared tracer/logger
// provider, ignoring the per-service resource. It lets the existing unit tests
// (which assert on span/log content, not resource identity) keep using a single
// in-memory recorder after NewDefault switched to a per-service ProviderFactory.
type singleProviderFactory struct {
	tp trace.TracerProvider
	lp log.LoggerProvider
}

func (f singleProviderFactory) TracerProviderForService(string, *sdkresource.Resource) trace.TracerProvider {
	return f.tp
}

func (f singleProviderFactory) LoggerProviderForService(string, *sdkresource.Resource) log.LoggerProvider {
	return f.lp
}

func newTestProviders(t *testing.T) (
	trace.TracerProvider,
	metric.MeterProvider,
	log.LoggerProvider,
	*tracetest.InMemoryExporter,
	*sdkmetric.ManualReader,
	*logRecorder,
) {
	t.Helper()

	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	recorder := &logRecorder{}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(recorder)))

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
		_ = lp.Shutdown(context.Background())
	})

	return tp, mp, lp, spanExporter, reader, recorder
}

type logRecorder struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (r *logRecorder) Export(ctx context.Context, records []sdklog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range records {
		r.records = append(r.records, records[i].Clone())
	}
	return nil
}

func (r *logRecorder) Shutdown(ctx context.Context) error {
	return nil
}

func (r *logRecorder) ForceFlush(ctx context.Context) error {
	return nil
}

func (r *logRecorder) Records() []sdklog.Record {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]sdklog.Record, len(r.records))
	copy(out, r.records)
	return out
}
