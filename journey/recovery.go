// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"context"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func (e *engineImpl) applyRecovery(ctx context.Context, node *Node, primary Outcome) Outcome {
	return e.applyRecoveryAt(ctx, node, primary, time.Now())
}

func (e *engineImpl) applyRecoveryAt(ctx context.Context, node *Node, primary Outcome, start time.Time) Outcome {
	out := primary
	out.PrimaryFailed = true
	policy := node.Edge.OnFailure
	if policy == nil {
		return out
	}

	for _, fallback := range policy.Fallback {
		fbOutcome := e.executeFallbackAt(ctx, node, fallback, start.Add(out.Latency))
		out.Latency += fbOutcome.Latency
		if fbOutcome.Success {
			out.Success = true
			out.StatusCode = fbOutcome.StatusCode
			out.ErrorType = ""
			out.FallbackUsed = fallback
			return out
		}
		out.FallbackAttempts = append(out.FallbackAttempts, fallback)
	}

	switch policy.OnExhausted {
	case topology.ExhaustedReturnDefault:
		out.Success = true
		out.StatusCode = 200
		out.ErrorType = ""
		out.DefaultUsed = true
	case topology.ExhaustedSucceedSilently:
		out.Success = true
		out.StatusCode = 200
		out.ErrorType = ""
		out.SilentlySucceeded = true
	default:
		out.Success = false
	}
	return out
}

func (e *engineImpl) executeFallbackAt(ctx context.Context, _ *Node, fbEdge *topology.Edge, start time.Time) Outcome {
	if fbEdge == nil || fbEdge.To == nil || fbEdge.To.Service == nil {
		return Outcome{Success: false, StatusCode: 503, ErrorType: "connection_refused"}
	}
	fbNode := &Node{
		Service:   fbEdge.To.Service,
		Operation: fbEdge.To.Name,
		Edge:      fbEdge,
	}
	return e.executeNodeAt(ctx, fbNode, nil, start)
}
