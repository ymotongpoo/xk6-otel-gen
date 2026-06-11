// SPDX-License-Identifier: Apache-2.0

package synth_test

import (
	"context"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"pgregory.net/rapid"
)

func TestBuildResource_Idempotent_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		svc := generators.ValidService().Draw(rt, "svc")
		idx := rapid.IntRange(0, svc.Replicas-1).Draw(rt, "idx")

		first := synth.BuildResource(svc, idx)
		second := synth.BuildResource(svc, idx)
		if !first.Equal(second) {
			rt.Fatalf("BuildResource not idempotent for svc=%q idx=%d", svc.Name, idx)
		}
	})
}

func TestSpanAttributes_AllowedKeysOnly_Property(t *testing.T) {
	t.Parallel()

	syn, spanExporter, _ := newPBTSynthesizer(t)

	rapid.Check(t, func(rt *rapid.T) {
		spanExporter.Reset()
		in := generators.ValidSpanInput().Draw(rt, "in")
		_, finish := syn.BeginSpan(context.Background(), in)
		finish(synth.Outcome{
			Success:    false,
			StatusCode: statusCodeForEdge(in.Edge),
			ErrorType:  "timeout",
			EndTime:    in.StartTime.Add(time.Millisecond),
		})

		spans := spanExporter.GetSpans()
		if len(spans) != 1 {
			rt.Fatalf("spans = %d, want 1", len(spans))
		}
		for _, kv := range spans[0].Attributes {
			if _, ok := allowedSpanAttrKeys[string(kv.Key)]; !ok {
				rt.Fatalf("attribute key %q not in allowed key set", kv.Key)
			}
		}
	})
}

func TestRecordMetric_HistogramInsertion_Property(t *testing.T) {
	t.Parallel()

	syn, _, reader := newPBTSynthesizer(t)

	rapid.Check(t, func(rt *rapid.T) {
		in := generators.ValidMetricInput().Draw(rt, "in")
		name := histogramNameForMetricInput(in)
		before := histogramCount(rt, reader, name)
		syn.RecordMetric(context.Background(), in)
		after := histogramCount(rt, reader, name)
		if after != before+1 {
			rt.Fatalf("%s count after RecordMetric = %d, want %d", name, after, before+1)
		}
	})
}

func TestFinishSpan_ErrorTypeRequired_Property(t *testing.T) {
	t.Parallel()

	syn, spanExporter, _ := newPBTSynthesizer(t)

	rapid.Check(t, func(rt *rapid.T) {
		spanExporter.Reset()
		in := generators.ValidSpanInput().Draw(rt, "in")
		errType := generators.ValidErrorType().Draw(rt, "error_type")
		_, finish := syn.BeginSpan(context.Background(), in)
		finish(synth.Outcome{
			Success:    false,
			StatusCode: statusCodeForEdge(in.Edge),
			ErrorType:  errType,
			EndTime:    in.StartTime.Add(time.Millisecond),
		})

		spans := spanExporter.GetSpans()
		if len(spans) != 1 {
			rt.Fatalf("spans = %d, want 1", len(spans))
		}
		for _, kv := range spans[0].Attributes {
			if string(kv.Key) == string(semconv.ErrorTypeKey) {
				if got := kv.Value.AsString(); got != errType {
					rt.Fatalf("error.type = %q, want %q", got, errType)
				}
				return
			}
		}
		rt.Fatal("error.type attribute missing")
	})
}

func newPBTSynthesizer(t *testing.T) (synth.Synthesizer, *tracetest.InMemoryExporter, *sdkmetric.ManualReader) {
	t.Helper()

	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	lp := sdklog.NewLoggerProvider()
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
		_ = lp.Shutdown(context.Background())
	})
	return synth.NewDefault(tp, mp, lp), spanExporter, reader
}

func statusCodeForEdge(edge *topology.Edge) int {
	if edge != nil && edge.Protocol == topology.ProtocolGRPC {
		return 14
	}
	return 500
}

func histogramCount(t *rapid.T, reader *sdkmetric.ManualReader, name string) uint64 {
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	var count uint64
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("%s data = %T, want metricdata.Histogram[float64]", name, metric.Data)
			}
			for _, point := range histogram.DataPoints {
				count += point.Count
			}
		}
	}
	return count
}

func histogramNameForMetricInput(in synth.MetricInput) string {
	switch in.Service.Kind {
	case topology.KindDatabase, topology.KindCache:
		return "db.client.operation.duration"
	case topology.KindQueue:
		if in.Edge != nil && in.Edge.To != nil && in.Edge.To.Service == in.Service {
			return "messaging.receive.duration"
		}
		return "messaging.publish.duration"
	case topology.KindExternalAPI:
		return "http.client.request.duration"
	}
	if in.Edge == nil {
		return "http.server.request.duration"
	}
	switch in.Edge.Protocol {
	case topology.ProtocolGRPC:
		if in.Edge.To != nil && in.Edge.To.Service == in.Service {
			return "rpc.server.duration"
		}
		return "rpc.client.duration"
	case topology.ProtocolMessaging:
		if in.Edge.To != nil && in.Edge.To.Service == in.Service {
			return "messaging.receive.duration"
		}
		return "messaging.publish.duration"
	default:
		if in.Edge.To != nil && in.Edge.To.Service == in.Service {
			return "http.server.request.duration"
		}
		return "http.client.request.duration"
	}
}

var allowedSpanAttrKeys = map[string]struct{}{
	string(semconv.ServiceNameKey):              {},
	string(semconv.HTTPRequestMethodKey):        {},
	string(semconv.HTTPResponseStatusCodeKey):   {},
	string(semconv.HTTPRouteKey):                {},
	string(semconv.ServerAddressKey):            {},
	string(semconv.ServerPortKey):               {},
	string(semconv.URLPathKey):                  {},
	string(semconv.RPCSystemKey):                {},
	string(semconv.RPCServiceKey):               {},
	string(semconv.RPCMethodKey):                {},
	string(semconv.RPCGRPCStatusCodeKey):        {},
	string(semconv.DBSystemKey):                 {},
	string(semconv.DBOperationNameKey):          {},
	string(semconv.MessagingSystemKey):          {},
	string(semconv.MessagingOperationNameKey):   {},
	string(semconv.MessagingDestinationNameKey): {},
	string(semconv.ErrorTypeKey):                {},
	"peer.service":                              {},
	"outcome":                                   {},
	"synth.service.framework":                   {},
}
