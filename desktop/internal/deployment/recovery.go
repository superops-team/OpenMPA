package deployment

import "sort"

// ListUpdates discovers saved receipts under the current authoritative binding.
// It never calls the cloud, rewrites the ledger or creates a directory/lock.
// These are historical receipts; ReadUpdate checks current platform state.
func (s *Service) ListUpdates(req ScopeRequest) OperationListResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, e := s.resolve(req.ConnectionEpoch)
	if e != nil {
		return OperationListResult{Error: e}
	}
	if t.Scope == "" {
		return OperationListResult{Error: failure("notConnected", 0)}
	}
	s.ensureLedger()
	if s.ledgerError {
		return OperationListResult{Error: failure("unavailable", 0)}
	}
	value := OperationList{ConnectionEpoch: t.ConnectionEpoch, Operations: []Operation{}}
	for _, op := range s.operations {
		if !sameResource(op.target, t) || (t.ToolID != "" && op.operation.ToolID != t.ToolID) {
			continue
		}
		copy := op.operation
		copy.ConnectionEpoch = t.ConnectionEpoch
		value.Operations = append(value.Operations, copy)
	}
	if !s.current(t) {
		return OperationListResult{Error: failure("staleConnection", 0)}
	}
	sort.Slice(value.Operations, func(i, j int) bool {
		a, b := value.Operations[i], value.Operations[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.OperationID < b.OperationID
	})
	return OperationListResult{OK: true, Value: &value}
}
