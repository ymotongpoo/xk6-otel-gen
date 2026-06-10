package synth

import (
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

var synthInstanceNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("xk6-otel-gen/synth"))

// InstanceID returns the deterministic UUID v5 service.instance.id for a
// service name and replica index.
func InstanceID(svcName string, idx int) string {
	return uuid.NewSHA1(synthInstanceNamespace, []byte(svcName+"/"+strconv.Itoa(idx))).String()
}

// BuildResource returns an OpenTelemetry Resource for one synthesized service
// instance using Semantic Conventions v1.27.0 resource attributes.
func BuildResource(svc *topology.Service, instanceIdx int) *resource.Resource {
	if svc == nil {
		panic("synth: BuildResource: svc must not be nil")
	}
	if svc.Name == "" {
		panic("synth: BuildResource: svc.Name must not be empty")
	}
	if instanceIdx < 0 {
		panic(fmt.Sprintf("synth: BuildResource: instanceIdx %d must be >= 0", instanceIdx))
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceName(string(svc.Name)),
		semconv.ServiceInstanceID(InstanceID(string(svc.Name), instanceIdx)),
		semconv.TelemetrySDKName("opentelemetry"),
		semconv.TelemetrySDKLanguageGo,
	}
	if svc.Version != "" {
		attrs = append(attrs, semconv.ServiceVersion(svc.Version))
	}
	if svc.Language != "" {
		// For synthesized services, topology.Service.Language represents the
		// runtime the simulated service would use, not the runtime of this Go SDK.
		attrs = append(attrs, semconv.ProcessRuntimeName(svc.Language))
	}
	if svc.Framework != "" {
		attrs = append(attrs, attribute.String("synth.service.framework", svc.Framework))
	}

	return resource.NewSchemaless(attrs...)
}
