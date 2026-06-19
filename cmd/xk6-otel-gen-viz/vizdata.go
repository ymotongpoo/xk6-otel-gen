// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

type vizData struct {
	Namespace string       `json:"namespace"`
	Nodes     []vizNode    `json:"nodes"`
	Edges     []vizEdge    `json:"edges"`
	Journeys  []vizJourney `json:"journeys"`
	Faults    []vizFault   `json:"faults"`
}

type vizNode struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Replicas   int            `json:"replicas"`
	Language   string         `json:"language"`
	Framework  string         `json:"framework"`
	Version    string         `json:"version"`
	Operations []vizOperation `json:"operations"`
}

type vizOperation struct {
	Name string `json:"name"`
}

type vizEdge struct {
	ID         string  `json:"id"`
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	Protocol   string  `json:"protocol"`
	FromOp     string  `json:"fromOp"`
	ToOp       string  `json:"toOp"`
	LatencyP50 string  `json:"latencyP50,omitempty"`
	LatencyP95 string  `json:"latencyP95,omitempty"`
	ErrorRate  float64 `json:"errorRate,omitempty"`
	Retries    int     `json:"retries,omitempty"`
}

type vizJourney struct {
	Name           string   `json:"name"`
	Weight         float64  `json:"weight"`
	ReachableNodes []string `json:"reachableNodes"`
	ReachableEdges []string `json:"reachableEdges"`
}

type vizFault struct {
	TargetKind string             `json:"targetKind"`
	TargetID   string             `json:"targetID"`
	FaultKind  string             `json:"faultKind"`
	Severity   vizSeverity        `json:"severity"`
	Schedule   []vizSchedulePoint `json:"schedule,omitempty"`
}

type vizSeverity struct {
	Probability float64 `json:"probability"`
	Multiplier  float64 `json:"multiplier,omitempty"`
	AddMs       float64 `json:"addMs,omitempty"`
	Value       float64 `json:"value,omitempty"`
}

type vizSchedulePoint struct {
	AtSeconds float64 `json:"atSeconds"`
	Intensity float64 `json:"intensity"`
}

func buildVizData(schema *topology.Schema) *vizData {
	if schema == nil {
		return &vizData{}
	}

	data := &vizData{
		Namespace: schema.Namespace,
		Nodes:     collectNodes(schema),
		Edges:     collectAllEdges(schema),
		Journeys:  collectJourneys(schema),
		Faults:    collectFaults(schema),
	}
	return data
}

func collectNodes(schema *topology.Schema) []vizNode {
	serviceIDs := make([]string, 0, len(schema.Services))
	for id := range schema.Services {
		serviceIDs = append(serviceIDs, string(id))
	}
	sort.Strings(serviceIDs)

	nodes := make([]vizNode, 0, len(serviceIDs))
	for _, id := range serviceIDs {
		svc := schema.Services[topology.ServiceID(id)]
		if svc == nil {
			continue
		}

		opNames := make([]string, 0, len(svc.Operations))
		for name := range svc.Operations {
			opNames = append(opNames, name)
		}
		sort.Strings(opNames)

		ops := make([]vizOperation, len(opNames))
		for i, name := range opNames {
			ops[i] = vizOperation{Name: name}
		}

		nodes = append(nodes, vizNode{
			ID:         string(svc.Name),
			Kind:       svc.Kind.String(),
			Replicas:   svc.Replicas,
			Language:   svc.Language,
			Framework:  svc.Framework,
			Version:    svc.Version,
			Operations: ops,
		})
	}
	return nodes
}

func collectAllEdges(schema *topology.Schema) []vizEdge {
	var edges []vizEdge
	for _, svc := range schema.Services {
		if svc == nil {
			continue
		}
		for _, op := range svc.Operations {
			if op == nil {
				continue
			}
			collectEdges(op.Calls, func(edge *topology.Edge) {
				if edge == nil || edge.From == nil || edge.To == nil {
					return
				}
				edges = append(edges, buildVizEdge(edge))
			})
		}
	}
	return edges
}

func collectEdges(calls []*topology.CallNode, fn func(*topology.Edge)) {
	for _, node := range calls {
		if node == nil {
			continue
		}
		if node.Edge != nil {
			fn(node.Edge)
			if node.Edge.OnFailure != nil {
				for _, fallback := range node.Edge.OnFailure.Fallback {
					if fallback != nil {
						fn(fallback)
					}
				}
			}
		}
		collectEdges(node.Parallel, fn)
	}
}

