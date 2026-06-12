// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/log"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

// Outcome is the per-step execution result captured by the Engine and rendered
// into telemetry by the Synthesizer.
type Outcome struct {
	// Success reports whether the step completed without a primary failure.
	Success bool
	// Latency is the simulated duration spent on this step.
	Latency time.Duration
	// StatusCode is the HTTP/gRPC status code associated with the outcome.
	StatusCode int
	// ErrorType is the semantic error.type value, or empty on success.
	ErrorType string
	// Cascaded reports that the step was skipped because an upstream step failed.
	Cascaded bool
	// PrimaryFailed reports that the primary edge failed before recovery.
	PrimaryFailed bool
	// FallbackAttempts is the ordered list of failed fallback edges.
	FallbackAttempts []*topology.Edge
	// FallbackUsed is the fallback edge that ultimately succeeded, if any.
	FallbackUsed *topology.Edge
	// DefaultUsed reports that OnExhausted returned a default response.
	DefaultUsed bool
	// SilentlySucceeded reports that OnExhausted converted the failure to success.
	SilentlySucceeded bool
}

// Execute runs plan once, emitting synthetic telemetry through the Engine's
// Synthesizer. Step-level failures are represented as outcomes, not returned
// errors.
func (e *Engine) Execute(ctx context.Context, plan *Plan) (err error) {
	if ctx == nil {
		return &ExecuteError{Kind: "nil_ctx"}
	}
	if plan == nil {
		return &ExecuteError{Kind: "nil_plan"}
	}
	defer func() {
		if r := recover(); r != nil {
			err = &ExecuteError{Kind: "internal", Inner: fmt.Errorf("panic during Execute: %v", r)}
		}
	}()

	_ = e.impl.executeNode(ctx, plan.Root, nil)
	return nil
}

func (e *engineImpl) executeNode(ctx context.Context, node *Node, parent *Outcome) Outcome {
	return e.executeNodeAt(ctx, node, parent, time.Now())
}

func (e *engineImpl) executeNodeAt(ctx context.Context, node *Node, parent *Outcome, start time.Time) Outcome {
	if node == nil {
		return Outcome{Success: false, ErrorType: "crashed", StatusCode: 500}
	}
	if len(node.Parallel) > 0 {
		return e.executeParallelGroupAt(ctx, node, parent, start)
	}
	if node.Service == nil {
		return e.executeSequentialVirtualAt(ctx, node, parent, start)
	}
	if parent != nil && !parent.Success {
		return e.executeCascadeAt(ctx, node, parent, start)
	}
	return e.executeAttemptSequence(ctx, node, start)
}

func (e *engineImpl) executeAttemptSequence(ctx context.Context, node *Node, start time.Time) Outcome {
	attempts := 1
	if node.Edge != nil && node.Edge.Retries > 0 {
		attempts += node.Edge.Retries
	}

	currentStart := start
	var total time.Duration
	var final Outcome
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			delay := retryBackoffDelay(node.Edge, attempt)
			total += delay
			currentStart = start.Add(total)
		}

		outcome := e.executeSingleAttempt(ctx, node, currentStart)
		total += outcome.Latency
		if outcome.Success {
			outcome.Latency = total
			return outcome
		}
		final = outcome
		if outcome.ErrorType == "context_canceled" {
			break
		}
	}

	final.Latency = capLatencyToTimeout(node.Edge, total)
	if node.Edge != nil && node.Edge.Timeout > 0 && total > node.Edge.Timeout {
		applyTimeoutFailure(node.Edge, &final)
	}
	if node.Edge != nil && node.Edge.OnFailure != nil {
		final = e.applyRecoveryAt(ctx, node, final, start)
	}
	if shouldCascadeChildren(final) {
		e.executeChildrenAt(ctx, node, &final, start.Add(final.Latency))
	}
	return final
}

