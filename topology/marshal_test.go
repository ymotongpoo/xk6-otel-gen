// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"strings"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"gopkg.in/yaml.v3"
)

func TestMarshal_AlphabeticalOrder(t *testing.T) {
	t.Parallel()

	s := validManualSchema()
	s.Journeys["zeta"] = s.Journeys["home"]
	s.Journeys["zeta"].Name = "zeta"
	s.Journeys["alpha"] = &topology.Journey{Name: "alpha", Steps: []*topology.Step{{Op: s.Services["frontend"].Operations["GET /"]}}, Weight: 1}
	backend := s.Services["backend"]
	backend.Operations["Alpha"] = &topology.Operation{Name: "Alpha", Service: backend}

	yamlBytes, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	out := string(yamlBytes)
	assertOrdered(t, out, "backend:", "frontend:")
	assertOrdered(t, out, "name: Alpha", "name: Fetch")
	assertOrdered(t, out, "alpha:", "home:")
	assertOrdered(t, out, "home:", "zeta:")
}

func TestMarshal_PreservesSequenceOrder(t *testing.T) {
	t.Parallel()

	const src = `
services:
  api:
    kind: application
    operations:
      - name: Root
        calls:
          - to: { service: dep1, operation: Fetch }
            protocol: http
            on_failure:
              fallback:
                - to: { service: dep2, operation: Fetch }
                  protocol: http
                - to: { service: dep3, operation: Fetch }
                  protocol: http
          - to: { service: dep3, operation: Fetch }
            protocol: http
  dep1: { kind: application, operations: [ { name: Fetch } ] }
  dep2: { kind: application, operations: [ { name: Fetch } ] }
  dep3: { kind: application, operations: [ { name: Fetch } ] }
journeys:
  flow:
    steps:
      - service: api
        operation: Root
      - service: dep1
        operation: Fetch
`
	s := mustParse(t, src)
	yamlBytes, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	out := string(yamlBytes)
	assertOrdered(t, out, "service: dep1", "service: dep3")
	assertOrdered(t, out, "service: dep2", "service: dep3")
	steps := out[strings.Index(out, "steps:"):]
	assertOrdered(t, steps, "operation: Root", "operation: Fetch")
}

func TestMarshal_OmitsZeroValues(t *testing.T) {
	t.Parallel()

	s := mustParse(t, minimalYAML)
	yamlBytes, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	out := string(yamlBytes)
	for _, omitted := range []string{"replicas:", "language:", "framework:", "version:", "weight:", "faults:"} {
		if strings.Contains(out, omitted) {
			t.Fatalf("marshal output contains default field %q:\n%s", omitted, out)
		}
	}
}

func assertOrdered(t *testing.T, text, first, second string) {
	t.Helper()
	firstIndex := strings.Index(text, first)
	secondIndex := strings.Index(text, second)
	if firstIndex < 0 || secondIndex < 0 {
		t.Fatalf("missing ordered substrings %q or %q in:\n%s", first, second, text)
	}
	if firstIndex >= secondIndex {
		t.Fatalf("expected %q before %q in:\n%s", first, second, text)
	}
}
