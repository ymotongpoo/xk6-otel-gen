package topology

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LintIssue describes one topology lint finding.
type LintIssue struct {
	Path     string
	Severity LintSeverity
	Message  string
}

// LintSeverity classifies a lint finding.
type LintSeverity int

const (
	// LintError indicates a parse, reference-resolution, or validation error.
	LintError LintSeverity = iota
	// LintWarning indicates a non-fatal issue such as an unknown YAML field.
	LintWarning
)

// String returns the stable text form of a lint severity.
func (s LintSeverity) String() string {
	switch s {
	case LintError:
		return "error"
	case LintWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// Lint reports topology issues without failing the call for semantic errors.
func Lint(r io.Reader) ([]LintIssue, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("topology: read input: %w", err)
	}

	issues := make([]LintIssue, 0)
	raw, strictErr := decodeRaw(bytes.NewReader(data), true)
	if strictErr != nil {
		warnings, ok := unknownFieldWarnings(strictErr)
		if !ok {
			laxRaw, laxErr := decodeRaw(bytes.NewReader(data), false)
			if laxErr != nil {
				var typeErr *yaml.TypeError
				if errors.As(laxErr, &typeErr) {
					issues = appendIssuesFromError(issues, laxErr, LintError)
					sortLintIssues(issues)
					return issues, nil
				}
				return issues, laxErr
			}
			raw = laxRaw
			issues = appendIssuesFromError(issues, strictErr, LintError)
		} else {
			issues = append(issues, warnings...)
			laxRaw, laxErr := decodeRaw(bytes.NewReader(data), false)
			if laxErr != nil {
				var typeErr *yaml.TypeError
				if errors.As(laxErr, &typeErr) {
					issues = appendIssuesFromError(issues, laxErr, LintError)
					sortLintIssues(issues)
					return issues, nil
				}
				return issues, laxErr
			}
			raw = laxRaw
		}
	}

	schema := buildSchema(raw)
	if err := resolveReferences(schema, raw); err != nil {
		issues = appendIssuesFromError(issues, err, LintError)
	}
	if err := Validate(schema); err != nil {
		issues = appendIssuesFromError(issues, err, LintError)
	}

	sortLintIssues(issues)
	return issues, nil
}

func unknownFieldWarnings(err error) ([]LintIssue, bool) {
	typeErr := yamlTypeError(err)
	if typeErr == nil {
		return nil, false
	}

	warnings := make([]LintIssue, 0, len(typeErr.Errors))
	for _, msg := range typeErr.Errors {
		if !strings.Contains(msg, "field ") || !strings.Contains(msg, " not found") {
			return nil, false
		}
		warnings = append(warnings, LintIssue{
			Path:     unknownFieldPath(msg),
			Severity: LintWarning,
			Message:  msg,
		})
	}
	return warnings, true
}

func yamlTypeError(err error) *yaml.TypeError {
	var pe *ParseError
	if errors.As(err, &pe) && pe.Inner != nil {
		err = pe.Inner
	}
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		return typeErr
	}
	return nil
}

func unknownFieldPath(msg string) string {
	field := ""
	if before, after, ok := strings.Cut(msg, "field "); ok {
		_ = before
		if value, _, ok := strings.Cut(after, " not found"); ok {
			field = strings.Trim(value, `"`)
		}
	}
	if field == "" {
		return "<root>"
	}
	return "<root>." + field
}

func appendIssuesFromError(issues []LintIssue, err error, severity LintSeverity) []LintIssue {
	if err == nil {
		return issues
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			issues = appendIssuesFromError(issues, child, severity)
		}
		return issues
	}

	var pe *ParseError
	if errors.As(err, &pe) {
		return append(issues, LintIssue{Path: pe.Path, Severity: severity, Message: pe.Message})
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		return append(issues, LintIssue{Path: ve.Path, Severity: severity, Message: fmt.Sprintf("[%s] %s", ve.Rule, ve.Message)})
	}
	return append(issues, LintIssue{Path: "<root>", Severity: severity, Message: err.Error()})
}

func sortLintIssues(issues []LintIssue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Path != issues[j].Path {
			return issues[i].Path < issues[j].Path
		}
		if issues[i].Severity != issues[j].Severity {
			return issues[i].Severity < issues[j].Severity
		}
		return issues[i].Message < issues[j].Message
	})
}
