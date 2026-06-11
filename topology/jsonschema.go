// SPDX-License-Identifier: Apache-2.0

package topology

import _ "embed"

//go:embed jsonschema/topology.schema.json
var jsonSchemaTemplate []byte

// ExportJSONSchema returns the embedded JSON Schema document.
func (s *Schema) ExportJSONSchema() ([]byte, error) {
	out := make([]byte, len(jsonSchemaTemplate))
	copy(out, jsonSchemaTemplate)
	return out, nil
}
