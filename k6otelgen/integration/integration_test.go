// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package integration

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegration_EndToEnd(t *testing.T) {
	requireDocker(t)
	xk6 := requireXK6(t)

	testdata := filepath.Join(repoRoot(t), "k6otelgen", "integration", "testdata")
	endpoint, cleanup := startCollector(t, testdata)
	defer cleanup()

	k6Bin := buildK6Binary(t, xk6, "github.com/ymotongpoo/xk6-otel-gen", t.TempDir())
	output := runK6Script(t, k6Bin, filepath.Join(testdata, "script.js"), "--out", "otel-gen=endpoint="+endpoint)
	if output.ExitCode != 0 {
		t.Fatalf("k6 run exit code = %d, want 0\n%s", output.ExitCode, output.Output)
	}

	traces := readCollectorTraces(t, filepath.Join(testdata, "otel-logs", "traces.json"))
	if !strings.Contains(traces, "frontend") || !strings.Contains(traces, "checkout") {
		t.Fatalf("collector traces missing expected service names:\n%s", traces)
	}
}
