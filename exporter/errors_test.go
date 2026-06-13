// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"errors"
	"testing"
)

func TestPipelineError_Error(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("dial failed")
	tests := []struct {
		name string
		err  *PipelineError
		want string
	}{
		{
			name: "with inner",
			err:  &PipelineError{Stage: "trace_exporter", Inner: sentinel},
			want: "exporter: pipeline trace_exporter failed: dial failed",
		},
		{
			name: "without inner",
			err:  &PipelineError{Stage: "validate"},
			want: "exporter: pipeline validate failed",
		},
		{
			name: "nil receiver",
			err:  nil,
			want: "<nil>",
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

func TestPipelineError_Unwrap(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("resource failed")
	err := &PipelineError{Stage: "resource", Inner: sentinel}
	if !errors.Is(err, sentinel) {
		t.Fatalf("errors.Is(%v, sentinel) = false, want true", err)
	}
}

func TestConfigError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *ConfigError
		want string
	}{
		{
			name: "string value",
			err:  &ConfigError{Field: "Endpoint", Value: "", Message: "must not be empty"},
			want: "exporter: invalid Config.Endpoint = : must not be empty",
		},
		{
			name: "duration-like value",
			err:  &ConfigError{Field: "Timeout", Value: 0, Message: "must be > 0"},
			want: "exporter: invalid Config.Timeout = 0: must be > 0",
		},
		{
			name: "map value",
			err:  &ConfigError{Field: "Headers", Value: map[string]string{"": "v"}, Message: "keys must not be empty"},
			want: "exporter: invalid Config.Headers = map[:v]: keys must not be empty",
		},
		{
			name: "nil receiver",
			err:  nil,
			want: "<nil>",
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

func TestSharedError_Error(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("factory failed")
	tests := []struct {
		name string
		err  *SharedError
		want string
	}{
		{
			name: "with inner",
			err:  &SharedError{Reason: "init_failed", Inner: sentinel},
			want: "exporter: shared pipeline init_failed: factory failed",
		},
		{
			name: "without inner",
			err:  &SharedError{Reason: "already_initialized"},
			want: "exporter: shared pipeline already_initialized",
		},
		{
			name: "nil receiver",
			err:  nil,
			want: "<nil>",
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
