// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6output

import (
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"go.k6.io/k6/output"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

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

func newManualTestOutput(t *testing.T) (*Output, *sdkmetric.ManualReader) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	o := &Output{
		params:        defaultParams(),
		meterProvider: mp,
		setCache:      &tagSetCache{},
		logger:        func(string, ...any) {},
	}
	if err := o.buildKnownInstruments(); err != nil {
		t.Fatalf("buildKnownInstruments() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mp.Shutdown(ctx)
	})
	return o, reader
}
