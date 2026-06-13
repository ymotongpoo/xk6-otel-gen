// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package topology

// ServiceKind identifies the semantic category of a service.
type ServiceKind int

const (
	// KindApplication represents an application service.
	KindApplication ServiceKind = iota
	// KindDatabase represents a database service.
	KindDatabase
	// KindExternalAPI represents an external API dependency.
	KindExternalAPI
	// KindCache represents a cache service.
	KindCache
	// KindQueue represents a queue or message broker service.
	KindQueue
)

// String returns the topology YAML token for k.
func (k ServiceKind) String() string {
	switch k {
	case KindApplication:
		return "application"
	case KindDatabase:
		return "database"
	case KindExternalAPI:
		return "external_api"
	case KindCache:
		return "cache"
	case KindQueue:
		return "queue"
	default:
		return "unknown"
	}
}

// Protocol identifies the transport protocol for an edge.
type Protocol int

const (
	// ProtocolHTTP represents HTTP calls.
	ProtocolHTTP Protocol = iota
	// ProtocolGRPC represents gRPC calls.
	ProtocolGRPC
	// ProtocolMessaging represents asynchronous messaging.
	ProtocolMessaging
)

// String returns the topology YAML token for p.
func (p Protocol) String() string {
	switch p {
	case ProtocolHTTP:
		return "http"
	case ProtocolGRPC:
		return "grpc"
	case ProtocolMessaging:
		return "messaging"
	default:
		return "unknown"
	}
}

// ExhaustedAction defines recovery behavior after all fallbacks fail.
type ExhaustedAction int

const (
	// ExhaustedPropagate propagates the failure downstream.
	ExhaustedPropagate ExhaustedAction = iota
	// ExhaustedReturnDefault returns a synthesized default response.
	ExhaustedReturnDefault
	// ExhaustedSucceedSilently converts the failure to a silent success.
	ExhaustedSucceedSilently
)

// String returns the topology YAML token for a.
func (a ExhaustedAction) String() string {
	switch a {
	case ExhaustedPropagate:
		return "propagate"
	case ExhaustedReturnDefault:
		return "return_default"
	case ExhaustedSucceedSilently:
		return "succeed_silently"
	default:
		return "unknown"
	}
}

// FaultKind identifies the type of fault to inject.
type FaultKind int

const (
	// FaultLatencyInflation inflates latency on the target.
	FaultLatencyInflation FaultKind = iota
	// FaultErrorRateOverride overrides error probability on the target.
	FaultErrorRateOverride
	// FaultDisconnect simulates a disconnect from the target.
	FaultDisconnect
	// FaultCrash simulates a crash of the target.
	FaultCrash
)

// String returns the topology YAML token for k.
func (k FaultKind) String() string {
	switch k {
	case FaultLatencyInflation:
		return "latency_inflation"
	case FaultErrorRateOverride:
		return "error_rate_override"
	case FaultDisconnect:
		return "disconnect"
	case FaultCrash:
		return "crash"
	default:
		return "unknown"
	}
}

// TargetKind identifies the topology entity addressed by a fault.
type TargetKind int

const (
	// TargetNode addresses a service node.
	TargetNode TargetKind = iota
	// TargetOperation addresses an operation.
	TargetOperation
	// TargetEdge addresses an edge.
	TargetEdge
)

// String returns the topology YAML token for k.
func (k TargetKind) String() string {
	switch k {
	case TargetNode:
		return "node"
	case TargetOperation:
		return "operation"
	case TargetEdge:
		return "edge"
	default:
		return "unknown"
	}
}

// BackoffPolicy identifies the retry backoff strategy for an edge.
type BackoffPolicy int

const (
	// BackoffExponential increases retry delay exponentially.
	BackoffExponential BackoffPolicy = iota
	// BackoffLinear increases retry delay linearly.
	BackoffLinear
	// BackoffConstant keeps retry delay constant.
	BackoffConstant
)

// String returns the topology YAML token for p.
func (p BackoffPolicy) String() string {
	switch p {
	case BackoffExponential:
		return "exponential"
	case BackoffLinear:
		return "linear"
	case BackoffConstant:
		return "constant"
	default:
		return "unknown"
	}
}
