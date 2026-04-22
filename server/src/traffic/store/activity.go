package store

import "nav-system/src/graph/model"

// EnterEdge increments the active-session count for an edge.
func (s *Store) EnterEdge(id model.EdgeID) {
	s.metaMu.Lock()
	data := s.ensureLocked(id)
	data.density[id].Add(1)
	if !s.isActive[id] {
		s.isActive[id] = true
		s.activityEdges = append(s.activityEdges, id)
	}
	s.metaMu.Unlock()
}

// LeaveEdge decrements the active-session count for an edge.
func (s *Store) LeaveEdge(id model.EdgeID) {
	s.metaMu.Lock()
	data := s.ensureLocked(id)
	d := data.density[id].Load() - 1
	if d <= 0 {
		data.density[id].Store(0)
	} else {
		data.density[id].Store(d)
	}
	if !s.isActive[id] {
		s.isActive[id] = true
		s.activityEdges = append(s.activityEdges, id)
	}
	s.metaMu.Unlock()
}

// Density returns the current number of active sessions on an edge.
func (s *Store) Density(id model.EdgeID) int {
	return int(loadDensity(s.data.Load(), id))
}
