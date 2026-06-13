// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

// Package journey orchestrates topology-defined journeys and delegates signal
// emission to a synth.Synthesizer.
//
// An Engine is constructed from a resolved topology.Schema, a FaultOverlay, and
// a Synthesizer. Construction eagerly builds immutable Plans for every journey
// so callers can reuse a Plan across many Execute calls and goroutines:
//
//	eng := journey.NewEngine(schema, schema.ApplyFaults(), syn)
//	plan, err := eng.BuildPlan("checkout")
//	err = eng.Execute(ctx, plan)
//
// Execution walks the Plan tree directly. Sequential children run in order,
// while virtual fan-out nodes run their branches concurrently with a
// sync.WaitGroup. Each concrete operation emits a span, metric, and log through
// the Synthesizer.
//
// Faults are folded from the topology FaultOverlay at execution time. Crash and
// disconnect faults create primary failures; error-rate overrides force
// probabilistic failures after the step's latency; latency inflation is added
// to the edge latency sample. Edge latency is sampled from the topology edge's
// LatencyDist, with a package-local default for journey entry nodes.
//
// Recovery is driven by Edge.OnFailure. Fallback edges are tried sequentially.
// If they are exhausted, the recovery policy either propagates the failure,
// returns a default success, or succeeds silently. When a failure propagates to
// children, those children do not execute real work, but their spans are still
// emitted with synth.cascaded=true for trace visibility.
package journey
