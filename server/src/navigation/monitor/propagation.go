package monitor

import (
	"context"
	"sync"
	"time"

	coreconfig "nav-system/src/core/config"
	navigationmanager "nav-system/src/navigation/manager"
	navigationsession "nav-system/src/navigation/session"
	"nav-system/src/utilities"
	"nav-system/src/graph/model"
	"nav-system/src/routing/engine"
	routinggeometry "nav-system/src/routing/geometry"
	trafficpropagation "nav-system/src/traffic/propagation"
	trafficstore "nav-system/src/traffic/store"
)

type propagationJob struct {
	session       *navigationsession.Session
	improvedEdges []trafficstore.ChangedEdge
}

type heuristicSessionSnapshot struct {
	carLat      float64
	carLon      float64
	destination *model.Node
	eta         float32
}

func RunPropagation(ctx context.Context, mgr *navigationmanager.Manager, store *trafficstore.Store, g *model.Graph) {
	workerCount := coreconfig.RoutingOptimizationWorkerLimit
	jobs := make(chan propagationJob, workerCount*coreconfig.RoutingPropagationJobQueueFactor)

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

	ticker := time.NewTicker(coreconfig.RoutingPropagationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case <-ticker.C:
			propagate(ctx, mgr, jobs, store, g)
		}
	}
}

func propagate(ctx context.Context, mgr *navigationmanager.Manager, jobs chan<- propagationJob, store *trafficstore.Store, g *model.Graph) {
	changedEdges := store.DirtySnapshot()
	improvedEdges := trafficpropagation.ImprovedEdges(changedEdges)
	if len(improvedEdges) > 0 {
		for _, sess := range mgr.ActiveSessions() {
			select {
			case <-ctx.Done():
				return
			case jobs <- propagationJob{session: sess, improvedEdges: improvedEdges}:
			default:
			}
		}
	}

	for _, update := range trafficpropagation.RecommendedSpeedUpdates(g, store, changedEdges) {
		broadcastSpeedUpdate(mgr, update.EdgeID, update.RecommendedSpeedKmh)
	}
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
			(distCarToEdge / coreconfig.RoutingMaxHeuristicSpeedMps) +
				store.LiveWeight(changed.EdgeID, edge.Weight) +
				(distEdgeToDestination / coreconfig.RoutingMaxHeuristicSpeedMps)

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
