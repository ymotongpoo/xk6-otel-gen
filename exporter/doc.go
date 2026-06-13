// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

// Package exporter provides an OTLP exporter pipeline for traces, metrics, and
// logs.
//
// A Pipeline owns one TracerProvider, one MeterProvider, and one LoggerProvider
// that share the same endpoint, headers, timeout, compression setting, and OTel
// resource. The package is intended for k6 extension code that needs to create
// synthetic OpenTelemetry signals in-process and send them to a Collector over
// OTLP/gRPC or OTLP/HTTP.
//
// Typical lifecycle:
//
//	cfg := exporter.Config{
//		Endpoint: "localhost:4317",
//		Insecure: true,
//	}
//	p, err := exporter.New(cfg)
//	if err != nil {
//		return err
//	}
//	defer p.Shutdown(context.Background())
//
//	tracer := p.TracerProvider().Tracer("xk6-otel-gen")
//	_, span := tracer.Start(ctx, "synthetic-operation")
//	span.End()
//
// Configuration is usually assembled from four layers, highest priority first:
// direct JS API options, ConfigFromEnv values from OTEL_EXPORTER_OTLP_*,
// topology YAML defaults supplied by higher-level packages, and built-in
// defaults. MergeWith applies this layering by letting non-zero override fields
// replace lower-priority fields.
//
// GetShared stores one process-wide Pipeline for k6 lifecycle integration. It
// caches both the first successful Pipeline and the first initialization error;
// ResetShared exists only to isolate tests that touch the shared holder.
package exporter
