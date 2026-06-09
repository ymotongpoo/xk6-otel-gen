package topology

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	invalidServiceKind     ServiceKind     = ServiceKind(-1)
	invalidProtocol        Protocol        = Protocol(-1)
	invalidBackoffPolicy   BackoffPolicy   = BackoffPolicy(-1)
	invalidFaultKind       FaultKind       = FaultKind(-1)
	invalidExhaustedAction ExhaustedAction = ExhaustedAction(-1)
)

// Parse reads topology YAML, resolves references, and validates the schema.
func Parse(r io.Reader) (*Schema, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("topology: read input: %w", err)
	}

	raw, err := decodeRaw(bytes.NewReader(data), false)
	if err != nil {
		return nil, err
	}

	schema := buildSchema(raw)
	if err := resolveReferences(schema, raw); err != nil {
		return nil, err
	}
	if err := Validate(schema); err != nil {
		return nil, err
	}
	return schema, nil
}

// ParseFile opens path and delegates parsing to Parse.
func ParseFile(path string) (*Schema, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("topology: open %s: %w", path, err)
	}
	defer f.Close()
	return Parse(f)
}

func decodeRaw(r io.Reader, strict bool) (*rawSchema, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(strict)

	var raw rawSchema
	if err := dec.Decode(&raw); err != nil {
		return nil, &ParseError{
			Path:    "<root>",
			Message: "yaml decode failed",
			Inner:   err,
		}
	}
	return &raw, nil
}

func buildSchema(raw *rawSchema) *Schema {
	schema := &Schema{
		Services: make(map[ServiceID]*Service),
		Journeys: make(map[string]*Journey),
	}
	if raw == nil {
		return schema
	}

	schema.Services = make(map[ServiceID]*Service, len(raw.Services))
	for name, rs := range raw.Services {
		if rs == nil {
			rs = &rawService{}
		}
		svc := &Service{
			Name:       ServiceID(name),
			Kind:       parseServiceKind(rs.Kind),
			Replicas:   intDefault(rs.Replicas, 1),
			Language:   rs.Language,
			Framework:  rs.Framework,
			Version:    rs.Version,
			Operations: make(map[string]*Operation, len(rs.Operations)),
		}
		for _, ro := range rs.Operations {
			if ro == nil {
				ro = &rawOperation{}
			}
			op := &Operation{
				Name:    ro.Name,
				Service: svc,
			}
			svc.Operations[ro.Name] = op
		}
		schema.Services[svc.Name] = svc
	}

	return schema
}

func resolveReferences(schema *Schema, raw *rawSchema) error {
	if raw == nil {
		return nil
	}

	errs := make([]error, 0)
	for svcName, rs := range raw.Services {
		svc := schema.Services[ServiceID(svcName)]
		if svc == nil || rs == nil {
			continue
		}
		for opIndex, ro := range rs.Operations {
			if ro == nil {
				continue
			}
			op := svc.Operations[ro.Name]
			if op == nil {
				continue
			}
			op.Calls = make([]*CallNode, 0, len(ro.Calls))
			for i, rc := range ro.Calls {
				path := fmt.Sprintf("services.%s.operations[%d].calls[%d]", svcName, opIndex, i)
				node, err := resolveCallNode(schema, svc, op, rc, path)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				op.Calls = append(op.Calls, node)
			}
		}
	}

	schema.Journeys = make(map[string]*Journey, len(raw.Journeys))
	for jName, rj := range raw.Journeys {
		if rj == nil {
			rj = &rawJourney{}
		}
		journey := &Journey{
			Name:   jName,
			Weight: float64Default(rj.Weight, 1.0),
			Steps:  make([]*Step, 0, len(rj.Steps)),
		}
		for i, rs := range rj.Steps {
			path := fmt.Sprintf("journeys.%s.steps[%d]", jName, i)
			step, err := resolveStep(schema, rs, path)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			journey.Steps = append(journey.Steps, step)
		}
		schema.Journeys[jName] = journey
	}

	schema.Faults = make([]FaultSpec, 0, len(raw.Faults))
	for i, rf := range raw.Faults {
		path := fmt.Sprintf("faults[%d]", i)
		if rf == nil {
			errs = append(errs, newParseError(path, "fault spec is required"))
			continue
		}
		target, err := resolveFaultTarget(schema, rf.Target, path+".target")
		if err != nil {
			errs = append(errs, err)
			continue
		}
		schema.Faults = append(schema.Faults, FaultSpec{
			Target:   target,
			Kind:     parseFaultKind(rf.Kind),
			Severity: resolveSeverity(rf.Severity),
		})
	}

	return errors.Join(errs...)
}

