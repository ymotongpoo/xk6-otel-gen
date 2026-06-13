// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"errors"
	"fmt"

	"github.com/grafana/sobek"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/journey"
)

// ConfigError describes k6otelgen load/configure failures surfaced to JS.
type ConfigError struct {
	Kind  string
	Path  string
	Inner error
}

// Error returns a kind- and path-qualified configuration error.
func (e *ConfigError) Error() string {
	if e == nil {
		return "k6otelgen: <nil>"
	}
	if e.Path == "" {
		if e.Inner != nil {
			return fmt.Sprintf("k6otelgen: %s: %v", e.Kind, e.Inner)
		}
		return fmt.Sprintf("k6otelgen: %s", e.Kind)
	}
	if e.Inner != nil {
		return fmt.Sprintf("k6otelgen: %s (%s): %v", e.Kind, e.Path, e.Inner)
	}
	return fmt.Sprintf("k6otelgen: %s (%s)", e.Kind, e.Path)
}

// Unwrap returns the wrapped lower-level error, if any.
func (e *ConfigError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}

func throwJSException(rt *sobek.Runtime, err error) {
	panic(rt.NewTypeError(formatErrorMessage(err)))
}

func formatErrorMessage(err error) string {
	if err == nil {
		return "k6otelgen: <nil>"
	}

	var (
		ce  *ConfigError
		ec  *exporter.ConfigError
		pe  *exporter.PipelineError
		ple *journey.PlanError
		ee  *journey.ExecuteError
	)
	switch {
	case errors.As(err, &ce):
		return fmt.Sprintf("k6otelgen: [%s] %s", ce.Kind, ce.Error())
	case errors.As(err, &ec):
		return fmt.Sprintf("k6otelgen: exporter config: %s", ec.Error())
	case errors.As(err, &pe):
		return fmt.Sprintf("k6otelgen: exporter pipeline: %s", pe.Error())
	case errors.As(err, &ple):
		return fmt.Sprintf("k6otelgen: plan: %s", ple.Error())
	case errors.As(err, &ee):
		return fmt.Sprintf("k6otelgen: execute: %s", ee.Error())
	default:
		return fmt.Sprintf("k6otelgen: %s", err.Error())
	}
}
