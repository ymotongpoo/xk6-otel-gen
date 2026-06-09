package topology

import "fmt"

// ParseError describes a YAML decoding or reference-resolution error.
type ParseError struct {
	Path    string
	Message string
	Inner   error
}

// Error returns a path-qualified parse error message.
func (e *ParseError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Inner != nil {
		return fmt.Sprintf("topology: %s: %s: %v", e.Path, e.Message, e.Inner)
	}
	return fmt.Sprintf("topology: %s: %s", e.Path, e.Message)
}

// Unwrap returns the underlying error, if any.
func (e *ParseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}

// ValidationError describes a structural or domain-rule validation failure.
type ValidationError struct {
	Path    string
	Rule    string
	Message string
}

// Error returns a path- and rule-qualified validation error message.
func (e *ValidationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("topology: %s: [%s] %s", e.Path, e.Rule, e.Message)
}

func newParseError(path, message string) *ParseError {
	return &ParseError{Path: path, Message: message}
}

func newParseErrorf(path, format string, args ...any) *ParseError {
	return &ParseError{Path: path, Message: fmt.Sprintf(format, args...)}
}

func newValidationError(path, rule, message string) *ValidationError {
	return &ValidationError{Path: path, Rule: rule, Message: message}
}

func newValidationErrorf(path, rule, format string, args ...any) *ValidationError {
	return &ValidationError{Path: path, Rule: rule, Message: fmt.Sprintf(format, args...)}
}