func resolveCallNode(schema *Schema, _ *Service, owningOp *Operation, rc *rawCallNode, path string) (*CallNode, error) {
	if rc == nil {
		return nil, newParseError(path, "call node is required")
	}

	hasTo := rc.To != nil
	hasParallel := len(rc.Parallel) > 0
	if hasTo == hasParallel {
		return nil, newParseError(path, "exactly one of 'to' or 'parallel' is required (R-STR-7)")
	}

	if hasParallel {
		errs := make([]error, 0)
		children := make([]*CallNode, 0, len(rc.Parallel))
		for i, child := range rc.Parallel {
			node, err := resolveCallNode(schema, nil, owningOp, child, fmt.Sprintf("%s.parallel[%d]", path, i))
			if err != nil {
				errs = append(errs, err)
				continue
			}
			children = append(children, node)
		}
		if err := errors.Join(errs...); err != nil {
			return nil, err
		}
		return &CallNode{Parallel: children}, nil
	}

	targetOp, err := lookupOperationAtPath(schema, rc.To.Service, rc.To.Operation, path+".to")
	if err != nil {
		return nil, err
	}

	edge := &Edge{
		From:         owningOp,
		To:           targetOp,
		Protocol:     parseProtocol(rc.Protocol),
		Latency:      resolveLatency(rc.Latency),
		ErrorRate:    float64Default(rc.ErrorRate, 0.0),
		Timeout:      durationDefault(rc.Timeout, 0),
		Retries:      intDefault(rc.Retries, 0),
		RetryBackoff: parseBackoff(rc.RetryBackoff),
	}
	if rc.OnFailure != nil {
		rp, err := resolveRecoveryPolicy(schema, owningOp, rc.OnFailure, path+".on_failure")
		if err != nil {
			return nil, err
		}
		edge.OnFailure = rp
	}
	return &CallNode{Edge: edge}, nil
}

func resolveStep(schema *Schema, rs *rawStep, path string) (*Step, error) {
	if rs == nil {
		return nil, newParseError(path, "step is required")
	}

	hasOp := rs.Service != "" || rs.Operation != ""
	hasParallel := len(rs.Parallel) > 0
	if hasOp == hasParallel {
		return nil, newParseError(path, "exactly one of service/operation or 'parallel' is required")
	}

	if hasParallel {
		errs := make([]error, 0)
		children := make([]*Step, 0, len(rs.Parallel))
		for i, child := range rs.Parallel {
			step, err := resolveStep(schema, child, fmt.Sprintf("%s.parallel[%d]", path, i))
			if err != nil {
				errs = append(errs, err)
				continue
			}
			children = append(children, step)
		}
		if err := errors.Join(errs...); err != nil {
			return nil, err
		}
		return &Step{Parallel: children}, nil
	}

	if rs.Service == "" {
		return nil, newParseError(path+".service", "service is required")
	}
	if rs.Operation == "" {
		return nil, newParseError(path+".operation", "operation is required")
	}
	op, err := lookupOperationAtPath(schema, rs.Service, rs.Operation, path)
	if err != nil {
		return nil, err
	}
	return &Step{Op: op}, nil
}

func resolveFaultTarget(schema *Schema, spec, path string) (FaultTarget, error) {
	kind, rest, ok := strings.Cut(spec, ":")
	if !ok || rest == "" {
		return FaultTarget{}, newParseError(path, `target must be "node:<svc>", "operation:<svc>.<op>", or "edge:<from>-><to>"`)
	}

	switch kind {
	case "node":
		svc := schema.Services[ServiceID(rest)]
		if svc == nil {
			return FaultTarget{}, newParseErrorf(path, "service %q not found", rest)
		}
		return FaultTarget{Kind: TargetNode, Service: svc}, nil
	case "operation":
		svcName, opName, err := parseOperationRef(rest, path)
		if err != nil {
			return FaultTarget{}, err
		}
		op, err := lookupOperationAtPath(schema, svcName, opName, path)
		if err != nil {
			return FaultTarget{}, err
		}
		return FaultTarget{Kind: TargetOperation, Operation: op}, nil
	case "edge":
		fromSpec, toSpec, ok := strings.Cut(rest, "->")
		if !ok || fromSpec == "" || toSpec == "" {
			return FaultTarget{}, newParseError(path, `edge target must be "edge:<svc>.<op>-><svc>.<op>"`)
		}
		fromSvc, fromOp, err := parseOperationRef(fromSpec, path+".from")
		if err != nil {
			return FaultTarget{}, err
		}
		toSvc, toOp, err := parseOperationRef(toSpec, path+".to")
		if err != nil {
			return FaultTarget{}, err
		}
		from, err := lookupOperationAtPath(schema, fromSvc, fromOp, path+".from")
		if err != nil {
			return FaultTarget{}, err
		}
		to, err := lookupOperationAtPath(schema, toSvc, toOp, path+".to")
		if err != nil {
			return FaultTarget{}, err
		}
		edge := findEdge(schema, from, to)
		if edge == nil {
			return FaultTarget{}, newParseErrorf(path, "edge %s.%s->%s.%s not found", fromSvc, fromOp, toSvc, toOp)
		}
		return FaultTarget{Kind: TargetEdge, Edge: edge}, nil
	default:
		return FaultTarget{}, newParseErrorf(path, "unsupported target kind %q", kind)
	}
}

