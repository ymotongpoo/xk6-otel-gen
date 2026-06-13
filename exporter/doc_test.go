// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter_test

import (
	"context"
	"fmt"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

func ExampleNew() {
	cfg := exporter.Config{
		Endpoint:     "localhost:4317",
		Insecure:     true,
		Timeout:      10 * time.Millisecond,
		BatchSize:    1,
		BatchTimeout: time.Millisecond,
		MaxQueueSize: 1,
	}
	p, err := exporter.New(cfg)
	if err != nil {
		return
	}
	defer func() {
		_ = p.Shutdown(context.Background())
	}()

	_ = p.TracerProvider().Tracer("example")
	// Output:
}

func ExampleConfig_MergeWith() {
	base := exporter.Config{Endpoint: "default:4317", Timeout: 10 * time.Second}
	override := exporter.Config{Endpoint: "override:4317"}
	merged := base.MergeWith(override)
	fmt.Println(merged.Endpoint, merged.Timeout)
	// Output: override:4317 10s
}

func ExampleGetShared() {
	exporter.ResetShared()
	defer exporter.ResetShared()

	p, err := exporter.GetShared(func() (*exporter.Pipeline, error) {
		return exporter.New(exporter.Config{
			Endpoint:     "localhost:4317",
			Insecure:     true,
			Timeout:      10 * time.Millisecond,
			BatchSize:    1,
			BatchTimeout: time.Millisecond,
			MaxQueueSize: 1,
		})
	})
	if err != nil {
		return
	}
	defer func() {
		_ = p.Shutdown(context.Background())
	}()

	_ = p
	// Output:
}
