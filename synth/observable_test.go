// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"context"
	"testing"

	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestUpdateState_AddsToObservableState(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, _ := newTestProviders(t)
	state := NewObservableState()
	syn := NewDefaultWithObservableState(singleProviderFactory{tp: tp, lp: lp}, mp, nil, state)
	svc := makeSpanService("checkout", topology.KindApplication)

	syn.UpdateState(context.Background(), StateUpdateInput{
		Service:     svc,
		Operation:   "place_order",
		InstanceIdx: 0,
		Key:         "orders.settlement",
		Delta:       80,
	})
	if got := state.Load("orders.settlement"); got != 80 {
		t.Fatalf("state.Load() = %g, want 80", got)
	}
}

func TestRegisterServiceMetrics_ObservableGaugeAccumulator(t *testing.T) {
	t.Parallel()

	_, mp, _, _, reader, _ := newTestProviders(t)
	svc := makeSpanService("kafka", topology.KindQueue)
	svc.Metrics = []topology.ObservableMetricSpec{{
		Name: "kafka.consumer.lag",
		Type: topology.MetricObservableGauge,
		Unit: "{message}",
		Attributes: map[string]any{
			"topic": "orders",
		},
		Source: &topology.MetricSourceSpec{
			Accumulator: "kafka.produced",
			Minus:       "kafka.consumed",
		},
	}}
	schema := observableMetricSchema(svc)
	state := NewObservableState()
	state.Add("kafka.produced", 10)
	state.Add("kafka.consumed", 4)

	reg, err := RegisterServiceMetrics(mp, schema, schema.ApplyFaults(), state)
	if err != nil {
		t.Fatalf("RegisterServiceMetrics() error = %v", err)
	}
	t.Cleanup(func() { _ = reg.Unregister() })

	point := gaugePoint(t, reader, "kafka.consumer.lag")
	if point.Value != 6 {
		t.Fatalf("Value = %g, want 6", point.Value)
	}
	attrs := resourceAttrs(point.Attributes.ToSlice())
	requireAttr(t, attrs, semconv.ServiceNameKey, "kafka")
	requireAttr(t, attrs, "topic", "orders")
}

func TestRegisterServiceMetrics_ObservableCounterAccumulator(t *testing.T) {
	t.Parallel()

	_, mp, _, _, reader, _ := newTestProviders(t)
	svc := makeSpanService("checkout", topology.KindApplication)
	svc.Metrics = []topology.ObservableMetricSpec{{
		Name: "orders.settlement.amount.total",
		Type: topology.MetricObservableCounter,
		Unit: "{usd}",
		Source: &topology.MetricSourceSpec{
			Accumulator: "orders.settlement",
		},
	}}
	schema := observableMetricSchema(svc)
	state := NewObservableState()
	state.Add("orders.settlement", 80)

	reg, err := RegisterServiceMetrics(mp, schema, schema.ApplyFaults(), state)
	if err != nil {
		t.Fatalf("RegisterServiceMetrics() error = %v", err)
	}
	t.Cleanup(func() { _ = reg.Unregister() })

	point := sumPoint(t, reader, "orders.settlement.amount.total")
	if point.Value != 80 {
		t.Fatalf("Value = %g, want 80", point.Value)
	}
}

func TestRegisterServiceMetrics_BaselineDeterministicFault(t *testing.T) {
	t.Parallel()

	_, mp, _, _, reader, _ := newTestProviders(t)
	svc := makeSpanService("shipping", topology.KindApplication)
	svc.Metrics = []topology.ObservableMetricSpec{{
		Name:     "shipping.quote.backlog",
		Type:     topology.MetricObservableGauge,
		Baseline: 5,
		WhenFault: &topology.MetricFaultLink{
			Kind:  topology.FaultLatencyInflation,
			Delta: 40,
		},
	}}
	schema := observableMetricSchema(svc)
	schema.Faults = []topology.FaultSpec{{
		Target: topology.FaultTarget{Kind: topology.TargetNode, Service: svc},
		Kind:   topology.FaultLatencyInflation,
		Severity: topology.SeverityParams{
			Probability: 1,
		},
	}}

	reg, err := RegisterServiceMetrics(mp, schema, schema.ApplyFaults(), NewObservableState())
	if err != nil {
		t.Fatalf("RegisterServiceMetrics() error = %v", err)
	}
	t.Cleanup(func() { _ = reg.Unregister() })

	point := gaugePoint(t, reader, "shipping.quote.backlog")
	if point.Value != 45 {
		t.Fatalf("Value = %g, want 45", point.Value)
	}
}

func TestRegisterServiceMetrics_UnregisterIdempotent(t *testing.T) {
	t.Parallel()

	_, mp, _, _, _, _ := newTestProviders(t)
	svc := makeSpanService("api", topology.KindApplication)
	svc.Metrics = []topology.ObservableMetricSpec{{
		Name:     "api.load",
		Type:     topology.MetricObservableGauge,
		Baseline: 1,
	}}
	schema := observableMetricSchema(svc)
	reg, err := RegisterServiceMetrics(mp, schema, schema.ApplyFaults(), NewObservableState())
	if err != nil {
		t.Fatalf("RegisterServiceMetrics() error = %v", err)
	}
	if err := reg.Unregister(); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}
	if err := reg.Unregister(); err != nil {
		t.Fatalf("second Unregister() error = %v", err)
	}
}

func observableMetricSchema(svc *topology.Service) *topology.Schema {
	return &topology.Schema{
		Namespace: topology.DefaultNamespace,
		Services: map[topology.ServiceID]*topology.Service{
			svc.Name: svc,
		},
		Journeys: map[string]*topology.Journey{},
	}
}
