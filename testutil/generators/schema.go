// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"fmt"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"pgregory.net/rapid"
)

// ValidSchema returns a generator producing schemas satisfying R-STR-1..R-STR-8.
func ValidSchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema] {
	o := applySchemaOptions(opts)
	return rapid.Custom(func(t *rapid.T) *topology.Schema {
		schema := &topology.Schema{
			Namespace: topology.DefaultNamespace,
			Services:  make(map[topology.ServiceID]*topology.Service),
			Journeys:  make(map[string]*topology.Journey),
		}
		topoOrder := buildServicesAndOperations(t, schema, o)
		buildEdges(t, topoOrder, o)
		buildJourneys(t, schema, topoOrder, o)
		buildFaults(t, schema, topoOrder, o)
		return schema
	})
}

// AnySchema returns valid schemas with BiasValid probability and degraded schemas otherwise.
func AnySchema(opts ...SchemaOption) *rapid.Generator[*topology.Schema] {
	o := applySchemaOptions(opts)
	return rapid.Custom(func(t *rapid.T) *topology.Schema {
		schema := ValidSchema(opts...).Draw(t, "valid_base")
		if o.biasValid >= 1 || rapid.Float64Range(0, 1).Draw(t, "bias_roll") < o.biasValid {
			return schema
		}
		mutators := schemaMutators()
		mutate := rapid.SampledFrom(mutators).Draw(t, "schema_mutator")
		return mutate(t, schema)
	})
}

func buildServicesAndOperations(t *rapid.T, schema *topology.Schema, o schemaOptions) []*topology.Operation {
	serviceCount := rapid.IntRange(1, o.maxServices).Draw(t, "n_services")
	serviceNames := rapid.SliceOfNDistinct(
		ValidServiceID(),
		serviceCount,
		serviceCount,
		func(id topology.ServiceID) topology.ServiceID { return id },
	).Draw(t, "service_names")

	topoOrder := make([]*topology.Operation, 0, serviceCount*o.maxOpsPerService)
	for svcIndex, name := range serviceNames {
		var kind topology.ServiceKind
		if o.fixedKind != nil {
			kind = *o.fixedKind
		} else {
			kind = ValidServiceKind().Draw(t, fmt.Sprintf("service_%d_kind", svcIndex))
		}
		svc := &topology.Service{
			Name:       name,
			Namespace:  topology.DefaultNamespace,
			Kind:       kind,
			Replicas:   ValidReplicaCount().Draw(t, fmt.Sprintf("service_%d_replicas", svcIndex)),
			Language:   rapid.SampledFrom([]string{"go", "java", "python", "nodejs", "ruby"}).Draw(t, fmt.Sprintf("service_%d_language", svcIndex)),
			Framework:  rapid.SampledFrom([]string{"net/http", "grpc", "gin", "spring", "fastapi", "express"}).Draw(t, fmt.Sprintf("service_%d_framework", svcIndex)),
			Version:    rapid.StringMatching(`^v[0-9]{1,2}\.[0-9]{1,2}\.[0-9]{1,2}$`).Draw(t, fmt.Sprintf("service_%d_version", svcIndex)),
			Operations: make(map[string]*topology.Operation),
		}
		schema.Services[name] = svc
		if rapid.Float64Range(0, 1).Draw(t, fmt.Sprintf("service_%d_metrics_roll", svcIndex)) < 0.35 {
			count := rapid.IntRange(1, 3).Draw(t, fmt.Sprintf("service_%d_n_metrics", svcIndex))
			svc.Metrics = make([]topology.ObservableMetricSpec, 0, count)
			for i := 0; i < count; i++ {
				svc.Metrics = append(svc.Metrics, ValidObservableMetricSpec().Draw(t, fmt.Sprintf("service_%d_metric_%d", svcIndex, i)))
			}
		}

		opCount := rapid.IntRange(1, o.maxOpsPerService).Draw(t, fmt.Sprintf("service_%d_n_ops", svcIndex))
		opNames := rapid.SliceOfNDistinct(
			ValidOperationName(),
			opCount,
			opCount,
			func(name string) string { return name },
		).Draw(t, fmt.Sprintf("service_%d_op_names", svcIndex))
		for _, opName := range opNames {
			op := &topology.Operation{
				Name:    opName,
				Service: svc,
			}
			if rapid.Float64Range(0, 1).Draw(t, fmt.Sprintf("service_%d_op_%s_state_roll", svcIndex, opName)) < 0.35 {
				count := rapid.IntRange(1, 3).Draw(t, fmt.Sprintf("service_%d_op_%s_n_state_updates", svcIndex, opName))
				op.StateUpdates = make([]topology.MetricStateUpdateSpec, 0, count)
				for i := 0; i < count; i++ {
					op.StateUpdates = append(op.StateUpdates, ValidMetricStateUpdateSpec().Draw(t, fmt.Sprintf("service_%d_op_%s_state_update_%d", svcIndex, opName, i)))
				}
			}
			svc.Operations[opName] = op
			topoOrder = append(topoOrder, op)
		}
	}

	return topoOrder
}

