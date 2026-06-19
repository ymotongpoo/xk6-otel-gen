// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalTopologyPath = "../../examples/minimal/topology.yaml"

func TestRun_MinimalTopology(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := run([]string{"-input", minimalTopologyPath}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "<title>demo") {
		t.Fatalf("stdout missing title: %s", prefix(out, 300))
	}
	if !strings.Contains(strings.ToLower(out), "cytoscape") {
		t.Fatalf("stdout missing cytoscape: %s", prefix(out, 300))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRun_OutputToFile(t *testing.T) {
	t.Parallel()

	outputPath := filepath.Join(t.TempDir(), "topology.html")
	var stdout, stderr bytes.Buffer
	code := run([]string{"-input", minimalTopologyPath, "-output", outputPath}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", outputPath, err)
	}
	if !strings.Contains(string(data), "<title>demo") {
		t.Fatalf("output file missing title: %s", prefix(string(data), 300))
	}
}

func TestRun_MissingInputFlag(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "-input flag is required") {
		t.Fatalf("stderr = %q, want missing input message", stderr.String())
	}
}

func TestRun_InvalidInputFile(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := run([]string{"-input", "/nonexistent/topology.yaml"}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run() exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "parse") {
		t.Fatalf("stderr = %q, want parse error", stderr.String())
	}
}

func TestRun_FlagParseError(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := run([]string{"-unknown"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("stderr = %q, want unknown flag message", stderr.String())
	}
}

func TestRun_HelpFlag(t *testing.T) {
	t.Parallel()

	for _, flagName := range []string{"-h", "-help"} {
		flagName := flagName
		t.Run(flagName, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := run([]string{flagName}, &stdout, &stderr)

			if code != 0 {
				t.Fatalf("run() exit code = %d, want 0", code)
			}
			if !strings.Contains(stderr.String(), "Usage of xk6-otel-gen-viz") {
				t.Fatalf("stderr = %q, want usage", stderr.String())
			}
		})
	}
}

func TestRun_StdoutWriteFailure(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	code := run([]string{"-input", minimalTopologyPath}, failingWriter{}, &stderr)

	if code != 1 {
		t.Fatalf("run() exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "write stdout") {
		t.Fatalf("stderr = %q, want stdout write error", stderr.String())
	}
}

func TestRun_FileCreateFailure(t *testing.T) {
	t.Parallel()

	outputPath := filepath.Join(t.TempDir(), "missing", "topology.html")
	var stdout, stderr bytes.Buffer
	code := run([]string{"-input", minimalTopologyPath, "-output", outputPath}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run() exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "create") {
		t.Fatalf("stderr = %q, want file create error", stderr.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("forced write failure")
}

func prefix(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}
