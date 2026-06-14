// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"math"
	"strings"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestParse_Metrics_FieldsResolved(t *testing.T) {
	t.Parallel()

	const src = `
services:
  checkout:
    kind: application
    operations:
      - name: place_order
        metrics:
          - name: orders.settlement.amount.total
            type: counter
            unit: "{usd}"
            baseline: 80
            condition: on_success
            attributes:
              currency: usd
journeys:
  checkout:
    steps:
      - service: checkout
        operation: place_order
`
	s := mustParse(t, src)
	op := s.Services["checkout"].Operations["place_order"]
	if len(op.Metrics) != 1 {
		t.Fatalf("len(Metrics) = %d, want 1", len(op.Metrics))
	}
	m := op.Metrics[0]
	if m.Name != "orders.settlement.amount.total" {
		t.Fatalf("Name = %q", m.Name)
	}
	if m.Type != topology.MetricCounter {
		t.Fatalf("Type = %v, want counter", m.Type)
	}
	if m.Unit != "{usd}" {
		t.Fatalf("Unit = %q", m.Unit)
	}
	if m.Baseline != 80 {
		t.Fatalf("Baseline = %g, want 80", m.Baseline)
	}
	if m.Condition != topology.ConditionOnSuccess {
		t.Fatalf("Condition = %v, want on_success", m.Condition)
	}
	if m.Attributes["currency"] != "usd" {
		t.Fatalf("Attributes = %#v", m.Attributes)
	}
}

func TestParse_Metrics_WhenFaultResolved(t *testing.T) {
	t.Parallel()

	const src = `
services:
  shipping:
    kind: application
    operations:
      - name: quote_shipping
        metrics:
          - name: shipping.quote.backlog
            type: gauge
            baseline: 5
            when_fault:
              kind: latency_inflation
              delta: 40
journeys:
  ship:
    steps:
      - service: shipping
        operation: quote_shipping
`
	s := mustParse(t, src)
	m := s.Services["shipping"].Operations["quote_shipping"].Metrics[0]
	if m.WhenFault == nil {
		t.Fatal("WhenFault is nil")
	}
	if m.WhenFault.Kind != topology.FaultLatencyInflation {
		t.Fatalf("WhenFault.Kind = %v", m.WhenFault.Kind)
	}
	if m.WhenFault.Delta != 40 {
		t.Fatalf("WhenFault.Delta = %g, want 40", m.WhenFault.Delta)
	}
	if m.WhenFault.HasValue {
		t.Fatal("WhenFault.HasValue = true, want false")
	}
}

func TestParse_Metrics_WhenFaultValueOverride(t *testing.T) {
	t.Parallel()

	const src = `
services:
  api:
    kind: application
    operations:
      - name: ping
        metrics:
          - name: custom.load
            type: gauge
            when_fault:
              kind: crash
              value: 0
journeys:
  home:
    steps:
      - service: api
        operation: ping
`
	s := mustParse(t, src)
	m := s.Services["api"].Operations["ping"].Metrics[0]
	if m.WhenFault == nil || !m.WhenFault.HasValue {
		t.Fatalf("WhenFault = %#v, want HasValue true", m.WhenFault)
	}
	if m.WhenFault.Value != 0 {
		t.Fatalf("WhenFault.Value = %g, want 0", m.WhenFault.Value)
	}
}

func TestParse_Metrics_DefaultsApplied(t *testing.T) {
	t.Parallel()

	const src = `
services:
  api:
    kind: application
    operations:
      - name: ping
        metrics:
          - name: ping.count
            type: histogram
journeys:
  home:
    steps:
      - service: api
        operation: ping
`
	s := mustParse(t, src)
	m := s.Services["api"].Operations["ping"].Metrics[0]
	if m.Condition != topology.ConditionAlways {
		t.Fatalf("Condition = %v, want always", m.Condition)
	}
	if m.Baseline != 0 {
		t.Fatalf("Baseline = %g, want 0", m.Baseline)
	}
}

func TestValidate_Metrics_InvalidFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rule string
		src  string
	}{
		{
			name: "empty name",
			rule: "D-METRIC",
			src: `
services:
  api:
    kind: application
    operations:
      - name: ping
        metrics:
          - name: ""
            type: counter
journeys:
  home:
    steps:
      - service: api
        operation: ping
`,
		},
		{
			name: "invalid type",
			rule: "D-ENUM",
			src: `
services:
  api:
    kind: application
    operations:
      - name: ping
        metrics:
          - name: ping.bad
            type: summary
journeys:
  home:
    steps:
      - service: api
        operation: ping
`,
		},
		{
			name: "invalid condition",
			rule: "D-ENUM",
			src: `
services:
  api:
    kind: application
    operations:
      - name: ping
        metrics:
          - name: ping.bad
            type: counter
            condition: sometimes
journeys:
  home:
    steps:
      - service: api
        operation: ping
`,
		},
		{
			name: "invalid when_fault kind",
			rule: "D-ENUM",
			src: `
services:
  api:
    kind: application
    operations:
      - name: ping
        metrics:
          - name: ping.bad
            type: counter
            when_fault:
              kind: brownout
journeys:
  home:
    steps:
      - service: api
        operation: ping
`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := topology.Parse(strings.NewReader(tt.src))
			if err == nil {
				t.Fatalf("Parse() error = nil, want %s", tt.rule)
			}
			if !strings.Contains(err.Error(), tt.rule) {
				t.Fatalf("Parse() error = %v, want rule %s", err, tt.rule)
			}
		})
	}
}

func TestValidate_Metrics_NonFiniteBaseline(t *testing.T) {
	t.Parallel()

	schema := mustParse(t, `
services:
  api:
    kind: application
    operations:
      - name: ping
        metrics:
          - name: ping.count
            type: counter
journeys:
  home:
    steps:
      - service: api
        operation: ping
`)
	op := schema.Services["api"].Operations["ping"]
	op.Metrics[0].Baseline = math.NaN()
	if err := topology.Validate(schema); err == nil {
		t.Fatal("Validate() error = nil, want D-METRIC")
	} else if !strings.Contains(err.Error(), "D-METRIC") {
		t.Fatalf("Validate() error = %v, want D-METRIC", err)
	}
}