func buildEdges(t *rapid.T, topoOrder []*topology.Operation, o schemaOptions) {
	for i, op := range topoOrder {
		candidates := topoOrder[i+1:]
		if len(candidates) == 0 || o.maxCallsPerOp == 0 {
			continue
		}
		maxCalls := min(o.maxCallsPerOp, len(candidates))
		callCount := rapid.IntRange(0, maxCalls).Draw(t, fmt.Sprintf("op_%d_n_calls", i))
		if callCount == 0 {
			continue
		}
		targets := rapid.SliceOfNDistinct(
			rapid.SampledFrom(candidates),
			callCount,
			callCount,
			func(op *topology.Operation) *topology.Operation { return op },
		).Draw(t, fmt.Sprintf("op_%d_targets", i))

		nodes := make([]*topology.CallNode, 0, len(targets))
		for targetIndex, target := range targets {
			edge := buildEdge(t, fmt.Sprintf("op_%d_edge_%d", i, targetIndex), op, target, candidates)
			nodes = append(nodes, &topology.CallNode{Edge: edge})
		}

		if len(nodes) > 1 && rapid.Float64Range(0, 1).Draw(t, fmt.Sprintf("op_%d_parallel_roll", i)) < 0.2 {
			op.Calls = append(op.Calls, &topology.CallNode{Parallel: nodes})
			continue
		}
		op.Calls = append(op.Calls, nodes...)
	}
}

func buildEdge(t *rapid.T, label string, from, to *topology.Operation, fallbackCandidates []*topology.Operation) *topology.Edge {
	latency := ValidLatencyPair().Draw(t, label+"_latency")
	edge := &topology.Edge{
		From:           from,
		To:             to,
		Protocol:       ValidProtocol().Draw(t, label+"_protocol"),
		Latency:        topology.LatencyDist{Distribution: "lognormal", P50: latency.P50, P95: latency.P95},
		ErrorRate:      ValidProbability().Draw(t, label+"_error_rate"),
		Timeout:        ValidTimeout().Draw(t, label+"_timeout"),
		Retries:        rapid.IntRange(0, 10).Draw(t, label+"_retries"),
		RetryBackoff:   validBackoffPolicy(t, label+"_retry_backoff"),
		RetryBaseDelay: ValidTimeout().Draw(t, label+"_retry_base_delay"),
	}
	if len(fallbackCandidates) > 0 && rapid.Float64Range(0, 1).Draw(t, label+"_recovery_roll") < 0.3 {
		fallbackTo := rapid.SampledFrom(fallbackCandidates).Draw(t, label+"_fallback_to")
		fallbackLatency := ValidLatencyPair().Draw(t, label+"_fallback_latency")
		fallback := &topology.Edge{
			From:           from,
			To:             fallbackTo,
			Protocol:       ValidProtocol().Draw(t, label+"_fallback_protocol"),
			Latency:        topology.LatencyDist{Distribution: "lognormal", P50: fallbackLatency.P50, P95: fallbackLatency.P95},
			ErrorRate:      ValidProbability().Draw(t, label+"_fallback_error_rate"),
			Timeout:        ValidTimeout().Draw(t, label+"_fallback_timeout"),
			Retries:        rapid.IntRange(0, 3).Draw(t, label+"_fallback_retries"),
			RetryBackoff:   validBackoffPolicy(t, label+"_fallback_backoff"),
			RetryBaseDelay: ValidTimeout().Draw(t, label+"_fallback_retry_base_delay"),
		}
		edge.OnFailure = &topology.RecoveryPolicy{
			Fallback:    []*topology.Edge{fallback},
			OnExhausted: validExhaustedAction(t, label+"_on_exhausted"),
			DefaultResponse: map[string]any{
				"status": "default",
			},
		}
	}
	return edge
}

