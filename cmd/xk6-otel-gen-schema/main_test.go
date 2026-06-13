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

func TestRun_StdoutDefault(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"$schema"`) {
		t.Fatalf("stdout missing $schema marker: %s", prefix(stdout.String(), 200))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRun_OutputToFile(t *testing.T) {
	t.Parallel()

	outputPath := filepath.Join(t.TempDir(), "topology.schema.json")
	var stdout, stderr bytes.Buffer
	code := run([]string{"-output", outputPath}, &stdout, &stderr)

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
	if !strings.Contains(string(data), `"$schema"`) {
		t.Fatalf("output file missing $schema marker: %s", prefix(string(data), 200))
	}
}

func TestRun_FlagParseError(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := run([]string{"-unknown"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("stderr = %q, want unknown flag message", stderr.String())
	}
}

func TestRun_FileCreateFailure(t *testing.T) {
	t.Parallel()

	outputPath := filepath.Join(t.TempDir(), "missing", "schema.json")
	var stdout, stderr bytes.Buffer
	code := run([]string{"-output", outputPath}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run() exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "write") {
		t.Fatalf("stderr = %q, want file write error", stderr.String())
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
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), "Usage of xk6-otel-gen-schema") {
				t.Fatalf("stderr = %q, want usage", stderr.String())
			}
		})
	}
}

func TestRun_StdoutWriteFailure(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	code := run(nil, failingWriter{}, &stderr)

	if code != 1 {
		t.Fatalf("run() exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "write stdout") {
		t.Fatalf("stderr = %q, want stdout write error", stderr.String())
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
