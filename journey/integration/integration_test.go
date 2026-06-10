//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestIntegration_Sequential_Correlated(t *testing.T) {
	_, cleanup := StartCollector(t)
	defer cleanup()

	schema := sequentialSchema()
	engine, pipeline := BuildEngine(t, schema, schema.ApplyFaults())
	plan, err := engine.BuildPlan("chain")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	shutdownPipeline(t, pipeline)

	traces := waitForTraces(t, "entry.GET /entry", "middle.GET /middle", "leaf.GET /leaf")
	if got := len(uniqueTraceIDs(traces)); got != 1 {
		t.Fatalf("unique trace_id count = %d, want 1\n%s", got, traces)
	}
}

func TestIntegration_CascadePropagation(t *testing.T) {
	_, cleanup := StartCollector(t)
	defer cleanup()

	schema := sequentialSchema()
	schema.Faults = []topology.FaultSpec{{
		Target:   topology.FaultTarget{Kind: topology.TargetNode, Service: schema.Services["middle"]},
		Kind:     topology.FaultCrash,
		Severity: topology.SeverityParams{Probability: 1, Multiplier: 1},
	}}
	engine, pipeline := BuildEngine(t, schema, schema.ApplyFaults())
	plan, err := engine.BuildPlan("chain")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	shutdownPipeline(t, pipeline)

	traces := waitForTraces(t, "entry.GET /entry", "middle.GET /middle", "leaf.GET /leaf", "crashed", "synth.cascaded")
	if got := len(uniqueTraceIDs(traces)); got != 1 {
		t.Fatalf("unique trace_id count = %d, want 1\n%s", got, traces)
	}
}

func TestIntegration_Recovery_FallbackUsed(t *testing.T) {
	_, cleanup := StartCollector(t)
	defer cleanup()

	schema := recoverySchema()
	middle := schema.Services["middle"].Operations["GET /middle"]
	schema.Faults = []topology.FaultSpec{{
		Target:   topology.FaultTarget{Kind: topology.TargetOperation, Operation: middle},
		Kind:     topology.FaultErrorRateOverride,
		Severity: topology.SeverityParams{Probability: 1, Multiplier: 1, Value: 1},
	}}
	engine, pipeline := BuildEngine(t, schema, schema.ApplyFaults())
	plan, err := engine.BuildPlan("chain")
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if err := engine.Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	shutdownPipeline(t, pipeline)

	traces := waitForTraces(t, "middle.GET /middle", "fallback.GET /fallback", "http.500")
	if got := len(uniqueTraceIDs(traces)); got != 1 {
		t.Fatalf("unique trace_id count = %d, want 1\n%s", got, traces)
	}
}

func uniqueTraceIDs(body string) map[string]struct{} {
	matches := regexp.MustCompile(`"traceId"\s*:\s*"([^"]+)"`).FindAllStringSubmatch(body, -1)
	out := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) == 2 && match[1] != "" {
			out[match[1]] = struct{}{}
		}
	}
	return out
}

func waitForTraces(t *testing.T, wants ...string) string {
	t.Helper()

	path := filepath.Join(integrationTestdataDir(), "otel-logs", "traces.json")
	deadline := time.Now().Add(30 * time.Second)
	var lastBody string
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			lastBody = string(data)
			allFound := true
			for _, want := range wants {
				if !strings.Contains(lastBody, want) {
					allFound = false
					break
				}
			}
			if allFound {
				return lastBody
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("traces output did not contain %v before timeout:\n%s", wants, lastBody)
	return ""
}

func assertContains(t *testing.T, fileName, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("%s output does not contain %q:\n%s", fileName, want, body)
	}
}
