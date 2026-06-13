// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"testing"
	"time"

	"go.opentelemetry.io/otel/log"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestSpanInput_FieldAccess(t *testing.T) {
	t.Parallel()

	svc := &topology.Service{Name: "frontend", Replicas: 2}
	from := &topology.Operation{Name: "GET /checkout", Service: svc}
	toSvc := &topology.Service{Name: "checkout", Replicas: 1}
	to := &topology.Operation{Name: "POST /orders", Service: toSvc}
	edge := &topology.Edge{From: from, To: to, Protocol: topology.ProtocolHTTP}
	start := time.Unix(1_700_000_000, 42)

	in := SpanInput{
		Service:     svc,
		Edge:        edge,
		Operation:   from.Name,
		StartTime:   start,
		InstanceIdx: 1,
	}

	if in.Service != svc {
		t.Fatalf("Service = %p, want %p", in.Service, svc)
	}
	if in.Edge != edge {
		t.Fatalf("Edge = %p, want %p", in.Edge, edge)
	}
	if in.Operation != "GET /checkout" {
		t.Fatalf("Operation = %q", in.Operation)
	}
	if !in.StartTime.Equal(start) {
		t.Fatalf("StartTime = %s, want %s", in.StartTime, start)
	}
	if in.InstanceIdx != 1 {
		t.Fatalf("InstanceIdx = %d, want 1", in.InstanceIdx)
	}
}

func TestOutcome_FieldAccess(t *testing.T) {
	t.Parallel()

	end := time.Unix(1_700_000_001, 99)
	outcome := Outcome{
		Success:    false,
		StatusCode: 503,
		ErrorType:  "http.503",
		EndTime:    end,
	}
	logInput := LogInput{
		Service:    &topology.Service{Name: "checkout"},
		Severity:   log.SeverityError,
		Body:       "checkout failed",
		Attributes: map[string]any{"error.type": outcome.ErrorType},
	}

	if outcome.Success {
		t.Fatal("Success = true, want false")
	}
	if outcome.StatusCode != 503 {
		t.Fatalf("StatusCode = %d, want 503", outcome.StatusCode)
	}
	if outcome.ErrorType != "http.503" {
		t.Fatalf("ErrorType = %q, want http.503", outcome.ErrorType)
	}
	if !outcome.EndTime.Equal(end) {
		t.Fatalf("EndTime = %s, want %s", outcome.EndTime, end)
	}
	if logInput.Severity != log.SeverityError {
		t.Fatalf("Severity = %v, want Error", logInput.Severity)
	}
}
