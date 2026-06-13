// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6output

import "fmt"

const (
	// ConfigErrorKindInvalidArgs identifies syntactically invalid --out args.
	ConfigErrorKindInvalidArgs = "invalid_args"
	// ConfigErrorKindInvalidProtocol identifies unsupported OTLP protocols.
	ConfigErrorKindInvalidProtocol = "invalid_protocol"
	// ConfigErrorKindTypeMismatch identifies values that cannot be decoded.
	ConfigErrorKindTypeMismatch = "type_mismatch"
	// ConfigErrorKindInvalidURL identifies malformed endpoint URLs.
	ConfigErrorKindInvalidURL = "invalid_url"
)

// ConfigError describes a k6output configuration parsing failure.
type ConfigError struct {
	Kind  string
	Field string
	Value string
	Inner error
}

// Error formats the configuration failure for k6 startup diagnostics.
func (e *ConfigError) Error() string {
	if e == nil {
		return "k6output: config error"
	}
	msg := "k6output: " + e.Kind
	switch {
	case e.Field != "" && e.Value != "":
		msg += fmt.Sprintf(" (%s=%q)", e.Field, e.Value)
	case e.Field != "":
		msg += fmt.Sprintf(" (%s)", e.Field)
	case e.Value != "":
		msg += fmt.Sprintf(" (%q)", e.Value)
	}
	if e.Inner != nil {
		msg += ": " + e.Inner.Error()
	}
	return msg
}

// Unwrap returns the underlying parsing error, if any.
func (e *ConfigError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}
