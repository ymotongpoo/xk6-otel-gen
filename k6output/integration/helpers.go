// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tracecollectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type commandOutput struct {
	ExitCode int
	Output   string
}

type metricsContent struct {
	Raw       string
	Documents []map[string]any
}

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker is required for k6output integration tests: %v", err)
	}
}

func requireXK6(t *testing.T) string {
	t.Helper()
	xk6, err := exec.LookPath("xk6")
	if err != nil {
		t.Skipf("xk6 is required for k6output integration tests: %v", err)
	}
	return xk6
}

func buildK6Binary(t *testing.T, xk6Path, modulePath, outputDir string) string {
	t.Helper()
	binPath := filepath.Join(outputDir, "k6")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, xk6Path, "build", "--output", binPath, "--with", modulePath+"=.")
	cmd.Dir = repoRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xk6 build failed: %v\n%s", err, output)
	}
	return binPath
}

func startCollector(t *testing.T, configDir string) (endpoint string, cleanup func()) {
	t.Helper()
	logsDir := filepath.Join(configDir, "otel-logs")
	if err := os.RemoveAll(logsDir); err != nil {
		t.Fatalf("remove collector logs: %v", err)
	}
	if err := os.MkdirAll(logsDir, 0o777); err != nil {
		t.Fatalf("create collector logs dir: %v", err)
	}
	if err := os.Chmod(logsDir, 0o777); err != nil {
		t.Fatalf("chmod collector logs dir: %v", err)
	}

	runCompose(t, configDir, "up", "-d")
	cleanup = func() {
		runCompose(t, configDir, "down", "--volumes", "--remove-orphans")
	}
	ready := false
	defer func() {
		if !ready {
			cleanup()
		}
	}()
	endpoint = "localhost:4317"
	waitForCollector(t, endpoint, 30*time.Second)
	ready = true
	return endpoint, cleanup
}

func runK6Script(t *testing.T, k6Bin, scriptPath string, args ...string) commandOutput {
	t.Helper()
	cmdArgs := append([]string{"run"}, args...)
	cmdArgs = append(cmdArgs, filepath.Base(scriptPath))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, k6Bin, cmdArgs...)
	cmd.Dir = filepath.Dir(scriptPath)
	cmd.Env = append(os.Environ(), "OTEL_ENDPOINT=localhost:4317")
	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		exitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	return commandOutput{ExitCode: exitCode, Output: string(output)}
}

func readCollectorMetrics(t *testing.T, path string) metricsContent {
	t.Helper()
	raw := readCollectorFile(t, path)
	var docs []map[string]any
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(line), &doc); err != nil {
			t.Fatalf("parse collector metric JSON line: %v\n%s", err, line)
		}
		docs = append(docs, doc)
	}
	return metricsContent{Raw: raw, Documents: docs}
}

func runCompose(t *testing.T, configDir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmdArgs := append([]string{"compose", "-f", filepath.Join(configDir, "docker-compose.yaml")}, args...)
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

func readCollectorFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(strings.TrimSpace(string(data))) > 0 {
			return string(data)
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("read collector output %s: %v", path, fmt.Errorf("no non-empty file before timeout: %w", lastErr))
	return ""
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}