func (e *engineImpl) executeSingleAttempt(ctx context.Context, node *Node, start time.Time) Outcome {
	ff := e.foldFaults(node)
	instanceIdx := e.randIntN(node.Service.Replicas)
	baseLatency := e.sampleEdgeLatency(node.Edge)
	effectiveLatency := baseLatency + ff.latencyInflate
	spanCtx, finishFn := e.synth.BeginSpan(ctx, synth.SpanInput{
		Service:     node.Service,
		Edge:        node.Edge,
		Operation:   node.Operation,
		StartTime:   start,
		InstanceIdx: instanceIdx,
	})

	if ff.crashed {
		outcome := Outcome{
			Success:    false,
			StatusCode: 500,
			ErrorType:  "crashed",
			Latency:    0,
		}
		e.finishAndEmitAt(spanCtx, node, instanceIdx, finishFn, outcome, start)
		return outcome
	}

	if ctx.Err() != nil {
		outcome := Outcome{
			Success:    false,
			StatusCode: 0,
			ErrorType:  "context_canceled",
			Latency:    0,
		}
		e.finishAndEmitAt(spanCtx, node, instanceIdx, finishFn, outcome, start)
		return outcome
	}

	if node.Edge != nil && node.Edge.Timeout > 0 && effectiveLatency > node.Edge.Timeout {
		outcome := Outcome{
			Success: false,
			Latency: node.Edge.Timeout,
		}
		applyTimeoutFailure(node.Edge, &outcome)
		e.finishAndEmitAt(spanCtx, node, instanceIdx, finishFn, outcome, start.Add(outcome.Latency))
		return outcome
	}

	if ff.disconnected {
		outcome := Outcome{
			Success:    false,
			StatusCode: 503,
			ErrorType:  "connection_refused",
			Latency:    effectiveLatency,
		}
		e.finishAndEmitAt(spanCtx, node, instanceIdx, finishFn, outcome, start.Add(outcome.Latency))
		return outcome
	}

	forceFailure := ff.errorRate > 0 && e.randFloat64() < ff.errorRate
	childParent := &Outcome{Success: true, StatusCode: 200}
	totalLatency := effectiveLatency
	for _, child := range node.Children {
		childOutcome := e.executeNodeAt(spanCtx, child, childParent, start.Add(totalLatency))
		totalLatency += childOutcome.Latency
		if !childOutcome.Success {
			break
		}
	}

	outcome := Outcome{
		Success:    !forceFailure,
		StatusCode: pickStatusCode(forceFailure, ff.errorType),
		Latency:    totalLatency,
	}
	if forceFailure {
		outcome.ErrorType = ff.errorType
		if outcome.ErrorType == "" {
			outcome.ErrorType = "http.500"
		}
		e.finishAndEmitAt(spanCtx, node, instanceIdx, finishFn, outcome, start.Add(outcome.Latency))
		return outcome
	}

	e.finishAndEmitAt(spanCtx, node, instanceIdx, finishFn, outcome, start.Add(outcome.Latency))
	return outcome
}

func (e *engineImpl) executeChildrenAt(ctx context.Context, node *Node, parent *Outcome, start time.Time) {
	cascadeMode := parent != nil && !parent.Success
	currentStart := start
	for _, child := range node.Children {
		childOutcome := e.executeNodeAt(ctx, child, parent, currentStart)
		currentStart = currentStart.Add(childOutcome.Latency)
		if !childOutcome.Success && !cascadeMode {
			return
		}
	}
}

func (e *engineImpl) executeCascadeAt(ctx context.Context, node *Node, parent *Outcome, start time.Time) Outcome {
	instanceIdx := e.randIntN(node.Service.Replicas)
	spanCtx, finishFn := e.synth.BeginSpan(ctx, synth.SpanInput{
		Service:     node.Service,
		Edge:        node.Edge,
		Operation:   node.Operation,
		StartTime:   start,
		InstanceIdx: instanceIdx,
	})
	outcome := Outcome{
		Success:    false,
		StatusCode: parent.StatusCode,
		ErrorType:  parent.ErrorType,
		Cascaded:   true,
		Latency:    0,
	}
	e.finishAndEmitAt(spanCtx, node, instanceIdx, finishFn, outcome, start)
	return outcome
}

func (e *engineImpl) executeSequentialVirtualAt(ctx context.Context, node *Node, parent *Outcome, start time.Time) Outcome {
	outcome := Outcome{Success: true, StatusCode: 200}
	currentParent := parent
	var total time.Duration
	for _, child := range node.Children {
		childOutcome := e.executeNodeAt(ctx, child, currentParent, start.Add(total))
		total += childOutcome.Latency
		outcome = childOutcome
		if !childOutcome.Success {
			currentParent = &childOutcome
		}
	}
	outcome.Latency = total
	return outcome
}

