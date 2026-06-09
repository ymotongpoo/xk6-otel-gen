package exporter

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

func TestBuildResource_AutoDetectOnly(t *testing.T) {
	t.Parallel()

	res, err := buildResource(context.Background(), Config{})
	if err != nil {
		t.Fatalf("buildResource() error = %v, want nil", err)
	}
	if res == nil {
		t.Fatal("buildResource() = nil, want resource")
	}
	if _, ok := resourceAttr(res, string(semconv.HostNameKey)); !ok {
		t.Fatalf("resource attributes missing host.name: %v", res.Attributes())
	}
	if _, ok := resourceAttr(res, string(semconv.OSTypeKey)); !ok {
		t.Fatalf("resource attributes missing os.type: %v", res.Attributes())
	}
}

func TestBuildResource_OverrideWins(t *testing.T) {
	t.Parallel()

	res, err := buildResource(context.Background(), Config{
		ResourceOverrides: map[string]string{
			string(semconv.ServiceNameKey): "catalog-service",
			"deployment.environment":       "prod",
		},
	})
	if err != nil {
		t.Fatalf("buildResource() error = %v, want nil", err)
	}
	if got, ok := resourceAttr(res, string(semconv.ServiceNameKey)); !ok || got.AsString() != "catalog-service" {
		t.Fatalf("service.name = %q, %v; want catalog-service, true", got.AsString(), ok)
	}
	if got, ok := resourceAttr(res, "deployment.environment"); !ok || got.AsString() != "prod" {
		t.Fatalf("deployment.environment = %q, %v; want prod, true", got.AsString(), ok)
	}
}

func resourceAttr(res *sdkresource.Resource, key string) (attribute.Value, bool) {
	for _, kv := range res.Attributes() {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}
