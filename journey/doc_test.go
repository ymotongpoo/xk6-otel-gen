// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey_test

import (
	"context"
	"fmt"

	"github.com/ymotongpoo/xk6-otel-gen/journey"
	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func ExampleNewEngine() {
	schema := exampleSchema()
	eng := journey.NewEngine(schema, schema.ApplyFaults(), exampleSynth{})

	fmt.Println(eng.ListJourneys())

	// Output: [checkout]
}

func ExampleEngine_BuildPlan() {
	schema := exampleSchema()
	eng := journey.NewEngine(schema, schema.ApplyFaults(), exampleSynth{})

	plan, _ := eng.BuildPlan("checkout")
	fmt.Println(plan.JourneyName, plan.Root.Operation)

	// Output: checkout GET /checkout
}

func ExampleEngine_Execute() {
	schema := exampleSchema()
	eng := journey.NewEngine(schema, schema.ApplyFaults(), exampleSynth{})
	plan, _ := eng.BuildPlan("checkout")

	err := eng.Execute(context.Background(), plan)
	fmt.Println(err == nil)

	// Output: true
}

type exampleSynth struct{}

func (exampleSynth) BeginSpan(ctx context.Context, _ synth.SpanInput) (context.Context, synth.FinishSpanFunc) {
	return ctx, func(synth.Outcome) {}
}

func (exampleSynth) RecordMetric(context.Context, synth.MetricInput) {}

func (exampleSynth) EmitLog(context.Context, synth.LogInput) {}

func (exampleSynth) RecordCustom(context.Context, synth.CustomMetricInput) {}

func (exampleSynth) EmitProfile(context.Context, synth.ProfileInput) {}

func exampleSchema() *topology.Schema {
	svc := &topology.Service{
		Name:       "frontend",
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: make(map[string]*topology.Operation),
	}
	op := &topology.Operation{Name: "GET /checkout", Service: svc}
	svc.Operations[op.Name] = op
	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{svc.Name: svc},
		Journeys: map[string]*topology.Journey{
			"checkout": {Name: "checkout", Steps: []*topology.Step{{Op: op}}, Weight: 1},
		},
	}
}
