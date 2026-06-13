// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestValidate_StructuralRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		rule   string
		mutate func(*topology.Schema)
	}{
		{
			name: "R-STR-1 service key mismatch",
			rule: "R-STR-1",
			mutate: func(s *topology.Schema) {
				s.Services["frontend"].Name = "renamed"
			},
		},
		{
			name: "R-STR-2 bad back pointer",
			rule: "R-STR-2",
			mutate: func(s *topology.Schema) {
				s.Services["frontend"].Operations["GET /"].Service = s.Services["backend"]
			},
		},
		{
			name: "R-STR-3 orphan edge target",
			rule: "R-STR-3",
			mutate: func(s *topology.Schema) {
				firstEdge(s).To = &topology.Operation{Name: "Missing", Service: &topology.Service{Name: "missing"}}
			},
		},
		{
			name: "R-STR-4 cycle",
			rule: "R-STR-4",
			mutate: func(s *topology.Schema) {
				edge := firstEdge(s)
				edge.To.Calls = []*topology.CallNode{{Edge: &topology.Edge{
					From:         edge.To,
					To:           edge.From,
					Protocol:     topology.ProtocolHTTP,
					Latency:      topology.LatencyDist{Distribution: "constant"},
					RetryBackoff: topology.BackoffExponential,
				}}}
			},
		},
		{
			name: "R-STR-5 stale journey step",
			rule: "R-STR-5",
			mutate: func(s *topology.Schema) {
				s.Journeys["home"].Steps[0].Op = &topology.Operation{Name: "Stale", Service: &topology.Service{Name: "stale"}}
			},
		},
		{
			name: "R-STR-6 stale fault target",
			rule: "R-STR-6",
			mutate: func(s *topology.Schema) {
				s.Faults = []topology.FaultSpec{{
					Target: topology.FaultTarget{
						Kind:      topology.TargetOperation,
						Operation: &topology.Operation{Name: "Stale", Service: &topology.Service{Name: "stale"}},
					},
					Kind: topology.FaultCrash,
				}}
			},
		},
		{
			name: "R-STR-7 bad call node variant",
			rule: "R-STR-7",
			mutate: func(s *topology.Schema) {
				node := s.Services["frontend"].Operations["GET /"].Calls[0]
				node.Parallel = []*topology.CallNode{{Edge: node.Edge}}
			},
		},
		{
			name: "R-STR-8 fallback owner mismatch",
			rule: "R-STR-8",
			mutate: func(s *topology.Schema) {
				edge := firstEdge(s)
				edge.OnFailure = &topology.RecoveryPolicy{
					Fallback: []*topology.Edge{{
						From:         edge.To,
						To:           edge.To,
						Protocol:     topology.ProtocolHTTP,
						Latency:      topology.LatencyDist{Distribution: "constant"},
						RetryBackoff: topology.BackoffExponential,
					}},
					OnExhausted: topology.ExhaustedPropagate,
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := validManualSchema()
			tt.mutate(s)
			err := topology.Validate(s)
			if err == nil {
				t.Fatalf("Validate() error = nil, want %s", tt.rule)
			}
			assertContains(t, err.Error(), tt.rule)
		})
	}
}

func TestValidate_DomainRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		rule   string
		mutate func(*topology.Schema)
	}{
		{"D-1 replicas", "D-1", func(s *topology.Schema) { s.Services["frontend"].Replicas = 0 }},
		{"D-2 error rate", "D-2", func(s *topology.Schema) { firstEdge(s).ErrorRate = 2 }},
		{"D-3 timeout", "D-3", func(s *topology.Schema) { firstEdge(s).Timeout = -time.Second }},
		{"D-4 retries", "D-4", func(s *topology.Schema) { firstEdge(s).Retries = -1 }},
		{"D-5 p95", "D-5", func(s *topology.Schema) { firstEdge(s).Latency.P95 = time.Millisecond }},
		{"D-6 p50", "D-6", func(s *topology.Schema) { firstEdge(s).Latency.P50 = -time.Millisecond }},
		{"D-7 distribution", "D-7", func(s *topology.Schema) { firstEdge(s).Latency.Distribution = "weibull" }},
		{"D-8 probability", "D-8", func(s *topology.Schema) {
			s.Faults = []topology.FaultSpec{{Target: topology.FaultTarget{Kind: topology.TargetNode, Service: s.Services["frontend"]}, Kind: topology.FaultCrash, Severity: topology.SeverityParams{Probability: 2}}}
		}},
		{"D-9 multiplier", "D-9", func(s *topology.Schema) {
			s.Faults = []topology.FaultSpec{{Target: topology.FaultTarget{Kind: topology.TargetNode, Service: s.Services["frontend"]}, Kind: topology.FaultLatencyInflation, Severity: topology.SeverityParams{Probability: 1}}}
		}},
		{"D-10 journey weight", "D-10", func(s *topology.Schema) { s.Journeys["home"].Weight = 0 }},
		{"D-11 journey steps", "D-11", func(s *topology.Schema) { s.Journeys["home"].Steps = nil }},
		{"D-12 operations", "D-12", func(s *topology.Schema) { s.Services["frontend"].Operations = map[string]*topology.Operation{} }},
		{"D-13 services", "D-13", func(s *topology.Schema) { s.Services = map[topology.ServiceID]*topology.Service{} }},
		{"D-14 journeys", "D-14", func(s *topology.Schema) { s.Journeys = map[string]*topology.Journey{} }},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := validManualSchema()
			tt.mutate(s)
			err := topology.Validate(s)
			if err == nil {
				t.Fatalf("Validate() error = nil, want %s", tt.rule)
			}
			if !strings.Contains(err.Error(), tt.rule) {
				t.Fatalf("Validate() error = %v, want rule %s", err, tt.rule)
			}
		})
	}
}

func TestValidate_OnValidSchemaReturnsNil(t *testing.T) {
	t.Parallel()

	if err := topology.Validate(validManualSchema()); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
