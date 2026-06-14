// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"strings"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestParse_LogEvents_FieldsResolved(t *testing.T) {
	t.Parallel()

	const src = `
services:
  payment:
    kind: application
    operations:
      - name: authorize_card
        log_events:
          - name: provider_call.timeout
            condition: on_error
            severity: error
            body: "payment provider call timed out"
            attributes:
              provider: stripe
              retryable: true
journeys:
  checkout:
    steps:
      - service: payment
        operation: authorize_card
`
	s := mustParse(t, src)
	op := s.Services["payment"].Operations["authorize_card"]
	if len(op.LogEvents) != 1 {
		t.Fatalf("len(LogEvents) = %d, want 1", len(op.LogEvents))
	}
	ev := op.LogEvents[0]
	if ev.Name != "provider_call.timeout" {
		t.Fatalf("Name = %q, want provider_call.timeout", ev.Name)
	}
	if ev.Condition != topology.ConditionOnError {
		t.Fatalf("Condition = %v, want on_error", ev.Condition)
	}
	if ev.Severity != topology.SeverityError {
		t.Fatalf("Severity = %v, want error", ev.Severity)
	}
	if ev.Body != "payment provider call timed out" {
		t.Fatalf("Body = %q", ev.Body)
	}
	if ev.Attributes["provider"] != "stripe" || ev.Attributes["retryable"] != true {
		t.Fatalf("Attributes = %#v", ev.Attributes)
	}
}

func TestParse_LogEvents_DefaultsApplied(t *testing.T) {
	t.Parallel()

	const src = `
services:
  api:
    kind: application
    operations:
      - name: ping
        log_events:
          - name: ping.completed
journeys:
  home:
    steps:
      - service: api
        operation: ping
`
	s := mustParse(t, src)
	ev := s.Services["api"].Operations["ping"].LogEvents[0]
	if ev.Condition != topology.ConditionAlways {
		t.Fatalf("Condition = %v, want always", ev.Condition)
	}
	if ev.Severity != topology.SeverityInfo {
		t.Fatalf("Severity = %v, want info", ev.Severity)
	}
}

func TestParse_LogEvents_WarningAlias(t *testing.T) {
	t.Parallel()

	const src = `
services:
  api:
    kind: application
    operations:
      - name: ping
        log_events:
          - name: ping.warn
            severity: warning
journeys:
  home:
    steps:
      - service: api
        operation: ping
`
	s := mustParse(t, src)
	if s.Services["api"].Operations["ping"].LogEvents[0].Severity != topology.SeverityWarn {
		t.Fatalf("Severity = %v, want warn", s.Services["api"].Operations["ping"].LogEvents[0].Severity)
	}
}

func TestValidate_LogEvents_InvalidFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rule string
		src  string
	}{
		{
			name: "empty name",
			rule: "D-LOG",
			src: `
services:
  api:
    kind: application
    operations:
      - name: ping
        log_events:
          - name: ""
journeys:
  home:
    steps:
      - service: api
        operation: ping
`,
		},
		{
			name: "invalid severity",
			rule: "D-ENUM",
			src: `
services:
  api:
    kind: application
    operations:
      - name: ping
        log_events:
          - name: ping.bad
            severity: critical
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
        log_events:
          - name: ping.bad
            condition: sometimes
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
