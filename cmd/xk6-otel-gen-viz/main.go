// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

// Command xk6-otel-gen-viz generates a self-contained interactive HTML
// visualization of a topology DAG from a YAML file.
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
	fs := flag.NewFlagSet("xk6-otel-gen-viz", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "input topology YAML file path (required)")
	output := fs.String("output", "", "output HTML file path (default stdout)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *input == "" {
		fmt.Fprintln(stderr, "xk6-otel-gen-viz: -input flag is required")
		return 2
	}

	schema, err := topology.ParseFile(*input)
	if err != nil {
		fmt.Fprintf(stderr, "xk6-otel-gen-viz: parse %q: %v\n", *input, err)
		return 1
	}

	data := buildVizData(schema)

	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintf(stderr, "xk6-otel-gen-viz: create %q: %v\n", *output, err)
			return 1
		}
		defer f.Close()
		if err := generateHTML(data, f); err != nil {
			fmt.Fprintf(stderr, "xk6-otel-gen-viz: generate HTML: %v\n", err)
			return 1
		}
		return 0
	}

	if err := generateHTML(data, stdout); err != nil {
		fmt.Fprintf(stderr, "xk6-otel-gen-viz: write stdout: %v\n", err)
		return 1
	}
	return 0
}
