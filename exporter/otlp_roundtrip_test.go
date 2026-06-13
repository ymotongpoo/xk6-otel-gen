// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"fmt"
	"testing"

	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	tracecollectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
	"pgregory.net/rapid"
)

func TestOTLPProtobufRoundTrip(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		orig := otlpTraceRequest().Draw(t, "request")
		wire, err := proto.Marshal(orig)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		var parsed tracecollectorpb.ExportTraceServiceRequest
		if err := proto.Unmarshal(wire, &parsed); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if !proto.Equal(orig, &parsed) {
			t.Fatalf("round-trip mismatch:\norig=%v\nparsed=%v", orig, &parsed)
		}
	})
}

func otlpTraceRequest() *rapid.Generator[*tracecollectorpb.ExportTraceServiceRequest] {
	return rapid.Custom(func(t *rapid.T) *tracecollectorpb.ExportTraceServiceRequest {
		resourceCount := rapid.IntRange(0, 3).Draw(t, "n_resources")
		resources := make([]*tracepb.ResourceSpans, 0, resourceCount)
		for i := 0; i < resourceCount; i++ {
			resources = append(resources, otlpResourceSpans(t, fmt.Sprintf("resource_%d", i)))
		}
		return &tracecollectorpb.ExportTraceServiceRequest{ResourceSpans: resources}
	})
}

func otlpResourceSpans(t *rapid.T, label string) *tracepb.ResourceSpans {
	scopeCount := rapid.IntRange(0, 3).Draw(t, label+"_n_scopes")
	scopes := make([]*tracepb.ScopeSpans, 0, scopeCount)
	for i := 0; i < scopeCount; i++ {
		scopes = append(scopes, otlpScopeSpans(t, fmt.Sprintf("%s_scope_%d", label, i)))
	}
	return &tracepb.ResourceSpans{
		Resource: &resourcepb.Resource{
			Attributes: []*commonpb.KeyValue{
				stringKeyValue(string(semconv.ServiceNameKey), rapid.StringMatching(`^[a-z][a-z0-9-]{0,20}$`).Draw(t, label+"_service_name")),
			},
		},
		ScopeSpans: scopes,
		SchemaUrl:  rapid.SampledFrom([]string{"", "https://opentelemetry.io/schemas/1.27.0"}).Draw(t, label+"_schema_url"),
	}
}

func otlpScopeSpans(t *rapid.T, label string) *tracepb.ScopeSpans {
	spanCount := rapid.IntRange(0, 5).Draw(t, label+"_n_spans")
	spans := make([]*tracepb.Span, 0, spanCount)
	for i := 0; i < spanCount; i++ {
		spans = append(spans, otlpSpan(t, fmt.Sprintf("%s_span_%d", label, i)))
	}
	return &tracepb.ScopeSpans{
		Spans:     spans,
		SchemaUrl: rapid.SampledFrom([]string{"", "https://opentelemetry.io/schemas/1.27.0"}).Draw(t, label+"_schema_url"),
	}
}

func otlpSpan(t *rapid.T, label string) *tracepb.Span {
	start := uint64(rapid.Int64Range(1, 1_000_000_000).Draw(t, label+"_start"))
	duration := uint64(rapid.Int64Range(0, 1_000_000).Draw(t, label+"_duration"))
	attrs := make([]*commonpb.KeyValue, 0, 2)
	if rapid.Bool().Draw(t, label+"_attr_roll") {
		attrs = append(attrs, stringKeyValue("http.method", rapid.SampledFrom([]string{"GET", "POST", "PUT", "DELETE"}).Draw(t, label+"_method")))
	}
	return &tracepb.Span{
		TraceId:           fixedBytes(t, label+"_trace_id", 16),
		SpanId:            fixedBytes(t, label+"_span_id", 8),
		ParentSpanId:      rapid.SampledFrom([][]byte{nil, fixedBytes(t, label+"_parent_span_id", 8)}).Draw(t, label+"_parent_choice"),
		Name:              rapid.StringMatching(`^[A-Za-z][A-Za-z0-9_.-]{0,40}$`).Draw(t, label+"_name"),
		Kind:              tracepb.Span_SPAN_KIND_SERVER,
		StartTimeUnixNano: start,
		EndTimeUnixNano:   start + duration,
		Attributes:        attrs,
		Status: &tracepb.Status{
			Code: tracepb.Status_STATUS_CODE_UNSET,
		},
	}
}

func fixedBytes(t *rapid.T, label string, n int) []byte {
	bytes := rapid.SliceOfN(rapid.Byte(), n, n).Draw(t, label)
	allZero := true
	for _, b := range bytes {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		bytes[0] = 1
	}
	return bytes
}

func stringKeyValue(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key: key,
		Value: &commonpb.AnyValue{
			Value: &commonpb.AnyValue_StringValue{StringValue: value},
		},
	}
}