func buildVizEdge(edge *topology.Edge) vizEdge {
	ve := vizEdge{
		ID:        edgeID(edge),
		Source:    string(edge.From.Service.Name),
		Target:    string(edge.To.Service.Name),
		Protocol:  edge.Protocol.String(),
		FromOp:    edge.From.Name,
		ToOp:      edge.To.Name,
		ErrorRate: edge.ErrorRate,
		Retries:   edge.Retries,
	}
	if edge.Latency.P50 != 0 {
		ve.LatencyP50 = edge.Latency.P50.String()
	}
	if edge.Latency.P95 != 0 {
		ve.LatencyP95 = edge.Latency.P95.String()
	}
	return ve
}

func edgeID(edge *topology.Edge) string {
	return fmt.Sprintf("%s.%s->%s.%s",
		edge.From.Service.Name, edge.From.Name,
		edge.To.Service.Name, edge.To.Name)
}

func collectJourneys(schema *topology.Schema) []vizJourney {
	names := make([]string, 0, len(schema.Journeys))
	for name := range schema.Journeys {
		names = append(names, name)
	}
	sort.Strings(names)

	journeys := make([]vizJourney, 0, len(names))
	for _, name := range names {
		j := schema.Journeys[name]
		if j == nil {
			continue
		}
		nodeIDs, edgeIDs := journeyReachability(j)
		journeys = append(journeys, vizJourney{
			Name:           name,
			Weight:         j.Weight,
			ReachableNodes: nodeIDs,
			ReachableEdges: edgeIDs,
		})
	}
	return journeys
}

func journeyReachability(j *topology.Journey) (nodeIDs []string, edgeIDs []string) {
	visitedOps := make(map[*topology.Operation]bool)
	nodeSet := make(map[string]bool)
	edgeSet := make(map[string]bool)

	var walkOp func(op *topology.Operation)
	walkOp = func(op *topology.Operation) {
		if op == nil || visitedOps[op] {
			return
		}
		visitedOps[op] = true
		nodeSet[string(op.Service.Name)] = true

		collectEdges(op.Calls, func(e *topology.Edge) {
			if e == nil || e.To == nil {
				return
			}
			edgeSet[edgeID(e)] = true
			walkOp(e.To)
		})
	}

	var walkSteps func(steps []*topology.Step)
	walkSteps = func(steps []*topology.Step) {
		for _, step := range steps {
			if step == nil {
				continue
			}
			if step.Op != nil {
				walkOp(step.Op)
			}
			if len(step.Parallel) > 0 {
				walkSteps(step.Parallel)
			}
		}
	}

	walkSteps(j.Steps)
	return mapKeysSorted(nodeSet), mapKeysSorted(edgeSet)
}

func mapKeysSorted(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func collectFaults(schema *topology.Schema) []vizFault {
	faults := make([]vizFault, 0, len(schema.Faults))
	for _, fault := range schema.Faults {
		faults = append(faults, buildVizFault(fault))
	}
	return faults
}

func buildVizFault(fault topology.FaultSpec) vizFault {
	vf := vizFault{
		TargetKind: fault.Target.Kind.String(),
		TargetID:   faultTargetID(fault.Target),
		FaultKind:  fault.Kind.String(),
		Severity: vizSeverity{
			Probability: fault.Severity.Probability,
			Multiplier:  fault.Severity.Multiplier,
			Value:       fault.Severity.Value,
		},
	}
	if fault.Severity.Add != 0 {
		vf.Severity.AddMs = fault.Severity.Add.Seconds() * 1000
	}
	if len(fault.Schedule) > 0 {
		vf.Schedule = make([]vizSchedulePoint, len(fault.Schedule))
		for i, pt := range fault.Schedule {
			vf.Schedule[i] = vizSchedulePoint{
				AtSeconds: durationSeconds(pt.At),
				Intensity: pt.Intensity,
			}
		}
	}
	return vf
}

func faultTargetID(target topology.FaultTarget) string {
	switch target.Kind {
	case topology.TargetNode:
		if target.Service != nil {
			return string(target.Service.Name)
		}
	case topology.TargetOperation:
		if target.Operation != nil && target.Operation.Service != nil {
			return fmt.Sprintf("%s.%s", target.Operation.Service.Name, target.Operation.Name)
		}
	case topology.TargetEdge:
		if target.Edge != nil {
			return edgeID(target.Edge)
		}
	}
	return ""
}

func durationSeconds(d time.Duration) float64 {
	return d.Seconds()
}
