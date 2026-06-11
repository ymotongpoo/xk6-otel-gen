// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestParse_MinimalSchema(t *testing.T) {
	t.Parallel()

	s, err := topology.Parse(strings.NewReader(minimalYAML))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(s.Services) != 1 {
		t.Fatalf("services len = %d, want 1", len(s.Services))
	}
	if _, ok := s.FindServiceByName("frontend"); !ok {
		t.Fatal("frontend service not found")
	}
}

func TestParse_DefaultsApplied(t *testing.T) {
	t.Parallel()

	const src = `
services:
  frontend:
    kind: application
    operations:
      - name: GET /
        calls:
          - to: { service: backend, operation: Fetch }
            protocol: http
            latency: { p50: 10ms }
  backend:
    kind: application
    operations:
      - name: Fetch
journeys:
  home:
    steps:
      - service: frontend
        operation: GET /
`
	s, err := topology.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	frontend := s.Services["frontend"]
	if s.Namespace != topology.DefaultNamespace || frontend.Namespace != topology.DefaultNamespace {
		t.Fatalf("namespace defaults not applied: schema=%q frontend=%q", s.Namespace, frontend.Namespace)
	}
	if frontend.Replicas != 1 {
		t.Fatalf("Replicas = %d, want 1", frontend.Replicas)
	}
	edge := firstEdge(s)
	if edge.ErrorRate != 0 || edge.Timeout != 0 || edge.Retries != 0 {
		t.Fatalf("edge defaults not applied: %+v", edge)
	}
	if edge.RetryBackoff != topology.BackoffExponential {
		t.Fatalf("RetryBackoff = %v, want exponential", edge.RetryBackoff)
	}
	if edge.Latency.Distribution != "constant" || edge.Latency.P50 != 10*time.Millisecond || edge.Latency.P95 != 10*time.Millisecond {
		t.Fatalf("latency defaults not applied: %+v", edge.Latency)
	}
	if s.Journeys["home"].Weight != 1 {
		t.Fatalf("journey weight = %v, want 1", s.Journeys["home"].Weight)
	}
}

func TestParse_NamespaceOverride(t *testing.T) {
	t.Parallel()

	const src = `
namespace: shop
services:
  frontend:
    kind: application
    operations: [{ name: GET / }]
  payment:
    namespace: pci
    kind: application
    operations: [{ name: authorize }]
journeys:
  home:
    steps:
      - service: frontend
        operation: GET /
`
	s, err := topology.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if s.Namespace != "shop" {
		t.Fatalf("Schema.Namespace = %q, want shop", s.Namespace)
	}
	if s.Services["frontend"].Namespace != "shop" {
		t.Fatalf("frontend namespace = %q, want inherited shop", s.Services["frontend"].Namespace)
	}
	if s.Services["payment"].Namespace != "pci" {
		t.Fatalf("payment namespace = %q, want pci", s.Services["payment"].Namespace)
	}
}

func TestParse_YAMLSyntaxError_FailFast(t *testing.T) {
	t.Parallel()

	_, err := topology.Parse(strings.NewReader("services:\n  frontend: ["))
	if err == nil {
		t.Fatal("Parse() error = nil, want syntax error")
	}
	var parseErr *topology.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error type = %T, want *ParseError", err)
	}
}

func TestParse_UnresolvedReference_ErrorPath(t *testing.T) {
	t.Parallel()

	const src = `
services:
  frontend:
    kind: application
    operations:
      - name: GET /
        calls:
          - to: { service: missing, operation: Fetch }
            protocol: http
journeys:
  home:
    steps:
      - service: frontend
        operation: GET /
`
	_, err := topology.Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("Parse() error = nil, want unresolved reference")
	}
	assertContains(t, err.Error(), "calls[0].to.service")
}

func TestParse_UnknownYAMLKey_IgnoredInParse(t *testing.T) {
	t.Parallel()

	const src = `
services:
  frontend:
    kind: application
    unexpected: ignored
    operations:
      - name: GET /
journeys:
  home:
    steps:
      - service: frontend
        operation: GET /
`
	if _, err := topology.Parse(strings.NewReader(src)); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestParseFile_NotFound(t *testing.T) {
	t.Parallel()

	_, err := topology.ParseFile("testdata/does-not-exist.yaml")
	if err == nil {
		t.Fatal("ParseFile() error = nil, want open error")
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("error type = %T, want *os.PathError", err)
	}
}

const minimalYAML = `
services:
  frontend:
    kind: application
    operations:
      - name: GET /
journeys:
  home:
    steps:
      - service: frontend
        operation: GET /
`

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func mustParse(t *testing.T, src string) *topology.Schema {
	t.Helper()
	s, err := topology.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return s
}

func validManualSchema() *topology.Schema {
	frontend := &topology.Service{
		Name:       "frontend",
		Namespace:  topology.DefaultNamespace,
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: make(map[string]*topology.Operation),
	}
	backend := &topology.Service{
		Name:       "backend",
		Namespace:  topology.DefaultNamespace,
		Kind:       topology.KindApplication,
		Replicas:   1,
		Operations: make(map[string]*topology.Operation),
	}
	root := &topology.Operation{Name: "GET /", Service: frontend}
	fetch := &topology.Operation{Name: "Fetch", Service: backend}
	frontend.Operations[root.Name] = root
	backend.Operations[fetch.Name] = fetch
	root.Calls = []*topology.CallNode{{
		Edge: &topology.Edge{
			From:         root,
			To:           fetch,
			Protocol:     topology.ProtocolHTTP,
			Latency:      topology.LatencyDist{Distribution: "constant", P50: 10 * time.Millisecond, P95: 20 * time.Millisecond},
			RetryBackoff: topology.BackoffExponential,
		},
	}}
	return &topology.Schema{
		Namespace: topology.DefaultNamespace,
		Services: map[topology.ServiceID]*topology.Service{
			frontend.Name: frontend,
			backend.Name:  backend,
		},
		Journeys: map[string]*topology.Journey{
			"home": {Name: "home", Steps: []*topology.Step{{Op: root}}, Weight: 1},
		},
	}
}

func firstEdge(s *topology.Schema) *topology.Edge {
	for _, svc := range s.Services {
		for _, op := range svc.Operations {
			if edge := firstEdgeInCalls(op.Calls); edge != nil {
				return edge
			}
		}
	}
	return nil
}

func firstEdgeInCalls(nodes []*topology.CallNode) *topology.Edge {
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Edge != nil {
			return node.Edge
		}
		if edge := firstEdgeInCalls(node.Parallel); edge != nil {
			return edge
		}
	}
	return nil
}

func allOperations(s *topology.Schema) []*topology.Operation {
	ops := make([]*topology.Operation, 0)
	for _, svc := range s.Services {
		for _, op := range svc.Operations {
			ops = append(ops, op)
		}
	}
	return ops
}

func allEdges(s *topology.Schema) []*topology.Edge {
	edges := make([]*topology.Edge, 0)
	for _, op := range allOperations(s) {
		edges = append(edges, edgesInCalls(op.Calls)...)
	}
	return edges
}

func edgesInCalls(nodes []*topology.CallNode) []*topology.Edge {
	edges := make([]*topology.Edge, 0)
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Edge != nil {
			edges = append(edges, node.Edge)
			if node.Edge.OnFailure != nil {
				edges = append(edges, node.Edge.OnFailure.Fallback...)
			}
		}
		edges = append(edges, edgesInCalls(node.Parallel)...)
	}
	return edges
}
