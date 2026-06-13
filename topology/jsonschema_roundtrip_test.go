// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"gopkg.in/yaml.v3"
	"pgregory.net/rapid"
)

func TestExportJSONSchema_RoundTrip(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := generators.ValidSchema().Draw(t, "schema")
		schemaBytes, err := s.ExportJSONSchema()
		if err != nil {
			t.Fatalf("ExportJSONSchema() error = %v", err)
		}
		yamlBytes, err := yaml.Marshal(s)
		if err != nil {
			t.Fatalf("yaml.Marshal() error = %v", err)
		}
		jsonBytes, err := yamlToJSON(yamlBytes)
		if err != nil {
			t.Fatalf("yamlToJSON() error = %v", err)
		}
		compiled, err := jsonschema.CompileString("topology.json", string(schemaBytes))
		if err != nil {
			t.Fatalf("CompileString() error = %v", err)
		}
		var doc any
		dec := json.NewDecoder(bytes.NewReader(jsonBytes))
		dec.UseNumber()
		if err := dec.Decode(&doc); err != nil {
			t.Fatalf("json.Decode() error = %v", err)
		}
		if err := compiled.Validate(doc); err != nil {
			t.Fatalf("schema validation error = %v\nYAML:\n%s\nJSON:\n%s", err, yamlBytes, jsonBytes)
		}
	})
}

func yamlToJSON(yamlBytes []byte) ([]byte, error) {
	var doc any
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return nil, err
	}
	return json.Marshal(toJSONValue(doc))
}

func toJSONValue(v any) any {
	switch value := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for k, child := range value {
			out[k] = toJSONValue(child)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(value))
		for k, child := range value {
			out[fmt.Sprint(k)] = toJSONValue(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(value))
		for _, child := range value {
			out = append(out, toJSONValue(child))
		}
		return out
	default:
		return value
	}
}
