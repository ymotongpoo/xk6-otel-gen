package generators

import (
	"fmt"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"pgregory.net/rapid"
)

func TestValidConfig_PassesValidate_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		cfg := ValidConfig().Draw(t, "cfg")
		if err := cfg.Validate(); err != nil {
			t.Fatalf("ValidConfig().Validate() error = %v for %#v", err, cfg)
		}
	})
}

func TestAnyConfig_SometimesInvalid_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		invalid := 0
		for i := 0; i < 100; i++ {
			cfg := AnyConfig().Draw(t, fmt.Sprintf("cfg_%d", i))
			if cfg.Validate() != nil {
				invalid++
			}
		}
		if invalid == 0 {
			t.Fatal("AnyConfig produced no invalid configs in 100 draws")
		}
	})
}

func TestValidConfig_Options_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		cfg := ValidConfig(
			WithFixedEndpoint("fixed.example.com:4317"),
			WithProtocol(exporter.ProtocolHTTP),
			WithMinTimeout(500*time.Millisecond),
		).Draw(t, "cfg")
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v for %#v", err, cfg)
		}
		if cfg.Endpoint != "fixed.example.com:4317" {
			t.Fatalf("Endpoint = %q, want fixed.example.com:4317", cfg.Endpoint)
		}
		if cfg.Protocol != exporter.ProtocolHTTP {
			t.Fatalf("Protocol = %v, want ProtocolHTTP", cfg.Protocol)
		}
		if cfg.Timeout < 500*time.Millisecond {
			t.Fatalf("Timeout = %v, want >= 500ms", cfg.Timeout)
		}
	})
}
