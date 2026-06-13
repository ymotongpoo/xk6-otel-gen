// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"errors"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestBuildPlan_UnknownJourney_ReturnsError(t *testing.T) {
	t.Parallel()

	engine := NewEngine(newPlanTestSchema(), &topology.FaultOverlay{}, phase2Synth{})
	_, err := engine.BuildPlan("missing")
	var planErr *PlanError
	if !errors.As(err, &planErr) {
		t.Fatalf("BuildPlan() error = %T, want *PlanError", err)
	}
	if planErr.Kind != "unknown_journey" {
		t.Fatalf("PlanError.Kind = %q, want unknown_journey", planErr.Kind)
	}
}

func TestBuildPlan_EmptyJourney_ReturnsError(t *testing.T) {
	t.Parallel()

	schema := newPlanTestSchema()
	schema.Journeys["empty"] = &topology.Journey{Name: "empty"}
	impl := &engineImpl{schema: schema}

	_, err := impl.buildPlan("empty")
	var planErr *PlanError
	if !errors.As(err, &planErr) {
		t.Fatalf("buildPlan() error = %T, want *PlanError", err)
	}
	if planErr.Kind != "empty_journey" {
		t.Fatalf("PlanError.Kind = %q, want empty_journey", planErr.Kind)
	}
}

func TestBuildPlan_SingleStep_HappyPath(t *testing.T) {
	t.Parallel()

	engine := NewEngine(newPlanTestSchema(), &topology.FaultOverlay{}, phase2Synth{})
	plan, err := engine.BuildPlan("checkout")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	if plan.JourneyName != "checkout" {
		t.Fatalf("JourneyName = %q, want checkout", plan.JourneyName)
	}
	if plan.Root.Service.Name != "api" {
		t.Fatalf("Root.Service.Name = %q, want api", plan.Root.Service.Name)
	}
	if plan.Root.Operation != "GET /checkout" {
		t.Fatalf("Root.Operation = %q, want GET /checkout", plan.Root.Operation)
	}
}

func TestBuildPlan_ParallelSteps(t *testing.T) {
	t.Parallel()

	schema := newPlanTestSchema()
	api := schema.Services["api"]
	payments := schema.Services["payments"]
	schema.Journeys["parallel"] = &topology.Journey{
		Name: "parallel",
		Steps: []*topology.Step{{
			Parallel: []*topology.Step{
				{Op: api.Operations["GET /checkout"]},
				{Op: payments.Operations["POST /charge"]},
			},
		}},
	}

	engine := NewEngine(schema, &topology.FaultOverlay{}, phase2Synth{})
	plan, err := engine.BuildPlan("parallel")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if len(plan.Root.Parallel) != 2 {
		t.Fatalf("len(Root.Parallel) = %d, want 2", len(plan.Root.Parallel))
	}
	if plan.Root.Service != nil || plan.Root.Operation != "" {
		t.Fatalf("parallel root = service %v operation %q, want virtual node", plan.Root.Service, plan.Root.Operation)
	}
}

func TestBuildPlan_NestedCalls(t *testing.T) {
	t.Parallel()

	engine := NewEngine(newPlanTestSchema(), &topology.FaultOverlay{}, phase2Synth{})
	plan, err := engine.BuildPlan("checkout")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	if len(plan.Root.Children) != 1 {
		t.Fatalf("len(root.Children) = %d, want 1", len(plan.Root.Children))
	}
	payment := plan.Root.Children[0]
	if payment.Service.Name != "payments" || payment.Operation != "POST /charge" {
		t.Fatalf("payment node = %s.%s, want payments.POST /charge", payment.Service.Name, payment.Operation)
	}
	if len(payment.Children) != 1 {
		t.Fatalf("len(payment.Children) = %d, want 1", len(payment.Children))
	}
	db := payment.Children[0]
	if db.Service.Name != "db" || db.Operation != "INSERT payment" {
		t.Fatalf("db node = %s.%s, want db.INSERT payment", db.Service.Name, db.Operation)
	}
}

func newPlanTestSchema() *topology.Schema {
	api := &topology.Service{
		Name:       "api",
		Kind:       topology.KindApplication,
		Replicas:   2,
		Operations: make(map[string]*topology.Operation),
	}
	payments := &topology.Service{
		Name:       "payments",
		Kind:       topology.KindApplication,
		Replicas:   2,
		Operations: make(map[string]*topology.Operation),
	}
	db := &topology.Service{
		Name:       "db",
		Kind:       topology.KindDatabase,
		Replicas:   1,
		Operations: make(map[string]*topology.Operation),
	}
	checkout := &topology.Operation{Name: "GET /checkout", Service: api}
	charge := &topology.Operation{Name: "POST /charge", Service: payments}
	insert := &topology.Operation{Name: "INSERT payment", Service: db}
	api.Operations[checkout.Name] = checkout
	payments.Operations[charge.Name] = charge
	db.Operations[insert.Name] = insert

	checkoutToCharge := &topology.Edge{
		From:     checkout,
		To:       charge,
		Protocol: topology.ProtocolHTTP,
		Latency:  topology.LatencyDist{Distribution: "fixed"},
	}
	chargeToDB := &topology.Edge{
		From:     charge,
		To:       insert,
		Protocol: topology.ProtocolHTTP,
		Latency:  topology.LatencyDist{Distribution: "fixed"},
	}
	checkout.Calls = []*topology.CallNode{{Edge: checkoutToCharge}}
	charge.Calls = []*topology.CallNode{{Edge: chargeToDB}}

	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{
			api.Name:      api,
			payments.Name: payments,
			db.Name:       db,
		},
		Journeys: map[string]*topology.Journey{
			"checkout": {
				Name:  "checkout",
				Steps: []*topology.Step{{Op: checkout}},
			},
		},
	}
}
