package topology

import "io"

// AUTOGEN-MARKER-U1: These panic stubs were scaffolded during U7
// (testutil/generators) Code Generation and are deferred to U1.

// Parse decodes a topology YAML from r.
// AUTOGEN-MARKER-U1: implementation deferred to U1 Code Generation.
func Parse(r io.Reader) (*Schema, error) {
	panic("topology.Parse: not yet implemented (U1 deferred)")
}

// ParseFile is a thin wrapper around Parse for filesystem paths.
// AUTOGEN-MARKER-U1: implementation deferred to U1 Code Generation.
func ParseFile(path string) (*Schema, error) {
	panic("topology.ParseFile: not yet implemented (U1 deferred)")
}

// Validate checks structural invariants of the Schema.
// AUTOGEN-MARKER-U1: implementation deferred to U1 Code Generation.
func Validate(s *Schema) error {
	panic("topology.Validate: not yet implemented (U1 deferred)")
}

// Equal compares two schemas by identifier-based deep equality.
// NOTE: U7 needs a minimal Equal for PBT round-trip checks. This skeleton
// provides a best-effort implementation that may be replaced by U1.
func Equal(a, b *Schema) bool {
	// TODO(u1): implement identifier-based deep comparison.
	// For U7's purposes, this is unused; the round-trip property is checked
	// by U1's own tests once Parse/MarshalYAML exist.
	return a == b
}

// FindServiceByName returns the service with the given identifier.
// AUTOGEN-MARKER-U1: implementation deferred to U1 Code Generation.
func (s *Schema) FindServiceByName(id ServiceID) (*Service, bool) {
	panic("topology.Schema.FindServiceByName: not yet implemented (U1 deferred)")
}

// JourneyNames returns the available journey names.
// AUTOGEN-MARKER-U1: implementation deferred to U1 Code Generation.
func (s *Schema) JourneyNames() []string {
	panic("topology.Schema.JourneyNames: not yet implemented (U1 deferred)")
}

// ApplyFaults computes a fault overlay for the schema.
// AUTOGEN-MARKER-U1: implementation deferred to U1 Code Generation.
func (s *Schema) ApplyFaults() *FaultOverlay {
	panic("topology.Schema.ApplyFaults: not yet implemented (U1 deferred)")
}

// ExportJSONSchema returns the JSON Schema for topology YAML.
// AUTOGEN-MARKER-U1: implementation deferred to U1 Code Generation.
func (s *Schema) ExportJSONSchema() ([]byte, error) {
	panic("topology.Schema.ExportJSONSchema: not yet implemented (U1 deferred)")
}

// MarshalYAML serializes Schema back to YAML by identifier.
// AUTOGEN-MARKER-U1: implementation deferred to U1 Code Generation.
func (s *Schema) MarshalYAML() (any, error) {
	panic("topology.Schema.MarshalYAML: not yet implemented (U1 deferred)")
}
