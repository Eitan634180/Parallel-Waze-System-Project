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
	s.Mu.RUnlock()
	if !ok {
		return
	}

	start := time.Now()
	newRoutes := router.Compute(lat, lon, destination.Lat, destination.Lon, 1, wf)
	log.Printf("[session] reroute compute reason=%s session=%s routes=%d in %s", reason, s.ID, len(newRoutes), time.Since(start).Round(time.Millisecond))
	if len(newRoutes) == 0 {
		return
	}

	applyRouteUpdate(s, newRoutes[0], g, store, mgr, prepareRoute, now, reason, oldETA, newETA)
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
) routing.Route {
	candidate.CongestionAhead, candidate.CongestedEdges = RouteCongestionSummary(candidate, store, g)
	route := prepareRoute(candidate)

	replaceSessionRoute(s, route, store, mgr, now, reason)
	sendRerouteMessage(s, route, reason, oldETA, newETA)
	sendCurrentSpeedHints(s, store, g)

	return route
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
