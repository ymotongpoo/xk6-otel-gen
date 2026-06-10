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
	// Latency is the elapsed wall-clock duration spent on this step.
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
	if node == nil {
		return Outcome{Success: false, ErrorType: "crashed", StatusCode: 500}
	}
	if len(node.Parallel) > 0 {
		return e.executeParallelGroup(ctx, node, parent)
	}
	if node.Service == nil {
		return e.executeSequentialVirtual(ctx, node, parent)
	}
	if parent != nil && !parent.Success {
		return Outcome{Success: false, Cascaded: true, ErrorType: parent.ErrorType, StatusCode: parent.StatusCode}
	}

	ff := e.foldFaults(node)
	instanceIdx := e.randIntN(node.Service.Replicas)
	baseLatency := e.sampleEdgeLatency(node.Edge)
	effectiveLatency := baseLatency + ff.latencyInflate
	start := time.Now()
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
			Latency:    time.Since(start),
		}
		e.finishAndEmit(spanCtx, node, instanceIdx, finishFn, outcome)
		return outcome
	}

	if canceled, end := waitWithCancel(ctx, effectiveLatency); canceled {
		outcome := Outcome{
			Success:    false,
			StatusCode: 0,
			ErrorType:  "context_canceled",
			Latency:    end.Sub(start),
		}
		e.finishAndEmit(spanCtx, node, instanceIdx, finishFn, outcome)
		return outcome
	}

	if ff.disconnected {
		outcome := Outcome{
			Success:    false,
			StatusCode: 503,
			ErrorType:  "connection_refused",
			Latency:    time.Since(start),
		}
		e.finishAndEmit(spanCtx, node, instanceIdx, finishFn, outcome)
		return outcome
	}

	forceFailure := ff.errorRate > 0 && e.randFloat64() < ff.errorRate
	childParent := &Outcome{Success: true, StatusCode: 200}
	for _, child := range node.Children {
		childOutcome := e.executeNode(spanCtx, child, childParent)
		if !childOutcome.Success {
			childParent = &childOutcome
			break
		}
	}

	outcome := Outcome{
		Success:    !forceFailure,
		StatusCode: pickStatusCode(forceFailure, ff.errorType),
		Latency:    time.Since(start),
	}
	if forceFailure {
		outcome.ErrorType = ff.errorType
		if outcome.ErrorType == "" {
			outcome.ErrorType = "http.500"
		}
	}

	e.finishAndEmit(spanCtx, node, instanceIdx, finishFn, outcome)
	return outcome
}

func (e *engineImpl) executeSequentialVirtual(ctx context.Context, node *Node, parent *Outcome) Outcome {
	outcome := Outcome{Success: true, StatusCode: 200}
	currentParent := parent
	var total time.Duration
	for _, child := range node.Children {
		childOutcome := e.executeNode(ctx, child, currentParent)
		total += childOutcome.Latency
		outcome = childOutcome
		if !childOutcome.Success {
			currentParent = &childOutcome
		}
	}
	outcome.Latency = total
	return outcome
}

func (e *engineImpl) executeParallelGroup(ctx context.Context, group *Node, parent *Outcome) Outcome {
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
			outcomes[i] = e.executeNode(ctx, child, parent)
		}()
	}
	wg.Wait()
	return aggregateParallelOutcomes(outcomes)
}

func (e *engineImpl) finishAndEmit(ctx context.Context, node *Node, instanceIdx int, finishFn synth.FinishSpanFunc, outcome Outcome) {
	now := time.Now()
	finishFn(toSynthOutcome(outcome, now))
	synthOutcome := toSynthOutcome(outcome, now)
	e.synth.RecordMetric(ctx, synth.MetricInput{
		Service:     node.Service,
		Edge:        node.Edge,
		Operation:   node.Operation,
		Latency:     outcome.Latency,
		Outcome:     synthOutcome,
		InstanceIdx: instanceIdx,
	})
	e.synth.EmitLog(ctx, synth.LogInput{
		Service:  node.Service,
		Severity: logSeverity(outcome),
		Body:     node.Operation + " " + outcomeLabel(outcome),
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
	}
}

func waitWithCancel(ctx context.Context, latency time.Duration) (bool, time.Time) {
	if latency <= 0 {
		select {
		case <-ctx.Done():
			return true, time.Now()
		default:
			return false, time.Now()
		}
	}

	timer := time.NewTimer(latency)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return true, time.Now()
	case <-timer.C:
		return false, time.Now()
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
