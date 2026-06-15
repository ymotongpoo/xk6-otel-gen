// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/journey"
	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracecollectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const collectorEndpoint = "localhost:4317"

// StartCollector starts the Docker Compose Collector used by integration tests.
func StartCollector(t *testing.T) (endpoint string, cleanup func()) {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker is required for journey integration tests: %v", err)
	}

	logsDir := filepath.Join(integrationTestdataDir(), "otel-logs")
	if err := os.RemoveAll(logsDir); err != nil {
		t.Fatalf("remove collector logs: %v", err)
	}
	if err := os.MkdirAll(logsDir, 0o777); err != nil {
		t.Fatalf("create collector logs dir: %v", err)
	}
	if err := os.Chmod(logsDir, 0o777); err != nil {
		t.Fatalf("chmod collector logs dir: %v", err)
	}

	runCompose(t, "up", "-d")
	cleanup = func() {
		runCompose(t, "down", "--volumes", "--remove-orphans")
	}
	ready := false
	defer func() {
		if !ready {
			cleanup()
		}
	}()
	waitForCollector(t, collectorEndpoint, 30*time.Second)
	ready = true
	return collectorEndpoint, cleanup
}

// ReadCollectorTraces reads the Collector file exporter trace output.
func ReadCollectorTraces(t *testing.T) []byte {
	t.Helper()
	return readCollectorFile(t, "traces.json")
}

// BuildEngine wires real exporter and synth components into a Journey Engine.
func BuildEngine(t *testing.T, schema *topology.Schema, overlay *topology.FaultOverlay) (*journey.Engine, *exporter.Pipeline) {
	t.Helper()

	p, err := exporter.New(exporter.Config{
		Endpoint:     collectorEndpoint,
		Insecure:     true,
		Timeout:      5 * time.Second,
		BatchSize:    1,
		BatchTimeout: 100 * time.Millisecond,
		MaxQueueSize: 16,
	})
	if err != nil {
		t.Fatalf("exporter.New() error = %v", err)
	}
	syn := synth.NewDefault(p, p.MeterProvider(), p.ProfileExporter())
	return journey.NewEngine(schema, overlay, syn), p
}

func forceFlush(t *testing.T, ctx context.Context, p *exporter.Pipeline) {
	t.Helper()

	if provider, ok := p.TracerProvider().(*sdktrace.TracerProvider); ok {
		if err := provider.ForceFlush(ctx); err != nil {
			t.Fatalf("trace ForceFlush() error = %v", err)
		}
	}
	if provider, ok := p.MeterProvider().(*sdkmetric.MeterProvider); ok {
		if err := provider.ForceFlush(ctx); err != nil {
			t.Fatalf("metric ForceFlush() error = %v", err)
		}
	}
	if provider, ok := p.LoggerProvider().(*sdklog.LoggerProvider); ok {
		if err := provider.ForceFlush(ctx); err != nil {
			t.Fatalf("log ForceFlush() error = %v", err)
		}
	}
}

func runCompose(t *testing.T, args ...string) {
	t.Helper()

	composeFile := filepath.Join(integrationTestdataDir(), "docker-compose.yaml")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmdArgs := append([]string{"compose", "-f", composeFile}, args...)
	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s failed: %v\n%s", strings.Join(cmdArgs, " "), err, output)
	}
}

func waitForCollector(t *testing.T, endpoint string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			_, err = tracecollectorpb.NewTraceServiceClient(conn).Export(ctx, &tracecollectorpb.ExportTraceServiceRequest{})
			_ = conn.Close()
		}
		cancel()
		if err == nil {
			return
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("collector endpoint %s was not ready within %s: %v", endpoint, timeout, lastErr)
}

func readCollectorFile(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join(integrationTestdataDir(), "otel-logs", name)
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(strings.TrimSpace(string(data))) > 0 {
			return data
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("read collector output %s: %v", path, fmt.Errorf("no non-empty file before timeout: %w", lastErr))
	return nil
}

func integrationTestdataDir() string {
	return filepath.Join("..", "testdata")
}

func shutdownPipeline(t *testing.T, p *exporter.Pipeline) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	forceFlush(t, ctx, p)
	if err := p.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func sequentialSchema() *topology.Schema {
	entrySvc := &topology.Service{Name: "entry", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	middleSvc := &topology.Service{Name: "middle", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	leafSvc := &topology.Service{Name: "leaf", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	entry := &topology.Operation{Name: "GET /entry", Service: entrySvc}
	middle := &topology.Operation{Name: "GET /middle", Service: middleSvc}
	leaf := &topology.Operation{Name: "GET /leaf", Service: leafSvc}
	entrySvc.Operations[entry.Name] = entry
	middleSvc.Operations[middle.Name] = middle
	leafSvc.Operations[leaf.Name] = leaf
	entryToMiddle := &topology.Edge{From: entry, To: middle, Protocol: topology.ProtocolHTTP, Latency: topology.LatencyDist{Distribution: "fixed"}}
	middleToLeaf := &topology.Edge{From: middle, To: leaf, Protocol: topology.ProtocolHTTP, Latency: topology.LatencyDist{Distribution: "fixed"}}
	entry.Calls = []*topology.CallNode{{Edge: entryToMiddle}}
	middle.Calls = []*topology.CallNode{{Edge: middleToLeaf}}
	return &topology.Schema{
		Services: map[topology.ServiceID]*topology.Service{
			entrySvc.Name:  entrySvc,
			middleSvc.Name: middleSvc,
			leafSvc.Name:   leafSvc,
		},
		Journeys: map[string]*topology.Journey{
			"chain": {Name: "chain", Steps: []*topology.Step{{Op: entry}}, Weight: 1},
		},
	}
}

func recoverySchema() *topology.Schema {
	schema := sequentialSchema()
	entry := schema.Services["entry"].Operations["GET /entry"]
	fallbackSvc := &topology.Service{Name: "fallback", Kind: topology.KindApplication, Replicas: 1, Operations: make(map[string]*topology.Operation)}
	fallback := &topology.Operation{Name: "GET /fallback", Service: fallbackSvc}
	fallbackSvc.Operations[fallback.Name] = fallback
	schema.Services[fallbackSvc.Name] = fallbackSvc
	primaryEdge := entry.Calls[0].Edge
	primaryEdge.OnFailure = &topology.RecoveryPolicy{
		Fallback:    []*topology.Edge{{From: entry, To: fallback, Protocol: topology.ProtocolHTTP, Latency: topology.LatencyDist{Distribution: "fixed"}}},
		OnExhausted: topology.ExhaustedPropagate,
	}
	return schema
}
