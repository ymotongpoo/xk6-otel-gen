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

	testdata := filepath.Join(repoRoot(t), "k6output", "integration", "testdata")
	endpoint, cleanup := startCollector(t, testdata)
	defer cleanup()

	k6Bin := buildK6Binary(t, xk6, "github.com/ymotongpoo/xk6-otel-gen", t.TempDir())
	output := runK6Script(t, k6Bin, filepath.Join(testdata, "script.js"), "--out", "otel-gen=endpoint="+endpoint+",insecure=true,batchSize=1,batchTimeout=100ms")
	if output.ExitCode != 0 {
		t.Fatalf("k6 run exit code = %d, want 0\n%s", output.ExitCode, output.Output)
	}

	metrics := readCollectorMetrics(t, filepath.Join(testdata, "otel-logs", "metrics.json"))
	assertContains(t, "metrics", metrics.Raw, "k6.iterations.total")
	assertContains(t, "metrics", metrics.Raw, "k6.vus")
	assertContains(t, "metrics", metrics.Raw, "xk6-otel-gen-runner")
	assertContains(t, "metrics", metrics.Raw, "service.name")
	if len(metrics.Documents) == 0 {
		t.Fatal("collector metrics JSON parsed no documents")
	}
}

func assertContains(t *testing.T, fileName, body, want string) {
	t.Helper()

	if !strings.Contains(body, want) {
		t.Fatalf("%s output does not contain %q:\n%s", fileName, want, body)
	}
}
