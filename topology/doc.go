// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

// Package topology provides the schema, parser, validator, and serializer for
// declarative microservice topologies consumed by xk6-otel-gen.
//
// A topology YAML file declares services, their operations and outgoing calls,
// named journeys that enter those operations, and optional fault specifications
// targeting services, operations, or edges. Parse reads the YAML representation
// and returns a *Schema with all cross-references resolved to *Service,
// *Operation, and *Edge pointers.
//
// The package also provides validation, identifier-based equality, YAML
// marshaling for round-trips, fault-overlay construction, JSON Schema export,
// and linting helpers. It performs no logging and keeps all diagnostics in
// returned errors or lint issues.
//
// IMMUTABILITY: *Schema and all contained values are designed to be immutable
// after Parse returns successfully. Mutating any field, map, or slice after
// Parse returns leads to undefined behavior, including data races under
// concurrent reads and inconsistent derived overlays. Treat parsed schemas as
// read-only values.
//
// CONCURRENCY: A read-only *Schema is safe to share across goroutines. Multiple
// journey engines may read the same Schema concurrently without package-level
// locking. The package holds no global mutable state and all top-level
// operations are reentrant.
//
// ERROR REPORTING: Parse returns *ParseError for YAML and reference-resolution
// failures, and Validate returns *ValidationError values for structural and
// domain-rule failures. When multiple issues exist, errors are joined with
// errors.Join; use errors.As with *ParseError or *ValidationError to inspect
// individual issue types.
package topology