func buildJourneys(t *rapid.T, schema *topology.Schema, topoOrder []*topology.Operation, _ schemaOptions) {
	journeyCount := rapid.IntRange(1, 3).Draw(t, "n_journeys")
	for i := 0; i < journeyCount; i++ {
		stepCount := rapid.IntRange(1, min(3, len(topoOrder))).Draw(t, fmt.Sprintf("journey_%d_n_steps", i))
		steps := make([]*topology.Step, 0, stepCount)
		for stepIndex := 0; stepIndex < stepCount; stepIndex++ {
			steps = append(steps, &topology.Step{
				Op: rapid.SampledFrom(topoOrder).Draw(t, fmt.Sprintf("journey_%d_step_%d_op", i, stepIndex)),
			})
		}
		name := fmt.Sprintf("journey-%d", i+1)
		schema.Journeys[name] = &topology.Journey{
			Name:   name,
			Steps:  steps,
			Weight: rapid.Float64Range(0.1, 10).Draw(t, fmt.Sprintf("journey_%d_weight", i)),
		}
	}
}

func buildFaults(t *rapid.T, schema *topology.Schema, topoOrder []*topology.Operation, o schemaOptions) {
	faultCount := rapid.IntRange(0, o.maxFaults).Draw(t, "n_faults")
	if faultCount == 0 {
		return
	}
	services := uniqueServices(topoOrder)
	edges := collectEdgesFromOperations(topoOrder)
	schema.Faults = make([]topology.FaultSpec, 0, faultCount)
	for i := 0; i < faultCount; i++ {
		targetKind := rapid.IntRange(0, 2).Draw(t, fmt.Sprintf("fault_%d_target_kind", i))
		if len(edges) == 0 && targetKind == 2 {
			targetKind = 1
		}

		target := topology.FaultTarget{}
		switch targetKind {
		case 0:
			target.Kind = topology.TargetNode
			target.Service = rapid.SampledFrom(services).Draw(t, fmt.Sprintf("fault_%d_service", i))
		case 1:
			target.Kind = topology.TargetOperation
			target.Operation = rapid.SampledFrom(topoOrder).Draw(t, fmt.Sprintf("fault_%d_operation", i))
		default:
			target.Kind = topology.TargetEdge
			target.Edge = rapid.SampledFrom(edges).Draw(t, fmt.Sprintf("fault_%d_edge", i))
		}

		schema.Faults = append(schema.Faults, topology.FaultSpec{
			Target: target,
			Kind: rapid.SampledFrom([]topology.FaultKind{
				topology.FaultLatencyInflation,
				topology.FaultErrorRateOverride,
				topology.FaultDisconnect,
				topology.FaultCrash,
			}).Draw(t, fmt.Sprintf("fault_%d_kind", i)),
			Severity: topology.SeverityParams{
				Probability: ValidProbability().Draw(t, fmt.Sprintf("fault_%d_probability", i)),
				Multiplier:  rapid.Float64Range(1, 10).Draw(t, fmt.Sprintf("fault_%d_multiplier", i)),
				Add:         ValidTimeout().Draw(t, fmt.Sprintf("fault_%d_add", i)),
				Value:       ValidProbability().Draw(t, fmt.Sprintf("fault_%d_value", i)),
			},
		})
	}
}

func validBackoffPolicy(t *rapid.T, label string) topology.BackoffPolicy {
	return rapid.SampledFrom([]topology.BackoffPolicy{
		topology.BackoffExponential,
		topology.BackoffLinear,
		topology.BackoffConstant,
	}).Draw(t, label)
}

func validExhaustedAction(t *rapid.T, label string) topology.ExhaustedAction {
	return rapid.SampledFrom([]topology.ExhaustedAction{
		topology.ExhaustedPropagate,
		topology.ExhaustedReturnDefault,
		topology.ExhaustedSucceedSilently,
	}).Draw(t, label)
}

func uniqueServices(ops []*topology.Operation) []*topology.Service {
	seen := make(map[*topology.Service]struct{})
	services := make([]*topology.Service, 0)
	for _, op := range ops {
		if _, ok := seen[op.Service]; ok {
			continue
		}
		seen[op.Service] = struct{}{}
		services = append(services, op.Service)
	}
	return services
}

func collectEdgesFromOperations(ops []*topology.Operation) []*topology.Edge {
	edges := make([]*topology.Edge, 0)
	for _, op := range ops {
		edges = append(edges, collectEdgesFromCallNodes(op.Calls)...)
	}
	return edges
}

func collectEdgesFromCallNodes(nodes []*topology.CallNode) []*topology.Edge {
	edges := make([]*topology.Edge, 0)
	for _, node := range nodes {
		switch {
		case node == nil:
			continue
		case node.Edge != nil:
			edges = append(edges, node.Edge)
			if node.Edge.OnFailure != nil {
				edges = append(edges, node.Edge.OnFailure.Fallback...)
			}
		case len(node.Parallel) > 0:
			edges = append(edges, collectEdgesFromCallNodes(node.Parallel)...)
		}
	}
	return edges
}
