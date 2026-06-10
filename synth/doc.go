// Package synth synthesizes OpenTelemetry spans, metrics, and log records for
// simulated services described by the topology package.
//
// A journey engine drives this package by passing SpanInput, MetricInput, and
// LogInput values for each simulated operation. NewDefault binds the
// synthesizer to caller-provided OpenTelemetry tracer, meter, and logger
// providers. The caller owns provider lifecycle and should flush or shut them
// down through the exporter pipeline after the journey completes.
//
// The normal span lifecycle is:
//
//	syn := synth.NewDefault(tp, mp, lp)
//	ctx, finish := syn.BeginSpan(ctx, synth.SpanInput{...})
//	// execute child journey work with ctx
//	finish(synth.Outcome{Success: true, EndTime: time.Now()})
//
// BuildResource builds per-service-instance resources with deterministic
// service.instance.id values. Signal attributes emitted by this package use
// OpenTelemetry Semantic Conventions v1.27.0 plus the documented
// synth.service.framework custom attribute for synthesized framework names.
package synth
