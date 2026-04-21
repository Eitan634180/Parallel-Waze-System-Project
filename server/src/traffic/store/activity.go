package store

import "nav-system/src/graph/model"

// EnterEdge increments the active-session count for an edge.
func (s *Store) EnterEdge(id model.EdgeID) {
	s.mu.Lock()
	s.ensure(id)
	s.density[id]++
	if !s.isActive[id] {
		s.isActive[id] = true
		s.activityEdges = append(s.activityEdges, id)
	}
	s.mu.Unlock()
}

// LeaveEdge decrements the active-session count for an edge.
func (s *Store) LeaveEdge(id model.EdgeID) {
	s.mu.Lock()
	s.ensure(id)
	d := s.density[id] - 1
	if d <= 0 {
		s.density[id] = 0
	} else {
		s.density[id] = d
	}
	if !s.isActive[id] {
		s.isActive[id] = true
		s.activityEdges = append(s.activityEdges, id)
	}
	s.mu.Unlock()
}

// Density returns the current number of active sessions on an edge.
func (s *Store) Density(id model.EdgeID) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if int(id) >= len(s.density) {
		return 0
	}
	return s.density[id]
}