func resolveRecoveryPolicy(schema *Schema, owningOp *Operation, rp *rawRecoveryPolicy, path string) (*RecoveryPolicy, error) {
	if rp == nil {
		return nil, nil
	}

	errs := make([]error, 0)
	out := &RecoveryPolicy{
		Fallback:        make([]*Edge, 0, len(rp.Fallback)),
		OnExhausted:     parseExhaustedAction(rp.OnExhausted),
		DefaultResponse: rp.DefaultResponse,
	}
	for i, rawFallback := range rp.Fallback {
		fallbackPath := fmt.Sprintf("%s.fallback[%d]", path, i)
		node, err := resolveCallNode(schema, nil, owningOp, rawFallback, fallbackPath)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if node.Edge == nil {
			errs = append(errs, newParseError(fallbackPath, "fallback must be a single edge call"))
			continue
		}
		node.Edge.From = owningOp
		out.Fallback = append(out.Fallback, node.Edge)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

func lookupOperation(schema *Schema, svcName, opName string) (*Operation, error) {
	return lookupOperationAtPath(schema, svcName, opName, fmt.Sprintf("operation:%s.%s", svcName, opName))
}

func lookupOperationAtPath(schema *Schema, svcName, opName, path string) (*Operation, error) {
	if svcName == "" {
		return nil, newParseError(path+".service", "service is required")
	}
	if opName == "" {
		return nil, newParseError(path+".operation", "operation is required")
	}
	svc, ok := schema.Services[ServiceID(svcName)]
	if !ok || svc == nil {
		return nil, newParseErrorf(path+".service", "service %q not found", svcName)
	}
	op, ok := svc.Operations[opName]
	if !ok || op == nil {
		return nil, newParseErrorf(path+".operation", "operation %q on service %q not found", opName, svcName)
	}
	return op, nil
}

func parseOperationRef(spec, path string) (string, string, error) {
	svcName, opName, ok := strings.Cut(spec, ".")
	if !ok || svcName == "" || opName == "" {
		return "", "", newParseError(path, `operation reference must be "<svc>.<op>"`)
	}
	return svcName, opName, nil
}

func findEdge(schema *Schema, from, to *Operation) *Edge {
	for _, svc := range schema.Services {
		if svc == nil {
			continue
		}
		for _, op := range svc.Operations {
			if edge := findEdgeInCalls(op.Calls, from, to); edge != nil {
				return edge
			}
		}
	}
	return nil
}

func findEdgeInCalls(nodes []*CallNode, from, to *Operation) *Edge {
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Edge != nil {
			if node.Edge.From == from && node.Edge.To == to {
				return node.Edge
			}
			if node.Edge.OnFailure != nil {
				for _, fallback := range node.Edge.OnFailure.Fallback {
					if fallback != nil && fallback.From == from && fallback.To == to {
						return fallback
					}
				}
			}
		}
		if edge := findEdgeInCalls(node.Parallel, from, to); edge != nil {
			return edge
		}
	}
	return nil
}

func resolveLatency(rl *rawLatencyDist) LatencyDist {
	if rl == nil {
		return LatencyDist{Distribution: "constant"}
	}
	p50 := durationDefault(rl.P50, 0)
	return LatencyDist{
		Distribution: stringDefault(rl.Distribution, "constant"),
		P50:          p50,
		P95:          durationDefault(rl.P95, p50),
	}
}

func resolveSeverity(rs *rawSeverity) SeverityParams {
	if rs == nil {
		return SeverityParams{}
	}
	return SeverityParams{
		Probability: float64Default(rs.Probability, 0),
		Multiplier:  float64Default(rs.Multiplier, 0),
		Add:         durationDefault(rs.Add, 0),
		Value:       float64Default(rs.Value, 0),
	}
}

func intDefault(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func float64Default(p *float64, def float64) float64 {
	if p == nil {
		return def
	}
	return *p
}

func durationDefault(p *time.Duration, def time.Duration) time.Duration {
	if p == nil {
		return def
	}
	return *p
}

func stringDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func parseServiceKind(s string) ServiceKind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "application":
		return KindApplication
	case "database":
		return KindDatabase
	case "external_api":
		return KindExternalAPI
	case "cache":
		return KindCache
	case "queue":
		return KindQueue
	default:
		return invalidServiceKind
	}
}

func parseProtocol(s string) Protocol {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "http":
		return ProtocolHTTP
	case "grpc":
		return ProtocolGRPC
	case "messaging":
		return ProtocolMessaging
	default:
		return invalidProtocol
	}
}

func parseBackoff(s string) BackoffPolicy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "exponential":
		return BackoffExponential
	case "linear":
		return BackoffLinear
	case "constant":
		return BackoffConstant
	default:
		return invalidBackoffPolicy
	}
}

func parseFaultKind(s string) FaultKind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "latency_inflation":
		return FaultLatencyInflation
	case "error_rate_override":
		return FaultErrorRateOverride
	case "disconnect":
		return FaultDisconnect
	case "crash":
		return FaultCrash
	default:
		return invalidFaultKind
	}
}

func parseExhaustedAction(s string) ExhaustedAction {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "propagate":
		return ExhaustedPropagate
	case "return_default":
		return ExhaustedReturnDefault
	case "succeed_silently":
		return ExhaustedSucceedSilently
	default:
		return invalidExhaustedAction
	}
}

func Validate(*Schema) error {
	return nil
}
