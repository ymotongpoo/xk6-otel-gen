// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"strconv"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestBuildResource_Minimal(t *testing.T) {
	t.Parallel()

	svc := &topology.Service{Name: "checkout", Replicas: 1}
	res := BuildResource(svc, 0)
	attrs := resourceAttrs(res.Attributes())

	requireAttr(t, attrs, semconv.ServiceNameKey, "checkout")
	requireAttr(t, attrs, semconv.ServiceNamespaceKey, topology.DefaultNamespace)
	requireAttr(t, attrs, semconv.ServiceInstanceIDKey, InstanceID("checkout", 0))
	requireAttr(t, attrs, semconv.TelemetrySDKNameKey, "opentelemetry")
	requireAttr(t, attrs, semconv.TelemetrySDKLanguageKey, "go")
	if _, ok := attrs[semconv.ServiceVersionKey]; ok {
		t.Fatal("service.version unexpectedly present")
	}
}

func TestBuildResource_AllFields(t *testing.T) {
	t.Parallel()

	svc := &topology.Service{
		Name:      "checkout",
		Namespace: "payments",
		Replicas:  2,
		Version:   "1.2.3",
		Language:  "go",
		Framework: "gin",
	}
	res := BuildResource(svc, 1)
	attrs := resourceAttrs(res.Attributes())

	requireAttr(t, attrs, semconv.ServiceNameKey, "checkout")
	requireAttr(t, attrs, semconv.ServiceNamespaceKey, "payments")
	requireAttr(t, attrs, semconv.ServiceVersionKey, "1.2.3")
	requireAttr(t, attrs, semconv.ServiceInstanceIDKey, InstanceID("checkout", 1))
	requireAttr(t, attrs, semconv.ProcessRuntimeNameKey, "go")
	requireAttr(t, attrs, attribute.Key("synth.service.framework"), "gin")
}

func TestBuildResource_NilPanics(t *testing.T) {
	t.Parallel()

	requirePanic(t, func() {
		BuildResource(nil, 0)
	})
}

func TestBuildResource_InvalidIdxPanics(t *testing.T) {
	t.Parallel()

	requirePanic(t, func() {
		BuildResource(&topology.Service{Name: "checkout"}, -1)
	})
}

func TestBuildResource_EmptyNamePanics(t *testing.T) {
	t.Parallel()

	requirePanic(t, func() {
		BuildResource(&topology.Service{}, 0)
	})
}

func TestInstanceID_Deterministic(t *testing.T) {
	t.Parallel()

	first := InstanceID("checkout", 3)
	second := InstanceID("checkout", 3)
	if first != second {
		t.Fatalf("InstanceID returned %q then %q", first, second)
	}
	if _, err := uuid.Parse(first); err != nil {
		t.Fatalf("InstanceID returned invalid UUID %q: %v", first, err)
	}
}

func resourceAttrs(kvs []attribute.KeyValue) map[attribute.Key]string {
	attrs := make(map[attribute.Key]string, len(kvs))
	for _, kv := range kvs {
		switch kv.Value.Type() {
		case attribute.STRING:
			attrs[kv.Key] = kv.Value.AsString()
		case attribute.INT64:
			attrs[kv.Key] = strconv.FormatInt(kv.Value.AsInt64(), 10)
		default:
			attrs[kv.Key] = kv.Value.AsString()
		}
	}
	return attrs
}

func requireAttr(t *testing.T, attrs map[attribute.Key]string, key attribute.Key, want string) {
	t.Helper()

	got, ok := attrs[key]
	if !ok {
		t.Fatalf("attribute %q missing", key)
	}
	if got != want {
		t.Fatalf("attribute %q = %q, want %q", key, got, want)
	}
}

func requirePanic(t *testing.T, fn func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
