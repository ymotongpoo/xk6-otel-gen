// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestLint_UnknownYAMLKeyWarning(t *testing.T) {
	t.Parallel()

	const src = `
services:
  frontend:
    kind: application
    unknown_service_key: ignored
    operations:
      - name: GET /
journeys:
  home:
    steps:
      - service: frontend
        operation: GET /
`
	issues, err := topology.Lint(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Lint() error = %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("Lint() returned no issues, want warning")
	}
	if issues[0].Severity != topology.LintWarning {
		t.Fatalf("severity = %v, want warning", issues[0].Severity)
	}
	assertContains(t, issues[0].Message, "unknown_service_key")
}

func TestLint_ValidationErrors(t *testing.T) {
	t.Parallel()

	const src = `
services:
  frontend:
    kind: application
    replicas: 0
    operations:
      - name: GET /
journeys:
  home:
    steps:
      - service: frontend
        operation: GET /
`
	issues, err := topology.Lint(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Lint() error = %v", err)
	}
	if !hasLintIssue(issues, topology.LintError, "D-1") {
		t.Fatalf("Lint() issues = %+v, want D-1 error", issues)
	}
}

func TestLint_TypeErrorAsIssue(t *testing.T) {
	t.Parallel()

	const src = `
services:
  frontend:
    kind: application
    operations:
      - name: GET /
        calls:
          - to: { service: backend, operation: Fetch }
            protocol: http
            timeout: [bad]
  backend:
    kind: application
    operations:
      - name: Fetch
journeys:
  home:
    steps:
      - service: frontend
        operation: GET /
`
	issues, err := topology.Lint(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Lint() error = %v", err)
	}
	if !hasLintIssue(issues, topology.LintError, "yaml decode failed") {
		t.Fatalf("Lint() issues = %+v, want YAML type error issue", issues)
	}
}

func TestLint_YAMLSyntaxErrorReturnsError(t *testing.T) {
	t.Parallel()

	issues, err := topology.Lint(strings.NewReader("services:\n  frontend: ["))
	if err == nil {
		t.Fatalf("Lint() error = nil, issues = %+v", issues)
	}
	var parseErr *topology.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error type = %T, want *ParseError", err)
	}
}

func TestLintSeverity_String(t *testing.T) {
	t.Parallel()

	tests := map[topology.LintSeverity]string{
		topology.LintError:       "error",
		topology.LintWarning:     "warning",
		topology.LintSeverity(9): "unknown",
	}
	for severity, want := range tests {
		if got := severity.String(); got != want {
			t.Fatalf("String() = %q, want %q", got, want)
		}
	}
}

func TestSchema_JourneyNames(t *testing.T) {
	t.Parallel()

	s := validManualSchema()
	s.Journeys["alpha"] = &topology.Journey{Name: "alpha", Steps: s.Journeys["home"].Steps, Weight: 1}
	got := s.JourneyNames()
	want := []string{"alpha", "home"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("JourneyNames() = %v, want %v", got, want)
		}
	}
}

func TestErrorTypes(t *testing.T) {
	t.Parallel()

	inner := errors.New("inner")
	parseErr := &topology.ParseError{Path: "services", Message: "bad", Inner: inner}
	if !errors.Is(parseErr, inner) {
		t.Fatal("ParseError does not unwrap inner error")
	}
	assertContains(t, parseErr.Error(), "services")
	validationErr := &topology.ValidationError{Path: "services", Rule: "D-13", Message: "empty"}
	assertContains(t, validationErr.Error(), "D-13")
	assertContains(t, (*topology.ParseError)(nil).Error(), "<nil>")
	assertContains(t, (*topology.ValidationError)(nil).Error(), "<nil>")
}

func hasLintIssue(issues []topology.LintIssue, severity topology.LintSeverity, contains string) bool {
	for _, issue := range issues {
		if issue.Severity == severity && strings.Contains(issue.Message, contains) {
			return true
		}
	}
	return false
}
