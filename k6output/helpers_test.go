package k6output

import (
	"net/url"
	"sync"
	"testing"

	"go.k6.io/k6/output"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

var sharedPipelineTestMu sync.Mutex

func newTestParams(t *testing.T, args string) output.Params {
	t.Helper()

	scriptURL, err := url.Parse("file:///tmp/test.js")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	return output.Params{
		OutputType:     "otel-gen",
		ConfigArgument: args,
		ScriptPath:     scriptURL,
	}
}

func newTestOutput(t *testing.T, args string) *Output {
	t.Helper()

	sharedPipelineTestMu.Lock()
	t.Cleanup(sharedPipelineTestMu.Unlock)
	exporter.ResetShared()

	out, err := New(newTestParams(t, args))
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	o, ok := out.(*Output)
	if !ok {
		t.Fatalf("New() = %T, want *Output", out)
	}
	return o
}

func recordingLogger() (func(string, ...any), *[]string) {
	var logs []string
	return func(format string, args ...any) {
		logs = append(logs, format)
	}, &logs
}
