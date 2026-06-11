// SPDX-License-Identifier: Apache-2.0

package exporter_test

import (
	"reflect"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/testutil/generators"
	"pgregory.net/rapid"
)

func TestMergeWith_OverrideWins_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		base := generators.ValidConfig().Draw(t, "base")
		override := generators.ValidConfig().Draw(t, "override")
		merged := base.MergeWith(override)
		if override.Protocol != exporter.ProtocolGRPC && merged.Protocol != override.Protocol {
			t.Fatalf("Protocol = %v, want override %v", merged.Protocol, override.Protocol)
		}
		if override.Endpoint != "" && merged.Endpoint != override.Endpoint {
			t.Fatalf("Endpoint = %q, want override %q", merged.Endpoint, override.Endpoint)
		}
		if override.Headers != nil && !reflect.DeepEqual(merged.Headers, override.Headers) {
			t.Fatalf("Headers = %#v, want override %#v", merged.Headers, override.Headers)
		}
		if override.Insecure && !merged.Insecure {
			t.Fatal("Insecure = false, want override true")
		}
		if override.Compression != "" && merged.Compression != override.Compression {
			t.Fatalf("Compression = %q, want override %q", merged.Compression, override.Compression)
		}
		if override.Timeout != 0 && merged.Timeout != override.Timeout {
			t.Fatalf("Timeout = %v, want override %v", merged.Timeout, override.Timeout)
		}
		if override.BatchSize > 0 && merged.BatchSize != override.BatchSize {
			t.Fatalf("BatchSize = %d, want override %d", merged.BatchSize, override.BatchSize)
		}
		if override.BatchTimeout > 0 && merged.BatchTimeout != override.BatchTimeout {
			t.Fatalf("BatchTimeout = %v, want override %v", merged.BatchTimeout, override.BatchTimeout)
		}
		if override.MaxQueueSize > 0 && merged.MaxQueueSize != override.MaxQueueSize {
			t.Fatalf("MaxQueueSize = %d, want override %d", merged.MaxQueueSize, override.MaxQueueSize)
		}
		if override.ResourceOverrides != nil && !reflect.DeepEqual(merged.ResourceOverrides, override.ResourceOverrides) {
			t.Fatalf("ResourceOverrides = %#v, want override %#v", merged.ResourceOverrides, override.ResourceOverrides)
		}
	})
}

func TestMergeWith_Idempotent_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		cfg := generators.ValidConfig().Draw(t, "cfg")
		merged := cfg.MergeWith(cfg)
		if !reflect.DeepEqual(merged, cfg) {
			t.Fatalf("MergeWith self = %#v, want %#v", merged, cfg)
		}
	})
}
