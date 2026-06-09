package generators

import (
	"math"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

// LatencyPair is a helper for p50/p95 latency durations.
type LatencyPair struct {
	P50 time.Duration
	P95 time.Duration
}

// ValidServiceID returns a ServiceID matching business-rules.md §3.
func ValidServiceID() *rapid.Generator[topology.ServiceID] {
	return rapid.Custom(func(t *rapid.T) topology.ServiceID {
		return topology.ServiceID(rapid.StringMatching(`^[a-z][a-z0-9-]{2,30}$`).Draw(t, "service_id"))
	})
}

// AnyServiceID returns a ServiceID that may violate business-rules.md §3.
func AnyServiceID() *rapid.Generator[topology.ServiceID] {
	return rapid.Custom(func(t *rapid.T) topology.ServiceID {
		return topology.ServiceID(rapid.OneOf(
			rapid.String(),
			rapid.Just(""),
			rapid.Just("UPPERCASE"),
			rapid.Just("-bad-prefix"),
			rapid.Just("service name with spaces"),
			rapid.StringMatching(`^[a-z][a-z0-9-]{31,60}$`),
		).Draw(t, "any_service_id"))
	})
}

// ValidOperationName returns an operation name in the range from business-rules.md §3.
func ValidOperationName() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		return rapid.OneOf(
			rapid.StringMatching(`^[A-Za-z][A-Za-z0-9_-]{0,40}$`),
			rapid.StringMatching(`^(GET|POST|PUT|DELETE|PATCH) /[a-z][a-z0-9/-]{0,60}(\{[a-z][a-z0-9_-]{0,20}\})?$`),
			rapid.StringMatching(`^[A-Za-z][A-Za-z0-9]+/[A-Za-z][A-Za-z0-9]{0,40}$`),
		).Draw(t, "operation_name")
	})
}

// AnyOperationName returns an operation name that may be empty or over length.
func AnyOperationName() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		return rapid.OneOf(
			rapid.String(),
			rapid.Just(""),
			rapid.StringN(121, 180, -1),
		).Draw(t, "any_operation_name")
	})
}

// ValidProbability returns a float64 in [0, 1]. See business-rules.md §3.
func ValidProbability() *rapid.Generator[float64] {
	return rapid.Custom(func(t *rapid.T) float64 {
		return rapid.Float64Range(0, 1).Draw(t, "probability")
	})
}

// AnyProbability returns a float64 that may be outside [0, 1] or non-finite.
func AnyProbability() *rapid.Generator[float64] {
	return rapid.Custom(func(t *rapid.T) float64 {
		return rapid.OneOf(
			rapid.Float64Range(-1, 2),
			rapid.Just(math.NaN()),
			rapid.Just(math.Inf(1)),
		).Draw(t, "any_probability")
	})
}

// ValidReplicaCount returns an int in [1, 100]. See business-rules.md §3.
func ValidReplicaCount() *rapid.Generator[int] {
	return rapid.Custom(func(t *rapid.T) int {
		return rapid.IntRange(1, 100).Draw(t, "replicas")
	})
}

// AnyReplicaCount returns an int that may be zero, negative, or very large.
func AnyReplicaCount() *rapid.Generator[int] {
	return rapid.Custom(func(t *rapid.T) int {
		return rapid.IntRange(-10, 1000).Draw(t, "any_replicas")
	})
}

// ValidLatencyPair returns p50/p95 where p95 >= p50. See business-rules.md §3.
func ValidLatencyPair() *rapid.Generator[LatencyPair] {
	return rapid.Custom(func(t *rapid.T) LatencyPair {
		p50ms := rapid.IntRange(1, 5_000).Draw(t, "p50_ms")
		p95ms := rapid.IntRange(p50ms, 30_000).Draw(t, "p95_ms")
		return LatencyPair{
			P50: time.Duration(p50ms) * time.Millisecond,
			P95: time.Duration(p95ms) * time.Millisecond,
		}
	})
}

// AnyLatencyPair may produce negative durations or p95 < p50.
func AnyLatencyPair() *rapid.Generator[LatencyPair] {
	return rapid.Custom(func(t *rapid.T) LatencyPair {
		p50ms := rapid.IntRange(-1_000, 5_000).Draw(t, "any_p50_ms")
		p95ms := rapid.IntRange(-1_000, 30_000).Draw(t, "any_p95_ms")
		return LatencyPair{
			P50: time.Duration(p50ms) * time.Millisecond,
			P95: time.Duration(p95ms) * time.Millisecond,
		}
	})
}

// ValidTimeout returns a duration in [100ms, 60s]. See business-rules.md §3.
func ValidTimeout() *rapid.Generator[time.Duration] {
	return rapid.Custom(func(t *rapid.T) time.Duration {
		return time.Duration(rapid.IntRange(100, 60_000).Draw(t, "timeout_ms")) * time.Millisecond
	})
}

// AnyTimeout returns a duration that may be zero or negative.
func AnyTimeout() *rapid.Generator[time.Duration] {
	return rapid.Custom(func(t *rapid.T) time.Duration {
		return time.Duration(rapid.IntRange(-10_000, 120_000).Draw(t, "any_timeout_ms")) * time.Millisecond
	})
}

// ValidServiceKind returns a generator for R-DOM-3 service kinds.
func ValidServiceKind() *rapid.Generator[topology.ServiceKind] {
	return rapid.Custom(func(t *rapid.T) topology.ServiceKind {
		return rapid.SampledFrom([]topology.ServiceKind{
			topology.KindApplication,
			topology.KindDatabase,
			topology.KindExternalAPI,
			topology.KindCache,
			topology.KindQueue,
		}).Draw(t, "service_kind")
	})
}

// ValidProtocol returns a generator for R-DOM-4 protocols.
func ValidProtocol() *rapid.Generator[topology.Protocol] {
	return rapid.Custom(func(t *rapid.T) topology.Protocol {
		return rapid.SampledFrom([]topology.Protocol{
			topology.ProtocolHTTP,
			topology.ProtocolGRPC,
			topology.ProtocolMessaging,
		}).Draw(t, "protocol")
	})
}

// ValidErrorRate is an alias for ValidProbability (edge error_rate ∈ [0, 1]).
func ValidErrorRate() *rapid.Generator[float64] {
	return ValidProbability()
}

// AnyErrorRate is an alias for AnyProbability.
func AnyErrorRate() *rapid.Generator[float64] {
	return AnyProbability()
}

// ValidTimeoutDuration is an alias for ValidTimeout.
func ValidTimeoutDuration() *rapid.Generator[time.Duration] {
	return ValidTimeout()
}

// AnyTimeoutDuration is an alias for AnyTimeout.
func AnyTimeoutDuration() *rapid.Generator[time.Duration] {
	return AnyTimeout()
}
