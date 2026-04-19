package session

import (
	"context"
	"sync"

	"nav-system/src/graph/builder"
	"nav-system/src/traffic"
	"nav-system/src/utilities"
)

const propagationJobQueueFactor = 4

type propagationJob struct {
	session       *Session
	improvedEdges []traffic.ChangedEdge
}

// RunPropagation broadcasts speed updates for edges whose live traffic changed.
func (m *Manager) RunPropagation(ctx context.Context, store *traffic.Store, g *builder.Graph) {
	workerCount := optimizationWorkerLimit
	jobs := make(chan propagationJob, workerCount*propagationJobQueueFactor)

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				flagBetterRouteIfHelpful(job.session, job.improvedEdges, store, g)
			}
		}()
	}

	ticker := newTicker(propagationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case <-ticker.C:
			m.propagate(ctx, jobs, store, g)
		}
	}
}

func (m *Manager) propagate(ctx context.Context, jobs chan<- propagationJob, store *traffic.Store, g *builder.Graph) {
	changedEdges := store.DirtySnapshot()
	improvedEdges := significantlyImprovedEdges(changedEdges)
	if len(improvedEdges) > 0 {
		active := m.activeSessions()
		for _, session := range active {
			select {
			case <-ctx.Done():
				return
			case jobs <- propagationJob{session: session, improvedEdges: improvedEdges}:
			default:
				// Drop job if queue is full (we'll try again on next tick)
			}
		}
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

func flagBetterRouteIfHelpful(s *Session, improvedEdges []traffic.ChangedEdge, store *traffic.Store, g *builder.Graph) {
	snapshot, ok := heuristicSnapshot(s, g)
	if !ok {
		return
	}

	carX, carY := utilities.ProjectAtReferenceLat(snapshot.carLat, snapshot.carLon, g.ProjectionRefLat)

	for _, changed := range improvedEdges {
		edge, ok := graphEdge(g, changed.EdgeID)
		if !ok {
			continue
		}

		fromNode := &g.Nodes[edge.FromNodeIdx]
		toNode := &g.Nodes[edge.ToNodeIdx]
		if fromNode == nil || toNode == nil {
			continue
		}

		distCarToEdge := utilities.Distance(carX, carY, fromNode.X, fromNode.Y)
		distEdgeToDestination := utilities.Distance(toNode.X, toNode.Y, snapshot.destination.X, snapshot.destination.Y)
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
	carLat      float64
	carLon      float64
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

	var destinationNode *builder.Node
	if destinationStep.NodeIdx < uint32(len(g.Nodes)) {
		destinationNode = &g.Nodes[destinationStep.NodeIdx]
	}
	if destinationNode == nil {
		return heuristicSessionSnapshot{}, false
	}

	return heuristicSessionSnapshot{
		carLat:      s.LastLat,
		carLon:      s.LastLon,
		destination: destinationNode,
		eta:         s.ETA,
	}, true
}
