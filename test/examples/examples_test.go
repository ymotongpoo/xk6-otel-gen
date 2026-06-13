// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package examples_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestExamples_TopologyValidates(t *testing.T) {
	t.Parallel()

	examplesRoot := filepath.Join("..", "..", "examples")
	entries, err := os.ReadDir(examplesRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", examplesRoot, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			yamlPath := filepath.Join(examplesRoot, name, "topology.yaml")
			file, err := os.Open(yamlPath)
			if errors.Is(err, os.ErrNotExist) {
				t.Skipf("%s does not contain topology.yaml yet", name)
			}
			if err != nil {
				t.Fatalf("Open(%q): %v", yamlPath, err)
			}
			defer file.Close()

			schema, err := topology.Parse(file)
			if err != nil {
				t.Fatalf("Parse(%q): %v", yamlPath, err)
			}
			if err := topology.Validate(schema); err != nil {
				t.Fatalf("Validate(%q): %v", yamlPath, err)
			}
		})
	}
}
