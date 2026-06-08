package generators

import (
	"fmt"
	"math"
	"regexp"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

func TestValidServiceID_MatchesPattern(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		id := ValidServiceID().Draw(t, "id")
		if !validServiceIDRegexp().MatchString(string(id)) {
			t.Fatalf("service ID %q does not match valid pattern", id)
		}
	})
}

func TestValidLatencyPair_P95AtLeastP50(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		latency := ValidLatencyPair().Draw(t, "latency")
		if latency.P95 < latency.P50 {
			t.Fatalf("p95 %s is less than p50 %s", latency.P95, latency.P50)
		}
	})
}

func TestAnyServiceID_ContainsInvalidStatistical(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		invalid := false
		for i := 0; i < 100; i++ {
			id := AnyServiceID().Draw(t, fmt.Sprintf("id_%d", i))
			if !validServiceIDRegexp().MatchString(string(id)) {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyServiceID produced no invalid values in 100 draws")
		}
	})
}

func validServiceIDRegexp() *regexp.Regexp {
	return regexp.MustCompile(`^[a-z][a-z0-9-]{2,30}$`)
}

func TestValidServiceKind_Enumerated(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		kind := ValidServiceKind().Draw(t, "kind")
		switch kind {
		case topology.KindApplication, topology.KindDatabase, topology.KindExternalAPI, topology.KindCache, topology.KindQueue:
		default:
			t.Fatalf("unexpected service kind: %v", kind)
		}
	})
}

func TestValidProtocol_Enumerated(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		protocol := ValidProtocol().Draw(t, "protocol")
		switch protocol {
		case topology.ProtocolHTTP, topology.ProtocolGRPC, topology.ProtocolMessaging:
		default:
			t.Fatalf("unexpected protocol: %v", protocol)
		}
	})
}

func TestAnyProbability_CoversNonFiniteOrOutOfRange(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		covered := false
		for i := 0; i < 100; i++ {
			p := AnyProbability().Draw(t, fmt.Sprintf("probability_%d", i))
			if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
				covered = true
				break
			}
		}
		if !covered {
			t.Fatal("AnyProbability produced no invalid values in 100 draws")
		}
	})
}
