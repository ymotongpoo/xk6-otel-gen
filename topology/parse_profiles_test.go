// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestParse_Profile_FieldsResolved(t *testing.T) {
	t.Parallel()

	const src = `
services:
  shipping:
    kind: application
    operations:
      - name: quote_shipping
        profile:
          enabled: true
          sample_rate: 100
          baseline:
            - frames: ["handleQuoteShipping", "calcBaseRate"]
              weight: 120
          incident:
            - frames: ["handleQuoteShipping", "calcBaseRate", "geoLookup"]
              weight: 900
          when_fault:
            kind: latency_inflation
journeys:
  ship:
    steps:
      - service: shipping
        operation: quote_shipping
`
	s := mustParse(t, src)
	prof := s.Services["shipping"].Operations["quote_shipping"].Profile
	if prof == nil || !prof.Enabled {
		t.Fatalf("Profile = %+v, want enabled", prof)
	}
	if prof.SampleRate != 100 {
		t.Fatalf("SampleRate = %d, want 100", prof.SampleRate)
	}
	if len(prof.Baseline) != 1 || prof.Baseline[0].Weight != 120 {
		t.Fatalf("Baseline = %+v", prof.Baseline)
	}
	if len(prof.Incident) != 1 || prof.Incident[0].Frames[2] != "geoLookup" {
		t.Fatalf("Incident = %+v", prof.Incident)
	}
	if prof.WhenFault == nil || prof.WhenFault.Kind != topology.FaultLatencyInflation {
		t.Fatalf("WhenFault = %+v", prof.WhenFault)
	}
}

func TestValidate_Profile_EnabledRequiresBaseline(t *testing.T) {
	t.Parallel()

	const src = `
services:
  api:
    kind: application
    operations:
      - name: GET /
        profile:
          enabled: true
journeys:
  j:
    steps:
      - service: api
        operation: GET /
`
	_, err := topology.Parse(bytes.NewReader([]byte(src)))
	if err == nil || !strings.Contains(err.Error(), "D-PROFILE") {
		t.Fatalf("Parse() error = %v, want D-PROFILE baseline error", err)
	}
}

func TestValidate_Profile_WhenFaultRequiresIncident(t *testing.T) {
	t.Parallel()

	const src = `
services:
  api:
    kind: application
    operations:
      - name: GET /
        profile:
          enabled: true
          baseline:
            - frames: ["root", "leaf"]
              weight: 1
          when_fault:
            kind: crash
journeys:
  j:
    steps:
      - service: api
        operation: GET /
`
	_, err := topology.Parse(bytes.NewReader([]byte(src)))
	if err == nil || !strings.Contains(err.Error(), "incident stacks required") {
		t.Fatalf("Parse() error = %v, want incident required", err)
	}
}

func TestValidate_Profile_InvalidSampleRate(t *testing.T) {
	t.Parallel()

	const src = `
services:
  api:
    kind: application
    operations:
      - name: GET /
        profile:
          enabled: true
          sample_rate: 0
          baseline:
            - frames: ["root", "leaf"]
              weight: 1
journeys:
  j:
    steps:
      - service: api
        operation: GET /
`
	_, err := topology.Parse(bytes.NewReader([]byte(src)))
	if err == nil || !strings.Contains(err.Error(), "sample_rate") {
		t.Fatalf("Parse() error = %v, want sample_rate error", err)
	}
}
