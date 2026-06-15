// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"sync"
	"testing"
	"time"

	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

// reproFactory builds a real per-service TracerProvider/LoggerProvider for each
// synthetic service (carrying that service's resource), all routed to one
// in-memory span exporter and log recorder. This exercises the same
// per-service-resource path as production.
type reproFactory struct {
	spanExp *tracetest.InMemoryExporter
	logRec  *reproLogRecorder
}

func (f *reproFactory) TracerProviderForService(_ string, res *sdkresource.Resource) trace.TracerProvider {
	return sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithSyncer(f.spanExp))
}

func (f *reproFactory) LoggerProviderForService(_ string, res *sdkresource.Resource) otellog.LoggerProvider {
	return sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewSimpleProcessor(f.logRec)))
}

type reproLogRecorder struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (r *reproLogRecorder) Export(_ context.Context, recs []sdklog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range recs {
		r.records = append(r.records, recs[i].Clone())
	}
	return nil
}
func (r *reproLogRecorder) Shutdown(context.Context) error   { return nil }
func (r *reproLogRecorder) ForceFlush(context.Context) error { return nil }
func (r *reproLogRecorder) all() []sdklog.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]sdklog.Record, len(r.records))
	copy(out, r.records)
	return out
}

// reproLatencySchema builds api -> payments -> db with a fixed 2s latency per
// hop, so a single journey iteration produces spans whose simulated timestamps
// stretch several seconds into the future relative to the real wall clock.
func reproLatencySchema() *topology.Schema {
	api := &topology.Service{Name: "api", Kind: topology.KindApplication, Replicas: 1, Operations: map[string]*topology.Operation{}}
	payments := &topology.Service{Name: "payments", Kind: topology.KindApplication, Replicas: 1, Operations: map[string]*topology.Operation{}}
	db := &topology.Service{Name: "db", Kind: topology.KindDatabase, Replicas: 1, Operations: map[string]*topology.Operation{}}

	checkout := &topology.Operation{Name: "GET /checkout", Service: api}
	charge := &topology.Operation{Name: "POST /charge", Service: payments}
	insert := &topology.Operation{Name: "INSERT payment", Service: db}
	api.Operations[checkout.Name] = checkout
	payments.Operations[charge.Name] = charge
	db.Operations[insert.Name] = insert

	twoSec := topology.LatencyDist{Distribution: "fixed", P50: 2 * time.Second}
	checkout.Calls = []*topology.CallNode{{Edge: &topology.Edge{From: checkout, To: charge, Protocol: topology.ProtocolHTTP, Latency: twoSec}}}
	charge.Calls = []*topology.CallNode{{Edge: &topology.Edge{From: charge, To: insert, Protocol: topology.ProtocolHTTP, Latency: twoSec}}}

	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{api.Name: api, payments.Name: payments, db.Name: db},
		Journeys: map[string]*topology.Journey{"checkout": {Name: "checkout", Steps: []*topology.Step{{Op: checkout}}}},
	}
}

// TestExecute_LogsAlignWithSpanTimeline drives the REAL journey engine + real
// synth + real OTel SDK providers, then checks every emitted log against the
// time window of the span it is correlated to (matched by span_id). It guards
// the trace->logs correlation fix: logs must carry the right trace_id/span_id
// AND fall inside their span's simulated time window, so Grafana's
// time-windowed trace->logs query finds them even for deep, high-latency spans.
func TestExecute_LogsAlignWithSpanTimeline(t *testing.T) {
	spanExp := tracetest.NewInMemoryExporter()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))
	rec := &reproLogRecorder{}
	t.Cleanup(func() {
		_ = mp.Shutdown(context.Background())
	})

	syn := synth.NewDefault(&reproFactory{spanExp: spanExp, logRec: rec}, mp, nil)
	schema := reproLatencySchema()
	eng := NewEngineWithSeed(schema, schema.ApplyFaults(), syn, 42)

	plan := eng.impl.plans["checkout"]
	if plan == nil {
		t.Fatal("no checkout plan")
	}
	if err := eng.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	spans := spanExp.GetSpans()
	logs := rec.all()
	t.Logf("emitted %d spans, %d logs", len(spans), len(logs))

	// Index spans by span_id.
	type window struct {
		name       string
		start, end time.Time
	}
	byID := map[trace.SpanID]window{}
	for _, s := range spans {
		byID[s.SpanContext.SpanID()] = window{name: s.Name, start: s.StartTime, end: s.EndTime}
		t.Logf("SPAN  %-22s [%s .. %s]", s.Name,
			s.StartTime.Format("15:04:05.000"), s.EndTime.Format("15:04:05.000"))
	}

	misses := 0
	for _, l := range logs {
		w, ok := byID[l.SpanID()]
		if !ok {
			t.Logf("LOG   span_id=%s has NO matching span (trace_id=%s)", l.SpanID(), l.TraceID())
			misses++
			continue
		}
		ts := l.Timestamp()
		outside := ts.Before(w.start) || ts.After(w.end)
		marker := "inside"
		if outside {
			marker = "OUTSIDE -> not found by time-windowed search"
			misses++
		}
		t.Logf("LOG   %-22s ts=%s  span[%s..%s]  %s", w.name,
			ts.Format("15:04:05.000"),
			w.start.Format("15:04:05.000"), w.end.Format("15:04:05.000"), marker)
	}

	if misses > 0 {
		t.Errorf("%d/%d logs fall outside their span's time window; trace->logs correlation would miss them (logs must use the span's simulated timestamp, not wall-clock now())", misses, len(logs))
	}

	// Each span must carry its synthetic service's service.name as a RESOURCE
	// attribute (not just a span attribute), or Tempo/Grafana shows the trace as
	// OTLPResourceNoServiceName. Map span name prefix -> expected service.name.
	wantSvc := map[string]string{
		"api.GET /checkout":     "api",
		"payments.POST /charge": "payments",
		"db.INSERT payment":     "db",
	}
	for _, s := range spans {
		var got string
		for _, kv := range s.Resource.Attributes() {
			if kv.Key == "service.name" {
				got = kv.Value.AsString()
			}
		}
		want := wantSvc[s.Name]
		if got == "" {
			t.Errorf("span %q resource has no service.name (OTLPResourceNoServiceName)", s.Name)
			continue
		}
		if want != "" && got != want {
			t.Errorf("span %q resource service.name = %q, want %q", s.Name, got, want)
		}
	}
}
