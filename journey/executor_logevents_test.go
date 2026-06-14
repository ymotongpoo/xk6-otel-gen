// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/log"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestExecute_LogEvents_ConditionGating(t *testing.T) {
	t.Parallel()

	schema := newLogEventsSchema()
	engine, plan, mock := newExecutablePlan(t, schema)
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	logs := logsForService(mock.snapshotLogs(), "api")
	genericCount := 0
	alwaysCount := 0
	successCount := 0
	errorCount := 0
	for _, call := range logs {
		if call.Input.EventName == "" {
			genericCount++
			continue
		}
		switch call.Input.EventName {
		case "checkout.always":
			alwaysCount++
		case "checkout.on_success":
			successCount++
		case "checkout.on_error":
			errorCount++
		default:
			t.Fatalf("unexpected EventName = %q", call.Input.EventName)
		}
	}
	if genericCount != 1 {
		t.Fatalf("generic logs = %d, want 1", genericCount)
	}
	if alwaysCount != 1 {
		t.Fatalf("always events = %d, want 1", alwaysCount)
	}
	if successCount != 1 {
		t.Fatalf("on_success events = %d, want 1", successCount)
	}
	if errorCount != 0 {
		t.Fatalf("on_error events = %d, want 0 on success path", errorCount)
	}
}

func TestExecute_LogEvents_OnErrorOnlyOnFailure(t *testing.T) {
	t.Parallel()

	schema := newLogEventsSchema()
	op := schema.Services["api"].Operations["GET /checkout"]
	engine, plan, mock := newExecutablePlanWithFaults(
		t,
		schema,
		faultOnOperation(op, topology.FaultErrorRateOverride, 1, 1, 0),
	)
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	logs := logsForService(mock.snapshotLogs(), "api")
	var gotAlways, gotSuccess, gotError int
	for _, call := range logs {
		switch call.Input.EventName {
		case "checkout.always":
			gotAlways++
		case "checkout.on_success":
			gotSuccess++
		case "checkout.on_error":
			gotError++
			if call.Input.Severity != log.SeverityError {
				t.Fatalf("on_error severity = %v, want error", call.Input.Severity)
			}
			if call.Input.Body != "checkout failed" {
				t.Fatalf("on_error body = %q", call.Input.Body)
			}
			if call.Input.Attributes["reason"] != "forced" {
				t.Fatalf("on_error attributes = %#v", call.Input.Attributes)
			}
		}
	}
	if gotAlways != 1 || gotSuccess != 0 || gotError != 1 {
		t.Fatalf("event counts always=%d success=%d error=%d, want 1/0/1", gotAlways, gotSuccess, gotError)
	}
}

func newLogEventsSchema() *topology.Schema {
	schema := newPlanTestSchema()
	checkout := schema.Services["api"].Operations["GET /checkout"]
	checkout.LogEvents = []topology.LogEventSpec{
		{Name: "checkout.always", Condition: topology.ConditionAlways, Severity: topology.SeverityInfo},
		{Name: "checkout.on_success", Condition: topology.ConditionOnSuccess, Severity: topology.SeverityInfo},
		{
			Name:       "checkout.on_error",
			Condition:  topology.ConditionOnError,
			Severity:   topology.SeverityError,
			Body:       "checkout failed",
			Attributes: map[string]any{"reason": "forced"},
		},
	}
	return schema
}

func logsForService(logs []logCall, serviceName topology.ServiceID) []logCall {
	out := make([]logCall, 0)
	for _, call := range logs {
		if call.Input.Service != nil && call.Input.Service.Name == serviceName {
			out = append(out, call)
		}
	}
	return out
}
