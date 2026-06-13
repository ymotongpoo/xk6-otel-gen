// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

// Command xk6-otel-gen-schema exports the topology JSON Schema for editor
// integration and CI validation.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("xk6-otel-gen-schema", flag.ContinueOnError)
	fs.SetOutput(stderr)
	output := fs.String("output", "", "output file path (default stdout)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	var schema topology.Schema
	schemaBytes, err := schema.ExportJSONSchema()
	if err != nil {
		fmt.Fprintf(stderr, "xk6-otel-gen-schema: export JSON Schema: %v\n", err)
		return 1
	}

	if *output != "" {
		if err := os.WriteFile(*output, schemaBytes, 0o644); err != nil {
			fmt.Fprintf(stderr, "xk6-otel-gen-schema: write %q: %v\n", *output, err)
			return 1
		}
		return 0
	}

	if _, err := stdout.Write(schemaBytes); err != nil {
		fmt.Fprintf(stderr, "xk6-otel-gen-schema: write stdout: %v\n", err)
		return 1
	}
	return 0
}
