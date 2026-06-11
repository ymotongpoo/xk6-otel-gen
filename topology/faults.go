// SPDX-License-Identifier: Apache-2.0

package topology

// ApplyFaults builds an overlay that indexes fault specs by target pointer.
func (s *Schema) ApplyFaults() *FaultOverlay {
	overlay := &FaultOverlay{
		nodeFaults:      make(map[*Service][]FaultSpec),
		operationFaults: make(map[*Operation][]FaultSpec),
		edgeFaults:      make(map[*Edge][]FaultSpec),
	}
	if s == nil {
		return overlay
	}

	for _, fault := range s.Faults {
		switch fault.Target.Kind {
		case TargetNode:
			if fault.Target.Service != nil {
				overlay.nodeFaults[fault.Target.Service] = append(overlay.nodeFaults[fault.Target.Service], fault)
			}
		case TargetOperation:
			if fault.Target.Operation != nil {
				overlay.operationFaults[fault.Target.Operation] = append(overlay.operationFaults[fault.Target.Operation], fault)
			}
		case TargetEdge:
			if fault.Target.Edge != nil {
				overlay.edgeFaults[fault.Target.Edge] = append(overlay.edgeFaults[fault.Target.Edge], fault)
			}
		}
	}
	return overlay
}

// NodeFaults returns fault specs targeting svc.
func (o *FaultOverlay) NodeFaults(svc *Service) []FaultSpec {
	if o == nil {
		return nil
	}
	return o.nodeFaults[svc]
}

// OperationFaults returns fault specs targeting op.
func (o *FaultOverlay) OperationFaults(op *Operation) []FaultSpec {
	if o == nil {
		return nil
	}
	return o.operationFaults[op]
}

// EdgeFaults returns fault specs targeting edge.
func (o *FaultOverlay) EdgeFaults(edge *Edge) []FaultSpec {
	if o == nil {
		return nil
	}
	return o.edgeFaults[edge]
}

// FaultOverlayEqual reports whether two overlays contain the same indexed faults.
func FaultOverlayEqual(a, b *FaultOverlay) bool {
	if a == nil || b == nil {
		return a == b
	}
	return equalFaultSpecMap(a.nodeFaults, b.nodeFaults) &&
		equalFaultSpecMap(a.operationFaults, b.operationFaults) &&
		equalFaultSpecMap(a.edgeFaults, b.edgeFaults)
}

func equalFaultSpecMap[K comparable](a, b map[K][]FaultSpec) bool {
	if len(a) != len(b) {
		return false
	}
	for key, aFaults := range a {
		bFaults, ok := b[key]
		if !ok || !equalFaults(aFaults, bFaults) {
			return false
		}
	}
	return true
}
