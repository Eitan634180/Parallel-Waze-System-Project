package store

import "nav-system/src/graph/model"

func (s *Store) checkDedup(id model.EdgeID, changed *[]ChangedEdge) {
	if s.snapshotDedup[id] {
		return
	}
	s.snapshotDedup[id] = true

	cur := s.weight[id]
	prev := s.prev[id]
	delta := cur - prev
	if delta < 0 {
		delta = -delta
	}

	density := s.density[id]
	densityChanged := density != s.prevDensity[id]

	if delta >= SignificantShift || densityChanged {
		*changed = append(*changed, ChangedEdge{EdgeID: id, OldMultiplier: prev, NewMultiplier: cur})
		s.prev[id] = cur
		s.prevDensity[id] = density
	}
}

// DirtySnapshot returns edges whose multiplier or density changed since the previous call.
// Safe to call concurrently with EnterEdge/LeaveEdge/RecordObservation.
func (s *Store) DirtySnapshot() []ChangedEdge {
	s.mu.Lock()
	defer s.mu.Unlock()

	var changed []ChangedEdge
	for _, id := range s.dirtyEdges {
		s.checkDedup(id, &changed)
	}
	for _, id := range s.activityEdges {
		s.checkDedup(id, &changed)
	}

	// Cleanup dedup state for the next call
	for _, id := range s.dirtyEdges {
		s.snapshotDedup[id] = false
	}
	for _, id := range s.activityEdges {
		s.snapshotDedup[id] = false
		s.isActive[id] = false
	}

	s.activityEdges = s.activityEdges[:0]

	return changed
}

// SwapPending atomically takes ownership of the pending set and replaces it
// with a fresh empty slice. Used by the customization loop to find edges whose weights changed.
func (s *Store) SwapPending() []model.EdgeID {
	s.mu.Lock()
	pending := s.pendingEdges
	s.pendingEdges = nil
	for _, id := range pending {
		s.isPending[id] = false
	}
	s.mu.Unlock()
	return pending
}

// RefillPendingForBenchmarks artificially repopulates the pending queue with all
// currently dirty edges. This is strictly for synthetic benchmarks where ApplyDecay
// does not naturally run between customization cycles.
func (s *Store) RefillPendingForBenchmarks() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingEdges = s.pendingEdges[:0]
	for _, id := range s.dirtyEdges {
		if !s.isPending[id] {
			s.isPending[id] = true
			s.pendingEdges = append(s.pendingEdges, id)
		}
	}
}
