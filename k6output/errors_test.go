// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6output

import (
	"errors"
	"testing"
)

func TestConfigError_Error(t *testing.T) {
	t.Parallel()

	inner := errors.New("parse bool")
	tests := []struct {
		name string
		err  *ConfigError
		want string
	}{
		{
			name: "field value without inner",
			err:  &ConfigError{Kind: ConfigErrorKindInvalidProtocol, Field: "protocol", Value: "udp"},
			want: `k6output: invalid_protocol (protocol="udp")`,
		},
		{
			name: "field value with inner",
			err:  &ConfigError{Kind: ConfigErrorKindTypeMismatch, Field: "insecure", Value: "yes", Inner: inner},
			want: `k6output: type_mismatch (insecure="yes"): parse bool`,
		},
		{
			name: "field only",
			err:  &ConfigError{Kind: ConfigErrorKindInvalidArgs, Field: "endpoint"},
			want: `k6output: invalid_args (endpoint)`,
		},
		{
			name: "value only",
			err:  &ConfigError{Kind: ConfigErrorKindInvalidURL, Value: "://"},
			want: `k6output: invalid_url ("://")`,
		},
		{
			name: "kind only",
			err:  &ConfigError{Kind: ConfigErrorKindInvalidArgs},
			want: `k6output: invalid_args`,
		},
		{
			name: "nil receiver",
			err:  nil,
			want: `k6output: config error`,
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

	inner := errors.New("sentinel")
	err := &ConfigError{Kind: ConfigErrorKindTypeMismatch, Inner: inner}
	if !errors.Is(err, inner) {
		t.Fatalf("errors.Is(%v, inner) = false, want true", err)
	}
	var nilErr *ConfigError
	if got := nilErr.Unwrap(); got != nil {
		t.Fatalf("nil ConfigError.Unwrap() = %v, want nil", got)
	}
}
