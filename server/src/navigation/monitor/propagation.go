package monitor

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/navigation"
	navigationmanager "nav-system/src/navigation/manager"
	navigationsession "nav-system/src/navigation/session"
	"nav-system/src/routing/engine"
	routinggeometry "nav-system/src/routing/geometry"
	trafficpropagation "nav-system/src/traffic/propagation"
	trafficstore "nav-system/src/traffic/store"
	"nav-system/src/utilities"
)

type heuristicSessionSnapshot struct {
	carLat      float64
	carLon      float64
	destination *model.Node
	eta         float32
}

func RunPropagation(ctx context.Context, mgr *navigationmanager.Manager, store *trafficstore.Store, g *model.Graph) {
	ticker := time.NewTicker(navigation.PropagationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			propagate(ctx, mgr, store, g)
		}
	}
}

func propagate(ctx context.Context, mgr *navigationmanager.Manager, store *trafficstore.Store, g *model.Graph) {
	changedEdges := store.DirtySnapshot()
	improvedEdges := trafficpropagation.ImprovedEdges(changedEdges)
	if len(improvedEdges) > 0 {
		flagBetterRoutes(ctx, mgr.ActiveSessions(), improvedEdges, store, g)
	}

	for _, update := range trafficpropagation.RecommendedSpeedUpdates(g, store, changedEdges) {
		broadcastSpeedUpdate(mgr, update.EdgeID, update.RecommendedSpeedKmh)
	}
}

func flagBetterRoutes(
	ctx context.Context,
	sessions []*navigationsession.Session,
	improvedEdges []trafficstore.ChangedEdge,
	store *trafficstore.Store,
	g *model.Graph,
) {
	if len(sessions) == 0 {
		return
	}

	workerCount := min(len(sessions), navigation.OptimizationWorkerLimit)
	if workerCount < 1 {
		workerCount = 1
	}

	var next atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				start := int(next.Add(propagationBatchChunkSize) - propagationBatchChunkSize)
				if start >= len(sessions) {
					return
				}

				end := start + propagationBatchChunkSize
				if end > len(sessions) {
					end = len(sessions)
				}

				for _, sess := range sessions[start:end] {
					select {
					case <-ctx.Done():
						return
					default:
					}
					flagBetterRouteIfHelpful(sess, improvedEdges, store, g)
				}
			}
		}()
	}
	wg.Wait()
}

func broadcastSpeedUpdate(mgr *navigationmanager.Manager, edgeID model.EdgeID, recommendedSpeed float32) {
	edgeIDValue := uint32(edgeID)
	recommendedSpeedValue := recommendedSpeed

	for _, sessionID := range mgr.SubscribersOf(edgeID) {
		sess := mgr.Get(sessionID)
		if sess == nil || !sessionHasConnection(sess) {
			continue
		}

		_ = sess.Send(navigationsession.OutMsg{
			Type:                "speed_update",
			EdgeID:              &edgeIDValue,
			RecommendedSpeedKmh: &recommendedSpeedValue,
		})
	}
}

func sessionHasConnection(s *navigationsession.Session) bool {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return s.Conn != nil
}

func flagBetterRouteIfHelpful(s *navigationsession.Session, improvedEdges []trafficstore.ChangedEdge, store *trafficstore.Store, g *model.Graph) {
	snapshot, ok := heuristicSnapshot(s, g)
	if !ok {
		return
	}

	carX, carY := utilities.ProjectAtReferenceLat(snapshot.carLat, snapshot.carLon, g.ProjectionRefLat)

	for _, changed := range improvedEdges {
		edge, ok := routinggeometry.EdgeByID(g, changed.EdgeID)
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
			(distCarToEdge / navigation.MaxHeuristicSpeedMps) +
				store.LiveWeight(changed.EdgeID, edge.Weight) +
				(distEdgeToDestination / navigation.MaxHeuristicSpeedMps)

		if idealETA < snapshot.eta {
			s.Mu.Lock()
			s.CheckBetterRoute = true
			s.Mu.Unlock()
			return
		}
	}
}

func heuristicSnapshot(s *navigationsession.Session, g *model.Graph) (heuristicSessionSnapshot, bool) {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	destinationStep, ok := engine.Destination(s.Route)
	if !ok {
		return heuristicSessionSnapshot{}, false
	}

	var destinationNode *model.Node
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
