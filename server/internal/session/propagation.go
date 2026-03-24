package session

import (
	"context"

	"nav-system/internal/geo"
	"nav-system/internal/graph/builder"
	"nav-system/internal/traffic"
)

// RunPropagation broadcasts speed updates for edges whose live traffic changed.
func (m *Manager) RunPropagation(ctx context.Context, store *traffic.Store, g *builder.Graph) {
	ticker := newTicker(propagationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.propagate(store, g)
		}
	}
}

func (m *Manager) propagate(store *traffic.Store, g *builder.Graph) {
	changedEdges := store.DirtySnapshot()
	improvedEdges := significantlyImprovedEdges(changedEdges)
	if len(improvedEdges) > 0 {
		go evaluateHeuristics(m.activeSessions(), improvedEdges, store, g)
	}

	for _, changed := range changedEdges {
		edge, ok := graphEdge(g, changed.EdgeID)
		if !ok || edge.SpeedKmh <= 0 {
			continue
		}

		recommendedSpeed := store.RecommendedSpeedKmh(changed.EdgeID, edge.SpeedKmh, edge.DistanceM)
		m.broadcastSpeedUpdate(changed.EdgeID, recommendedSpeed)
	}
}

func significantlyImprovedEdges(changed []traffic.ChangedEdge) []traffic.ChangedEdge {
	improved := make([]traffic.ChangedEdge, 0, len(changed))
	for _, edge := range changed {
		if edge.OldMultiplier-edge.NewMultiplier >= traffic.SignificantShift {
			improved = append(improved, edge)
		}
	}
	return improved
}

func (m *Manager) activeSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

func (m *Manager) broadcastSpeedUpdate(edgeID builder.EdgeID, recommendedSpeed float32) {
	edgeIDValue := uint32(edgeID)
	recommendedSpeedValue := recommendedSpeed

	for _, sessionID := range m.SubscribersOf(edgeID) {
		session := m.Get(sessionID)
		if session == nil || !sessionHasConnection(session) {
			continue
		}

		_ = session.Send(OutMsg{
			Type:                "speed_update",
			EdgeID:              &edgeIDValue,
			RecommendedSpeedKmh: &recommendedSpeedValue,
		})
	}
}

func sessionHasConnection(s *Session) bool {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return s.Conn != nil
}

func evaluateHeuristics(sessions []*Session, improvedEdges []traffic.ChangedEdge, store *traffic.Store, g *builder.Graph) {
	workerCount := len(sessions)
	if workerCount == 0 {
		return
	}
	if workerCount > optimizationWorkerLimit {
		workerCount = optimizationWorkerLimit
	}

	chunkSize := (len(sessions) + workerCount - 1) / workerCount
	done := make(chan struct{}, workerCount)

	for start := 0; start < len(sessions); start += chunkSize {
		end := start + chunkSize
		if end > len(sessions) {
			end = len(sessions)
		}

		go func(batch []*Session) {
			defer func() { done <- struct{}{} }()
			for _, session := range batch {
				flagBetterRouteIfHelpful(session, improvedEdges, store, g)
			}
		}(sessions[start:end])
	}

	for workers := 0; workers < workerCount; workers++ {
		<-done
	}
}

func flagBetterRouteIfHelpful(s *Session, improvedEdges []traffic.ChangedEdge, store *traffic.Store, g *builder.Graph) {
	snapshot, ok := heuristicSnapshot(s, g)
	if !ok {
		return
	}

	for _, changed := range improvedEdges {
		edge, ok := graphEdge(g, changed.EdgeID)
		if !ok {
			continue
		}

		fromNode := g.NodeByID(edge.FromNodeID)
		toNode := g.NodeByID(edge.ToNodeID)
		if fromNode == nil || toNode == nil {
			continue
		}

		distCarToEdge := geo.Distance(snapshot.carX, snapshot.carY, fromNode.X, fromNode.Y)
		distEdgeToDestination := geo.Distance(toNode.X, toNode.Y, snapshot.destination.X, snapshot.destination.Y)
		idealETA :=
			(distCarToEdge / maxHeuristicSpeedMps) +
				store.LiveWeight(changed.EdgeID, edge.Weight) +
				(distEdgeToDestination / maxHeuristicSpeedMps)

		if idealETA < snapshot.eta {
			s.Mu.Lock()
			s.CheckBetterRoute = true
			s.Mu.Unlock()
			return
		}
	}
}

type heuristicSessionSnapshot struct {
	carX        float32
	carY        float32
	destination *builder.Node
	eta         float32
}

func heuristicSnapshot(s *Session, g *builder.Graph) (heuristicSessionSnapshot, bool) {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	destinationStep, ok := routeDestination(s.Route)
	if !ok {
		return heuristicSessionSnapshot{}, false
	}

	destinationNode := g.NodeByID(destinationStep.NodeID)
	if destinationNode == nil {
		return heuristicSessionSnapshot{}, false
	}

	carX, carY := geo.Project(s.LastLat, s.LastLon)
	return heuristicSessionSnapshot{
		carX:        carX,
		carY:        carY,
		destination: destinationNode,
		eta:         s.ETA,
	}, true
}
