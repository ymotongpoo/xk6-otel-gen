// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"errors"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/journey"
)

func TestConfigError_Error(t *testing.T) {
	t.Parallel()

	inner := errors.New("read failed")
	tests := []struct {
		name string
		err  *ConfigError
		want string
	}{
		{
			name: "without inner",
			err:  &ConfigError{Kind: "file_not_found", Path: "topology.yaml"},
			want: "k6otelgen: file_not_found (topology.yaml)",
		},
		{
			name: "with inner",
			err:  &ConfigError{Kind: "parse_error", Path: "topology.yaml", Inner: inner},
			want: "k6otelgen: parse_error (topology.yaml): read failed",
		},
		{
			name: "without path",
			err:  &ConfigError{Kind: "not_loaded"},
			want: "k6otelgen: not_loaded",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigError_Unwrap(t *testing.T) {
	t.Parallel()

	inner := errors.New("inner")
	err := &ConfigError{Kind: "parse_error", Inner: inner}
	if got := err.Unwrap(); !errors.Is(got, inner) {
		t.Fatalf("Unwrap() = %v, want %v", got, inner)
	}
}

func TestFormatErrorMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "k6otelgen config",
			err:  &ConfigError{Kind: "not_loaded"},
			want: "k6otelgen: [not_loaded] k6otelgen: not_loaded",
		},
		{
			name: "exporter config",
			err:  &exporter.ConfigError{Field: "Endpoint", Value: "", Message: "must not be empty"},
			want: "k6otelgen: exporter config: exporter: invalid Config.Endpoint = : must not be empty",
		},
		{
			name: "exporter pipeline",
			err:  &exporter.PipelineError{Stage: "trace_exporter", Inner: errors.New("dial failed")},
			want: "k6otelgen: exporter pipeline: exporter: pipeline trace_exporter failed: dial failed",
		},
		{
			name: "journey plan",
			err:  &journey.PlanError{Kind: "unknown_journey", Path: []string{"checkout"}},
			want: "k6otelgen: plan: journey: BuildPlan: unknown_journey at checkout",
		},
		{
			name: "journey execute",
			err:  &journey.ExecuteError{Kind: "nil_ctx"},
			want: "k6otelgen: execute: journey: Execute: nil_ctx",
		},
		{
			name: "generic",
			err:  errors.New("plain failure"),
			want: "k6otelgen: plain failure",
		},
		{
			name: "nil",
			err:  nil,
			want: "k6otelgen: <nil>",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := formatErrorMessage(tt.err); got != tt.want {
				t.Fatalf("formatErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}
