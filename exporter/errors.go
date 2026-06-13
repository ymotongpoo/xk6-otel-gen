// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter

import "fmt"

// PipelineError wraps failures returned by New while building or validating a Pipeline.
type PipelineError struct {
	Stage string
	Inner error
}

// Error returns a stage-qualified Pipeline construction error.
func (e *PipelineError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Inner != nil {
		return fmt.Sprintf("exporter: pipeline %s failed: %v", e.Stage, e.Inner)
	}
	return fmt.Sprintf("exporter: pipeline %s failed", e.Stage)
}

// Unwrap returns the underlying New failure.
func (e *PipelineError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}

// ConfigError describes a Config.Validate field-level validation failure.
type ConfigError struct {
	Field   string
	Value   any
	Message string
}

// Error returns a Config field validation error message.
func (e *ConfigError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("exporter: invalid Config.%s = %v: %s", e.Field, e.Value, e.Message)
}

// SharedError wraps failures returned by GetShared or SetShared.
type SharedError struct {
	Reason string
	Inner  error
}

// Error returns a reason-qualified shared Pipeline holder error.
func (e *SharedError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Inner != nil {
		return fmt.Sprintf("exporter: shared pipeline %s: %v", e.Reason, e.Inner)
	}
	return fmt.Sprintf("exporter: shared pipeline %s", e.Reason)
}

// Unwrap returns the underlying shared holder error.
func (e *SharedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}
