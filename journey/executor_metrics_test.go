// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestExecute_CustomMetrics_ConditionGating(t *testing.T) {
	t.Parallel()

	schema := newMetricsSchema()
	engine, plan, mock := newExecutablePlan(t, schema)
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	calls := customMetricsForService(mock.snapshotCustomMetrics(), "api")
	if len(calls) != 2 {
		t.Fatalf("custom metrics = %d, want 2 (always + on_success)", len(calls))
	}
	got := map[string]float64{}
	for _, call := range calls {
		got[call.Input.Name] = call.Input.Value
	}
	if got["checkout.always"] != 1 {
		t.Fatalf("always value = %g, want 1", got["checkout.always"])
	}
	if got["checkout.on_success"] != 10 {
		t.Fatalf("on_success value = %g, want 10", got["checkout.on_success"])
	}
}

func TestExecute_CustomMetrics_OnErrorOnlyOnFailure(t *testing.T) {
	t.Parallel()

	schema := newMetricsSchema()
	op := schema.Services["api"].Operations["GET /checkout"]
	engine, plan, mock := newExecutablePlanWithFaults(
		t,
		schema,
		faultOnOperation(op, topology.FaultErrorRateOverride, 1, 1, 0),
	)
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	calls := customMetricsForService(mock.snapshotCustomMetrics(), "api")
	got := map[string]float64{}
	for _, call := range calls {
		got[call.Input.Name] = call.Input.Value
	}
	if got["checkout.always"] != 1 {
		t.Fatalf("always value = %g, want 1", got["checkout.always"])
	}
	if _, ok := got["checkout.on_success"]; ok {
		t.Fatalf("on_success emitted on failure: %#v", got)
	}
	if got["checkout.on_error"] != 99 {
		t.Fatalf("on_error value = %g, want 99", got["checkout.on_error"])
	}
}

func TestExecute_CustomMetrics_FaultLinkedDelta(t *testing.T) {
	t.Parallel()

	schema := newFaultLinkedMetricsSchema()
	op := schema.Services["shipping"].Operations["quote_shipping"]
	engine, plan, mock := newExecutablePlanWithFaults(
		t,
		schema,
		faultOnOperation(op, topology.FaultLatencyInflation, 1, 0, 50*time.Millisecond),
	)
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	calls := customMetricsForService(mock.snapshotCustomMetrics(), "shipping")
	if len(calls) != 1 {
		t.Fatalf("custom metrics = %d, want 1", len(calls))
	}
	call := calls[0].Input
	if call.Name != "shipping.quote.backlog" {
		t.Fatalf("Name = %q", call.Name)
	}
	if call.Type != topology.MetricGauge {
		t.Fatalf("Type = %v, want gauge", call.Type)
	}
	if call.Value != 45 {
		t.Fatalf("Value = %g, want 45 (baseline 5 + delta 40)", call.Value)
	}
}

func TestExecute_CustomMetrics_FaultInactiveUsesBaseline(t *testing.T) {
	t.Parallel()

	schema := newFaultLinkedMetricsSchema()
	engine, plan, mock := newExecutablePlan(t, schema)
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	calls := customMetricsForService(mock.snapshotCustomMetrics(), "shipping")
	if len(calls) != 1 {
		t.Fatalf("custom metrics = %d, want 1", len(calls))
	}
	if calls[0].Input.Value != 5 {
		t.Fatalf("Value = %g, want baseline 5", calls[0].Input.Value)
	}
}

func newMetricsSchema() *topology.Schema {
	schema := newPlanTestSchema()
	checkout := schema.Services["api"].Operations["GET /checkout"]
	checkout.Metrics = []topology.MetricSpec{
		{Name: "checkout.always", Type: topology.MetricCounter, Baseline: 1, Condition: topology.ConditionAlways},
		{Name: "checkout.on_success", Type: topology.MetricCounter, Baseline: 10, Condition: topology.ConditionOnSuccess},
		{Name: "checkout.on_error", Type: topology.MetricGauge, Baseline: 99, Condition: topology.ConditionOnError},
	}
	return schema
}

func newFaultLinkedMetricsSchema() *topology.Schema {
	shipping := &topology.Service{
		Name:       "shipping",
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: make(map[string]*topology.Operation),
	}
	op := &topology.Operation{
		Name:    "quote_shipping",
		Service: shipping,
		Metrics: []topology.MetricSpec{
			{
				Name:      "shipping.quote.backlog",
				Type:      topology.MetricGauge,
				Baseline:  5,
				Condition: topology.ConditionAlways,
				WhenFault: &topology.MetricFaultLink{
					Kind:  topology.FaultLatencyInflation,
					Delta: 40,
				},
			},
		},
	}
	shipping.Operations[op.Name] = op
	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{shipping.Name: shipping},
		Journeys: map[string]*topology.Journey{
			"ship": {Name: "ship", Steps: []*topology.Step{{Op: op}}},
		},
	}
}

func customMetricsForService(calls []customMetricCall, serviceName topology.ServiceID) []customMetricCall {
	out := make([]customMetricCall, 0)
	for _, call := range calls {
		if call.Input.Service != nil && call.Input.Service.Name == serviceName {
			out = append(out, call)
		}
	}
	return out
}
