package reroute

import (
	"log"
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/navigation"
	"nav-system/src/navigation/internal/routeutil"
	navigationmanager "nav-system/src/navigation/manager"
	navigationsession "nav-system/src/navigation/session"
	"nav-system/src/routing"
	"nav-system/src/routing/engine"
	trafficstore "nav-system/src/traffic/store"
)

type SessionVersion struct {
	StepIdx       int
	RouteRevision uint64
}

func DoReroute(
	s *navigationsession.Session,
	lat, lon float64,
	g *model.Graph,
	store *trafficstore.Store,
	mgr *navigationmanager.Manager,
	router *routing.Router,
	wf routing.WeightFunc,
	prepareRoute func(routing.Route) routing.Route,
	now time.Time,
	reason string,
	oldETA *float32,
	newETA *float32,
) {
	s.Mu.RLock()
	destination, ok := engine.Destination(s.Route)
	version := SessionVersion{StepIdx: s.StepIdx, RouteRevision: s.RouteRevision}
	s.Mu.RUnlock()
	if !ok {
		return
	}

	start := time.Now()
	newRoutes := router.Compute(lat, lon, destination.Lat, destination.Lon, 1, wf)
	elapsed := time.Since(start)
	if elapsed >= navigation.SlowComputeLogThreshold {
		log.Printf("session: reroute compute slow (reason=%s session=%s routes=%d duration=%s)", reason, s.ID, len(newRoutes), elapsed.Round(time.Millisecond))
	}
	if len(newRoutes) == 0 {
		return
	}

	ApplyRouteUpdate(s, newRoutes[0], g, store, mgr, prepareRoute, now, reason, oldETA, newETA, &version)
}

func ApplyRouteUpdate(
	s *navigationsession.Session,
	candidate routing.Route,
	g *model.Graph,
	store *trafficstore.Store,
	mgr *navigationmanager.Manager,
	prepareRoute func(routing.Route) routing.Route,
	now time.Time,
	reason string,
	oldETA *float32,
	newETA *float32,
	expectedVersion *SessionVersion,
) routing.Route {
	candidate.CongestionAhead, candidate.CongestedEdges = routeutil.RouteCongestionSummary(candidate, store, g)
	route := prepareRoute(candidate)
	if sessionVersionChanged(s, expectedVersion) {
		return routing.Route{}
	}

	replaceSessionRoute(s, route, store, mgr, now, reason)
	sendRerouteMessage(s, route, reason, oldETA, newETA)
	sendCurrentSpeedHints(s, store, g)

	return route
}

func sessionVersionChanged(s *navigationsession.Session, expected *SessionVersion) bool {
	if expected == nil {
		return false
	}

	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return s.StepIdx != expected.StepIdx || s.RouteRevision != expected.RouteRevision
}

func replaceSessionRoute(s *navigationsession.Session, route routing.Route, store *trafficstore.Store, mgr *navigationmanager.Manager, now time.Time, reason string) {
	s.Mu.RLock()
	currentEdgeID := s.CurrentEdgeID
	s.Mu.RUnlock()
	if currentEdgeID != nil {
		store.LeaveEdge(model.EdgeID(*currentEdgeID))
	}

	mgr.UpdateRoute(s, route)

	s.Mu.Lock()
	if s.CurrentEdgeID != nil {
		store.EnterEdge(model.EdgeID(*s.CurrentEdgeID))
	}
	s.ETA = route.TotalTimeSec
	s.LastReroute = now
	s.LastRerouteReason = reason
	s.OffRouteViolations = 0
	s.Mu.Unlock()
}