func (e *engineImpl) executeParallelGroupAt(ctx context.Context, group *Node, parent *Outcome, start time.Time) Outcome {
	outcomes := make([]Outcome, len(group.Parallel))
	var wg sync.WaitGroup
	for i, child := range group.Parallel {
		i, child := i, child
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					outcomes[i] = Outcome{Success: false, ErrorType: "crashed", StatusCode: 500}
				}
			}()
			outcomes[i] = e.executeNodeAt(ctx, child, parent, start)
		}()
	}
	wg.Wait()
	return aggregateParallelOutcomes(outcomes)
}

func (e *engineImpl) finishAndEmitAt(ctx context.Context, node *Node, instanceIdx int, finishFn synth.FinishSpanFunc, outcome Outcome, end time.Time) {
	finishFn(toSynthOutcome(outcome, end))
	synthOutcome := toSynthOutcome(outcome, end)
	e.synth.RecordMetric(ctx, synth.MetricInput{
		Service:     node.Service,
		Edge:        node.Edge,
		Operation:   node.Operation,
		Latency:     outcome.Latency,
		Outcome:     synthOutcome,
		InstanceIdx: instanceIdx,
	})
	e.synth.EmitLog(ctx, synth.LogInput{
		Service:   node.Service,
		Severity:  logSeverity(outcome),
		Body:      node.Operation + " " + outcomeLabel(outcome),
		Timestamp: end,
		Attributes: map[string]any{
			"outcome":    outcomeLabel(outcome),
			"error.type": outcome.ErrorType,
		},
	})
}

func pickStatusCode(failed bool, errorType string) int {
	if !failed {
		return 200
	}
	if strings.HasPrefix(errorType, "http.") {
		code, err := strconv.Atoi(strings.TrimPrefix(errorType, "http."))
		if err == nil {
			return code
		}
	}
	switch errorType {
	case "grpc.unavailable":
		return 14
	case "grpc.deadline_exceeded":
		return 4
	case "grpc.unauthenticated":
		return 16
	case "context_canceled":
		return 0
	default:
		return 500
	}
}

func toSynthOutcome(outcome Outcome, end time.Time) synth.Outcome {
	return synth.Outcome{
		Success:    outcome.Success,
		StatusCode: outcome.StatusCode,
		ErrorType:  outcome.ErrorType,
		EndTime:    end,
		Cascaded:   outcome.Cascaded,
	}
}

func aggregateParallelOutcomes(outcomes []Outcome) Outcome {
	agg := Outcome{Success: true, StatusCode: 200}
	for _, outcome := range outcomes {
		if outcome.Latency > agg.Latency {
			agg.Latency = outcome.Latency
		}
		if !outcome.Success && agg.Success {
			agg.Success = false
			agg.StatusCode = outcome.StatusCode
			agg.ErrorType = outcome.ErrorType
			agg.Cascaded = outcome.Cascaded
		}
	}
	return agg
}

func retryBackoffDelay(edge *topology.Edge, retryAttempt int) time.Duration {
	if edge == nil || retryAttempt <= 0 {
		return 0
	}
	base := edge.RetryBaseDelay
	if base == 0 {
		base = topology.DefaultRetryBaseDelay
	}
	switch edge.RetryBackoff {
	case topology.BackoffConstant:
		return base
	case topology.BackoffLinear:
		return time.Duration(retryAttempt) * base
	case topology.BackoffExponential:
		fallthrough
	default:
		return time.Duration(1<<(retryAttempt-1)) * base
	}
}

func capLatencyToTimeout(edge *topology.Edge, latency time.Duration) time.Duration {
	if edge != nil && edge.Timeout > 0 && latency > edge.Timeout {
		return edge.Timeout
	}
	return latency
}

func applyTimeoutFailure(edge *topology.Edge, outcome *Outcome) {
	if outcome == nil {
		return
	}
	outcome.Success = false
	switch {
	case edge != nil && edge.Protocol == topology.ProtocolGRPC:
		outcome.StatusCode = 4
		outcome.ErrorType = "grpc.deadline_exceeded"
	default:
		outcome.StatusCode = 504
		outcome.ErrorType = "timeout"
	}
}

func shouldCascadeChildren(outcome Outcome) bool {
	if outcome.Success {
		return false
	}
	switch outcome.ErrorType {
	case "crashed", "connection_refused", "context_canceled", "timeout", "grpc.deadline_exceeded":
		return true
	default:
		return false
	}
}

func logSeverity(outcome Outcome) log.Severity {
	if outcome.Success {
		return log.SeverityInfo
	}
	return log.SeverityError
}

func outcomeLabel(outcome Outcome) string {
	if outcome.Success {
		return "success"
	}
	return "failure"
}
