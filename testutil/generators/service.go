package generators

import (
	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

// ValidService returns a service with valid name, replicas, and operation back-pointers.
func ValidService(opts ...ServiceOption) *rapid.Generator[*topology.Service] {
	o := applyServiceOptions(opts)
	return rapid.Custom(func(t *rapid.T) *topology.Service {
		var kind topology.ServiceKind
		if o.fixedKind != nil {
			kind = *o.fixedKind
		} else {
			kind = ValidServiceKind().Draw(t, "kind")
		}

		svc := &topology.Service{
			Name:       ValidServiceID().Draw(t, "name"),
			Kind:       kind,
			Replicas:   ValidReplicaCount().Draw(t, "replicas"),
			Operations: make(map[string]*topology.Operation),
		}

		opCount := rapid.IntRange(1, o.maxOpsPerService).Draw(t, "n_ops")
		opNames := rapid.SliceOfNDistinct(
			ValidOperationName(),
			opCount,
			opCount,
			func(name string) string { return name },
		).Draw(t, "op_names")
		for _, name := range opNames {
			svc.Operations[name] = &topology.Operation{
				Name:    name,
				Service: svc,
			}
		}

		return svc
	})
}

// AnyService returns a service that may violate name or back-pointer invariants.
func AnyService(opts ...ServiceOption) *rapid.Generator[*topology.Service] {
	return rapid.Custom(func(t *rapid.T) *topology.Service {
		svc := ValidService(opts...).Draw(t, "valid_service")
		switch rapid.IntRange(0, 4).Draw(t, "service_mutation") {
		case 0:
			svc.Name = AnyServiceID().Draw(t, "any_name")
		case 1:
			if len(svc.Operations) > 0 {
				name := rapid.SampledFrom(mapKeys(svc.Operations)).Draw(t, "op_to_detach")
				svc.Operations[name].Service = nil
			}
		case 2:
			if len(svc.Operations) > 0 {
				name := rapid.SampledFrom(mapKeys(svc.Operations)).Draw(t, "op_to_scramble")
				svc.Operations[name].Service = &topology.Service{Name: "other"}
			}
		case 3:
			svc.Operations = map[string]*topology.Operation{}
		}
		return svc
	})
}

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
