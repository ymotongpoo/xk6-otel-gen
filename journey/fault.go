package journey

import (
	"math"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

const defaultEntryLatency = 10 * time.Millisecond

type foldedFault struct {
	crashed        bool
	disconnected   bool
	errorRate      float64
	errorType      string
	latencyInflate time.Duration
}

func (e *engineImpl) foldFaults(node *Node) foldedFault {
	var ff foldedFault
	if node == nil {
		return ff
	}

	for _, spec := range e.overlay.NodeFaults(node.Service) {
		switch spec.Kind {
		case topology.FaultCrash:
			if e.faultActive(spec) {
				ff.crashed = true
			}
		case topology.FaultLatencyInflation:
			if e.faultActive(spec) {
				ff.latencyInflate += e.sampleInflation(spec)
			}
		}
	}

	if node.Edge != nil {
		for _, spec := range e.overlay.EdgeFaults(node.Edge) {
			switch spec.Kind {
			case topology.FaultDisconnect:
				if e.faultActive(spec) {
					ff.disconnected = true
				}
			case topology.FaultLatencyInflation:
				if e.faultActive(spec) {
					ff.latencyInflate += e.sampleInflation(spec)
				}
			}
		}
	}

	if node.Service == nil {
		return ff
	}
	if op := node.Service.Operations[node.Operation]; op != nil {
		for _, spec := range e.overlay.OperationFaults(op) {
			switch spec.Kind {
			case topology.FaultErrorRateOverride:
				ff.errorRate = clampProbability(spec.Severity.Value)
				ff.errorType = defaultFaultErrorType(node)
			case topology.FaultLatencyInflation:
				if e.faultActive(spec) {
					ff.latencyInflate += e.sampleInflation(spec)
				}
			}
		}
	}

	return ff
}

func (e *engineImpl) sampleInflation(spec topology.FaultSpec) time.Duration {
	add := spec.Severity.Add
	if add < 0 {
		add = 0
	}
	if spec.Severity.Multiplier <= 1 {
		return add
	}
	return add + time.Duration((spec.Severity.Multiplier-1)*float64(defaultEntryLatency))
}

func (e *engineImpl) sampleEdgeLatency(edge *topology.Edge) time.Duration {
	if edge == nil {
		return defaultEntryLatency
	}
	dist := edge.Latency
	switch dist.Distribution {
	case "", "fixed":
		return dist.P50
	case "lognormal":
		return e.sampleLognormal(dist.P50, dist.P95)
	case "uniform":
		return e.sampleUniform(dist.P50, dist.P95)
	default:
		return dist.P50
	}
}

func (e *engineImpl) sampleLognormal(p50, p95 time.Duration) time.Duration {
	if p50 <= 0 {
		return 0
	}
	if p95 <= p50 {
		return p50
	}
	const z95 = 1.6448536269514722
	mu := math.Log(float64(p50))
	sigma := math.Log(float64(p95)/float64(p50)) / z95
	u1 := e.randFloat64()
	if u1 <= 0 {
		u1 = math.SmallestNonzeroFloat64
	}
	u2 := e.randFloat64()
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	return time.Duration(math.Exp(mu + sigma*z))
}

func (e *engineImpl) sampleUniform(p50, p95 time.Duration) time.Duration {
	if p95 <= p50 {
		return p50
	}
	return p50 + time.Duration(e.randFloat64()*float64(p95-p50))
}

func (e *engineImpl) faultActive(spec topology.FaultSpec) bool {
	p := spec.Severity.Probability
	switch {
	case p <= 0:
		return false
	case p >= 1:
		return true
	default:
		return e.randFloat64() < p
	}
}

func clampProbability(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

func defaultFaultErrorType(node *Node) string {
	if node != nil && node.Service != nil && (node.Service.Kind == topology.KindDatabase || node.Service.Kind == topology.KindCache) {
		return "db.connection_lost"
	}
	if node != nil && node.Edge != nil && node.Edge.Protocol == topology.ProtocolGRPC {
		return "grpc.unavailable"
	}
	return "http.500"
}
