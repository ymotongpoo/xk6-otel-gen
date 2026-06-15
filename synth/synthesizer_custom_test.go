// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestRecordCustom_Counter(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, reader, _ := newTestProviders(t)
	syn := NewDefault(singleProviderFactory{tp: tp, lp: lp}, mp, nil)
	svc := makeSpanService("checkout", topology.KindApplication)
	syn.RecordCustom(context.Background(), CustomMetricInput{
		Service:     svc,
		Operation:   "place_order",
		InstanceIdx: 0,
		Name:        "orders.settlement.amount.total",
		Type:        topology.MetricCounter,
		Unit:        "{usd}",
		Value:       80,
		Attributes:  map[string]any{"currency": "usd"},
	})

	point := sumPoint(t, reader, "orders.settlement.amount.total")
	if point.Value != 80 {
		t.Fatalf("Value = %g, want 80", point.Value)
	}
	attrs := resourceAttrs(point.Attributes.ToSlice())
	requireAttr(t, attrs, semconv.ServiceNameKey, "checkout")
	requireAttr(t, attrs, "currency", "usd")
}

func TestRecordCustom_Gauge(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, reader, _ := newTestProviders(t)
	syn := NewDefault(singleProviderFactory{tp: tp, lp: lp}, mp, nil)
	svc := makeSpanService("shipping", topology.KindApplication)
	syn.RecordCustom(context.Background(), CustomMetricInput{
		Service:     svc,
		Operation:   "quote_shipping",
		InstanceIdx: 0,
		Name:        "shipping.quote.backlog",
		Type:        topology.MetricGauge,
		Unit:        "{request}",
		Value:       45,
	})

	point := gaugePoint(t, reader, "shipping.quote.backlog")
	if point.Value != 45 {
		t.Fatalf("Value = %g, want 45", point.Value)
	}
	attrs := resourceAttrs(point.Attributes.ToSlice())
	requireAttr(t, attrs, semconv.ServiceNameKey, "shipping")
}

func TestRecordCustom_Histogram(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, reader, _ := newTestProviders(t)
	syn := NewDefault(singleProviderFactory{tp: tp, lp: lp}, mp, nil)
	svc := makeSpanService("api", topology.KindApplication)
	syn.RecordCustom(context.Background(), CustomMetricInput{
		Service:     svc,
		Operation:   "ping",
		InstanceIdx: 0,
		Name:        "api.latency.custom",
		Type:        topology.MetricHistogram,
		Unit:        "s",
		Value:       0.25,
	})

	point := histogramPoint(t, reader, "api.latency.custom")
	if point.Count != 1 || point.Sum != 0.25 {
		t.Fatalf("histogram = count %d sum %g, want count 1 sum 0.25", point.Count, point.Sum)
	}
}

func TestRecordCustom_NilService_NoOp(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, reader, _ := newTestProviders(t)
	syn := NewDefault(singleProviderFactory{tp: tp, lp: lp}, mp, nil)
	syn.RecordCustom(context.Background(), CustomMetricInput{
		Name:  "ignored",
		Type:  topology.MetricCounter,
		Value: 1,
	})

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == "ignored" {
				t.Fatalf("unexpected metric recorded: %#v", metric)
			}
		}
	}
}

func sumPoint(t *testing.T, reader metricReader, name string) metricdata.DataPoint[float64] {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			sum, ok := metric.Data.(metricdata.Sum[float64])
			if !ok {
				t.Fatalf("%s data = %T, want metricdata.Sum[float64]", name, metric.Data)
			}
			if len(sum.DataPoints) != 1 {
				t.Fatalf("%s datapoints = %d, want 1", name, len(sum.DataPoints))
			}
			return sum.DataPoints[0]
		}
	}
	t.Fatalf("sum metric %q not found", name)
	return metricdata.DataPoint[float64]{}
}

func gaugePoint(t *testing.T, reader metricReader, name string) metricdata.DataPoint[float64] {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			gauge, ok := metric.Data.(metricdata.Gauge[float64])
			if !ok {
				t.Fatalf("%s data = %T, want metricdata.Gauge[float64]", name, metric.Data)
			}
			if len(gauge.DataPoints) != 1 {
				t.Fatalf("%s datapoints = %d, want 1", name, len(gauge.DataPoints))
			}
			return gauge.DataPoints[0]
		}
	}
	t.Fatalf("gauge metric %q not found", name)
	return metricdata.DataPoint[float64]{}
}
