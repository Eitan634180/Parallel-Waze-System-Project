package session

import (
	"log"
	"time"

	"nav-system/internal/graph/builder"
	"nav-system/internal/routing"
	"nav-system/internal/traffic"
)

func doReroute(
	s *Session,
	lat, lon float64,
	g *builder.Graph,
	store *traffic.Store,
	mgr *Manager,
	router *routing.Router,
	wf routing.WeightFunc,
	prepareRoute func(routing.Route) routing.Route,
	now time.Time,
	reason string,
	oldETA *float32,
	newETA *float32,
) {
	s.Mu.RLock()
	destination, ok := routeDestination(s.Route)
	version := sessionVersion{stepIdx: s.StepIdx, routeID: s.Route.ID}
	s.Mu.RUnlock()
	if !ok {
		return
	}

	start := time.Now()
	newRoutes := router.Compute(lat, lon, destination.Lat, destination.Lon, 1, wf)
	elapsed := time.Since(start)
	if elapsed >= slowComputeLogThreshold {
		log.Printf("session: reroute compute slow (reason=%s session=%s routes=%d duration=%s)", reason, s.ID, len(newRoutes), elapsed.Round(time.Millisecond))
	}
	if len(newRoutes) == 0 {
		return
	}

	applyRouteUpdate(s, newRoutes[0], g, store, mgr, prepareRoute, now, reason, oldETA, newETA, &version)
}

func applyRouteUpdate(
	s *Session,
	candidate routing.Route,
	g *builder.Graph,
	store *traffic.Store,
	mgr *Manager,
	prepareRoute func(routing.Route) routing.Route,
	now time.Time,
	reason string,
	oldETA *float32,
	newETA *float32,
	expectedVersion *sessionVersion,
) routing.Route {
	candidate.CongestionAhead, candidate.CongestedEdges = RouteCongestionSummary(candidate, store, g)
	route := prepareRoute(candidate)
	if sessionVersionChanged(s, expectedVersion) {
		return routing.Route{}
	}

	replaceSessionRoute(s, route, store, mgr, now, reason)
	sendRerouteMessage(s, route, reason, oldETA, newETA)
	sendCurrentSpeedHints(s, store, g)

	return route
}

func sessionVersionChanged(s *Session, expected *sessionVersion) bool {
	if expected == nil {
		return false
	}

	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return s.StepIdx != expected.stepIdx || s.Route.ID != expected.routeID
}

func replaceSessionRoute(s *Session, route routing.Route, store *traffic.Store, mgr *Manager, now time.Time, reason string) {
	s.Mu.RLock()
	currentEdgeID := s.CurrentEdgeID
	s.Mu.RUnlock()
	if currentEdgeID != nil {
		store.LeaveEdge(builder.EdgeID(*currentEdgeID))
	}

	mgr.UpdateRoute(s, route)

	s.Mu.Lock()
	if s.CurrentEdgeID != nil {
		store.EnterEdge(builder.EdgeID(*s.CurrentEdgeID))
	}
	s.ETA = route.TotalTimeSec
	s.LastReroute = now
	s.LastRerouteReason = reason
	s.OffRouteViolations = 0
	s.Mu.Unlock()
}
