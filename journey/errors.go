// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"fmt"
	"strings"
)

// PlanError describes a failure while building an immutable journey Plan.
type PlanError struct {
	Kind  string
	Path  []string
	Inner error
}

// Error returns the formatted plan-build failure.
func (e *PlanError) Error() string {
	if e == nil {
		return "journey: BuildPlan: <nil>"
	}
	var b strings.Builder
	b.WriteString("journey: BuildPlan: ")
	b.WriteString(e.Kind)
	if len(e.Path) > 0 {
		b.WriteString(" at ")
		b.WriteString(strings.Join(e.Path, " -> "))
	}
	if e.Inner != nil {
		b.WriteString(": ")
		b.WriteString(e.Inner.Error())
	}
	return b.String()
}

// Unwrap returns the wrapped lower-level error, if any.
func (e *PlanError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}

// ExecuteError describes programmer errors and recovered panics during Execute.
type ExecuteError struct {
	Kind  string
	Inner error
}

// Error returns the formatted execution failure.
func (e *ExecuteError) Error() string {
	if e == nil {
		return "journey: Execute: <nil>"
	}
	if e.Inner != nil {
		return fmt.Sprintf("journey: Execute: %s: %v", e.Kind, e.Inner)
	}
	return fmt.Sprintf("journey: Execute: %s", e.Kind)
}

// Unwrap returns the wrapped lower-level error, if any.
func (e *ExecuteError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}

// AllowedErrorTypes lists the semantic error.type values the journey engine can
// emit in step outcomes. Treat this slice as read-only.
var AllowedErrorTypes = []string{
	"timeout",
	"connection_refused",
	"dns_failure",
	"http.500",
	"http.502",
	"http.503",
	"http.504",
	"grpc.unavailable",
	"grpc.deadline_exceeded",
	"grpc.unauthenticated",
	"db.connection_lost",
	"db.constraint_violation",
	"crashed",
	"circuit_open",
	"rate_limited",
	"context_canceled",
}
