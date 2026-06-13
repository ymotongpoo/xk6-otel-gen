// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

// Package generators provides domain-specific PBT (property-based testing)
// generators for the xk6-otel-gen project, built on pgregory.net/rapid.
//
// All public generators return *rapid.Generator[T] and must be drawn within
// a rapid.Check or rapid.MakeCheck call. See pgregory.net/rapid documentation
// for usage details.
//
// Generator naming:
//   - Valid<TypeName>() -- guaranteed to produce values that satisfy domain
//     invariants.
//   - Any<TypeName>() -- may produce structurally degenerate values, useful
//     for testing validation logic.
//
// Generators are composable via functional options such as MaxServices(10).
// Both Valid and Any flavors share atomic primitives where possible.
//
// PBT compliance: This package satisfies PBT-07 (Generator Quality) and
// PBT-09 (Framework Selection) per the project's AI-DLC PBT extension.
//
// See also:
//   - aidlc-docs/construction/u7-testutil/functional-design/ for design rationale.
//   - aidlc-docs/construction/u7-testutil/nfr-design/ for performance and patterns.
package generators
