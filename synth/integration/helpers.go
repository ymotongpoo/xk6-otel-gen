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
	tracecollectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const collectorEndpoint = "localhost:4317"

// StartCollector starts the Docker Compose Collector used by integration tests.
func StartCollector(t *testing.T) (endpoint string, cleanup func()) {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker is required for synth integration tests: %v", err)
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

// ReadCollectorMetrics reads the Collector file exporter metric output.
func ReadCollectorMetrics(t *testing.T) []byte {
	t.Helper()
	return readCollectorFile(t, "metrics.json")
}

// ReadCollectorLogs reads the Collector file exporter log output.
func ReadCollectorLogs(t *testing.T) []byte {
	t.Helper()
	return readCollectorFile(t, "logs.json")
}

// BuildPipeline builds the real exporter pipeline for synth integration tests.
func BuildPipeline(t *testing.T, cfg exporter.Config) *exporter.Pipeline {
	t.Helper()

	p, err := exporter.New(cfg)
	if err != nil {
		t.Fatalf("exporter.New() error = %v", err)
	}
	return p
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
