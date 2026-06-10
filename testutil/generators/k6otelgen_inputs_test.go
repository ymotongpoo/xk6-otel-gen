package generators

import (
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func TestValidConfigureOpts_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		opts := ValidConfigureOpts().Draw(t, "opts")

		if endpoint, ok := opts["endpoint"].(string); !ok || endpoint == "" {
			t.Fatalf("endpoint = %#v, want non-empty string", opts["endpoint"])
		}
		protocol, ok := opts["protocol"].(string)
		if !ok || (protocol != "grpc" && protocol != "http") {
			t.Fatalf("protocol = %#v, want grpc or http", opts["protocol"])
		}
		if _, ok := opts["insecure"].(bool); !ok {
			t.Fatalf("insecure = %#v, want bool", opts["insecure"])
		}
		assertStringAnyMap(t, opts["headers"], "headers")
		assertStringAnyMap(t, opts["resourceOverrides"], "resourceOverrides")
		switch timeout := opts["timeout"].(type) {
		case int:
			if timeout <= 0 {
				t.Fatalf("timeout = %d, want positive", timeout)
			}
		case string:
			if !strings.HasSuffix(timeout, "ms") {
				t.Fatalf("timeout = %q, want duration string", timeout)
			}
		default:
			t.Fatalf("timeout = %#v, want int or string", timeout)
		}
		batchSize, ok := opts["batchSize"].(int)
		if !ok || batchSize <= 0 {
			t.Fatalf("batchSize = %#v, want positive int", opts["batchSize"])
		}
		batchTimeout, ok := opts["batchTimeout"].(int)
		if !ok || batchTimeout <= 0 {
			t.Fatalf("batchTimeout = %#v, want positive int", opts["batchTimeout"])
		}
		maxQueueSize, ok := opts["maxQueueSize"].(int)
		if !ok || maxQueueSize < batchSize {
			t.Fatalf("maxQueueSize = %#v, want >= batchSize %d", opts["maxQueueSize"], batchSize)
		}
	})
}

func TestValidLoadPath_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		path := ValidLoadPath().Draw(t, "path")
		if path == "" {
			t.Fatal("path is empty")
		}
		if filepath.IsAbs(path) {
			t.Fatalf("path = %q, want relative", path)
		}
		if strings.Contains(path, "..") {
			t.Fatalf("path = %q, want no traversal", path)
		}
		if filepath.Ext(path) != ".yaml" {
			t.Fatalf("path = %q, want .yaml extension", path)
		}
	})
}

func assertStringAnyMap(t *rapid.T, value any, name string) {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want map[string]any", name, value)
	}
	for key, value := range m {
		if key == "" {
			t.Fatalf("%s has empty key", name)
		}
		if _, ok := value.(string); !ok {
			t.Fatalf("%s[%q] = %#v, want string", name, key, value)
		}
	}
}
