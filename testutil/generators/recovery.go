// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"fmt"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

// RecoveryPolicyOption mutates recovery-policy generation parameters.
type RecoveryPolicyOption func(*recoveryPolicyOptions)

type recoveryPolicyOptions struct {
	maxFallbacks int
	onExhausted  *topology.ExhaustedAction
}

func defaultRecoveryPolicyOptions() recoveryPolicyOptions {
	return recoveryPolicyOptions{
		maxFallbacks: 3,
	}
}

func applyRecoveryPolicyOptions(opts []RecoveryPolicyOption) recoveryPolicyOptions {
	o := defaultRecoveryPolicyOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// MaxFallbacks caps the number of fallback edges in a recovery policy.
func MaxFallbacks(n int) RecoveryPolicyOption {
	return func(o *recoveryPolicyOptions) {
		o.maxFallbacks = clampInt(n, 1, n)
	}
}

// WithOnExhausted fixes the exhausted action for a recovery policy.
func WithOnExhausted(action topology.ExhaustedAction) RecoveryPolicyOption {
	return func(o *recoveryPolicyOptions) {
		o.onExhausted = &action
	}
}

// ValidRecoveryPolicy returns a recovery policy whose fallback edges are owned by from.
func ValidRecoveryPolicy(from *topology.Operation, fallbackTargets []*topology.Operation, opts ...RecoveryPolicyOption) *rapid.Generator[*topology.RecoveryPolicy] {
	o := applyRecoveryPolicyOptions(opts)
	return rapid.Custom(func(t *rapid.T) *topology.RecoveryPolicy {
		if len(fallbackTargets) == 0 {
			fallbackTargets = []*topology.Operation{from}
		}
		chainLen := rapid.IntRange(1, min(o.maxFallbacks, len(fallbackTargets))).Draw(t, "chain_len")
		fallbacks := make([]*topology.Edge, 0, chainLen)
		for i := 0; i < chainLen; i++ {
			to := fallbackTargets[i%len(fallbackTargets)]
			fallbacks = append(fallbacks, ValidEdge(from, to, WithoutRecovery()).Draw(t, fmt.Sprintf("fallback_%d", i)))
		}

		onExhausted := validExhaustedAction(t, "on_exhausted")
		if o.onExhausted != nil {
			onExhausted = *o.onExhausted
		}

		policy := &topology.RecoveryPolicy{
			Fallback:    fallbacks,
			OnExhausted: onExhausted,
		}
		if onExhausted == topology.ExhaustedReturnDefault || rapid.Float64Range(0, 1).Draw(t, "default_response_roll") < 0.5 {
			policy.DefaultResponse = map[string]any{
				"status": "default",
			}
		}
		return policy
	})
}

// AnyRecoveryPolicy returns a recovery policy that may violate ownership or enum invariants.
func AnyRecoveryPolicy(from *topology.Operation, fallbackTargets []*topology.Operation, opts ...RecoveryPolicyOption) *rapid.Generator[*topology.RecoveryPolicy] {
	return rapid.Custom(func(t *rapid.T) *topology.RecoveryPolicy {
		policy := ValidRecoveryPolicy(from, fallbackTargets, opts...).Draw(t, "valid_recovery")
		switch rapid.IntRange(0, 3).Draw(t, "recovery_mutation") {
		case 0:
			policy.Fallback = nil
		case 1:
			if len(policy.Fallback) > 0 {
				policy.Fallback[0].From = &topology.Operation{Name: "stale-owner", Service: &topology.Service{Name: "stale"}}
			}
		case 2:
			policy.OnExhausted = topology.ExhaustedAction(rapid.IntRange(10, 20).Draw(t, "invalid_exhausted"))
		case 3:
			policy.DefaultResponse = nil
		}
		return policy
	})
}
