// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestExecute_Profile_EmitsBaselineAndIncident(t *testing.T) {
	t.Parallel()

	schema := newProfileTestSchema()
	engine, plan, mock := newExecutablePlanWithFaults(t, schema, faultOnOperation(
		schema.Services["shipping"].Operations["quote_shipping"],
		topology.FaultLatencyInflation,
		1, 0, 50*time.Millisecond,
	))
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	profiles := mock.snapshotProfiles()
	if len(profiles) != 1 {
		t.Fatalf("profiles = %d, want 1", len(profiles))
	}
	in := profiles[0].Input
	if in.ProfileID == "" {
		t.Fatal("ProfileID empty")
	}
	if len(in.Stacks) != 1 || in.Stacks[0].Weight != 900 || in.Stacks[0].Frames[2] != "geoLookup" {
		t.Fatalf("Stacks = %+v, want incident variant", in.Stacks)
	}
	if in.StartTime.After(in.EndTime) {
		t.Fatalf("window = [%s,%s]", in.StartTime, in.EndTime)
	}
}

func TestExecute_Profile_FaultInactiveUsesBaseline(t *testing.T) {
	t.Parallel()

	schema := newProfileTestSchema()
	schema.Faults = nil
	engine, plan, mock := newExecutablePlan(t, schema)
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	profiles := mock.snapshotProfiles()
	if len(profiles) != 1 {
		t.Fatalf("profiles = %d, want 1", len(profiles))
	}
	if profiles[0].Input.Stacks[0].Weight != 120 {
		t.Fatalf("Stacks = %+v, want baseline", profiles[0].Input.Stacks)
	}
}

func TestExecute_ProfileDisabled_NoEmit(t *testing.T) {
	t.Parallel()

	schema := newPlanTestSchema()
	engine, plan, mock := newExecutablePlan(t, schema)
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(mock.snapshotProfiles()) != 0 {
		t.Fatalf("profiles = %d, want 0", len(mock.snapshotProfiles()))
	}
}

func newProfileTestSchema() *topology.Schema {
	shipping := &topology.Service{
		Name:       "shipping",
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: make(map[string]*topology.Operation),
	}
	quote := &topology.Service{
		Name:       "quote",
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: make(map[string]*topology.Operation),
	}
	op := &topology.Operation{
		Name:    "quote_shipping",
		Service: shipping,
		Profile: &topology.ProfileSpec{
			Enabled:    true,
			SampleRate: 100,
			Baseline: []topology.StackSample{
				{Frames: []string{"handleQuoteShipping", "calcBaseRate"}, Weight: 120},
			},
			Incident: []topology.StackSample{
				{Frames: []string{"handleQuoteShipping", "calcBaseRate", "geoLookup"}, Weight: 900},
			},
			WhenFault: &topology.ProfileFaultLink{Kind: topology.FaultLatencyInflation},
		},
	}
	getQuote := &topology.Operation{Name: "get_quote", Service: quote}
	shipping.Operations[op.Name] = op
	quote.Operations[getQuote.Name] = getQuote
	op.Calls = []*topology.CallNode{{
		Edge: &topology.Edge{
			From:     op,
			To:       getQuote,
			Protocol: topology.ProtocolGRPC,
			Latency:  topology.LatencyDist{Distribution: "fixed", P50: 5 * time.Millisecond},
		},
	}}
	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{
			shipping.Name: shipping,
			quote.Name:    quote,
		},
		Journeys: map[string]*topology.Journey{
			"ship": {Name: "ship", Steps: []*topology.Step{{Op: op}}, Weight: 1},
		},
	}
}
