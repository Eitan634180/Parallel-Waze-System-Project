package reroute

import (
	"log"
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/navigation"
	navigationanalysis "nav-system/src/navigation/analysis"
	navigationsessions "nav-system/src/navigation/sessions"
	routingengine "nav-system/src/routing/engine"
	routingentities "nav-system/src/routing/entities"
	trafficstore "nav-system/src/traffic/store"
)

type SessionVersion struct {
	StepIdx       int
	RouteRevision uint64
}

func DoReroute(
	s *navigationsessions.Session,
	lat, lon float64,
	g *model.Graph,
	store *trafficstore.Store,
	mgr *navigationsessions.Manager,
	router *routingengine.Router,
	wf routingengine.WeightFunc,
	prepareRoute func(routingentities.Route) routingentities.Route,
	now time.Time,
	reason string,
	oldETA *float32,
	newETA *float32,
) {
	s.Mu.RLock()
	destination, ok := s.Route.Destination()
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
	s *navigationsessions.Session,
	candidate routingentities.Route,
	g *model.Graph,
	store *trafficstore.Store,
	mgr *navigationsessions.Manager,
	prepareRoute func(routingentities.Route) routingentities.Route,
	now time.Time,
	reason string,
	oldETA *float32,
	newETA *float32,
	expectedVersion *SessionVersion,
) routingentities.Route {
	if sessionVersionChanged(s, expectedVersion) {
		return routingentities.Route{}
	}

	candidate.CongestionAhead, candidate.CongestedEdges = navigationanalysis.RouteCongestionSummary(candidate, store, g)
	route := prepareRoute(candidate)
	if sessionVersionChanged(s, expectedVersion) {
		return routingentities.Route{}
	}

	stepIdx := route.InitialStepIndex()
	if reason == navigation.RerouteReasonLocalPatch && expectedVersion != nil {
		stepIdx = expectedVersion.StepIdx
	}

	if !replaceSessionRoute(s, route, stepIdx, store, mgr, now, reason, expectedVersion) {
		return routingentities.Route{}
	}
	sendRerouteMessage(s, route, reason, oldETA, newETA)
	sendCurrentSpeedHints(s, store, g)

	return route
}

func sessionVersionChanged(s *navigationsessions.Session, expected *SessionVersion) bool {
	if expected == nil {
		return false
	}

	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return s.StepIdx != expected.StepIdx || s.RouteRevision != expected.RouteRevision
}

func replaceSessionRoute(
	s *navigationsessions.Session,
	route routingentities.Route,
	stepIdx int,
	store *trafficstore.Store,
	mgr *navigationsessions.Manager,
	now time.Time,
	reason string,
	expectedVersion *SessionVersion,
) bool {
	var oldEdgeID, newEdgeID *uint32
	var ok bool
	if expectedVersion != nil {
		oldEdgeID, newEdgeID, ok = mgr.UpdateRouteAtStepIfCurrent(
			s,
			route,
			stepIdx,
			expectedVersion.StepIdx,
			expectedVersion.RouteRevision,
		)
	} else {
		s.Mu.RLock()
		currentStepIdx := s.StepIdx
		currentRouteRevision := s.RouteRevision
		s.Mu.RUnlock()
		oldEdgeID, newEdgeID, ok = mgr.UpdateRouteAtStepIfCurrent(s, route, stepIdx, currentStepIdx, currentRouteRevision)
	}
	if !ok {
		return false
	}

	if oldEdgeID != nil {
		store.LeaveEdge(model.EdgeID(*oldEdgeID))
	}
	if newEdgeID != nil {
		store.EnterEdge(model.EdgeID(*newEdgeID))
	}

	s.Mu.Lock()
	s.ETA = route.TotalTimeSec
	s.LastReroute = now
	s.LastRerouteReason = reason
	s.OffRouteViolations = 0
	s.Mu.Unlock()
	return true
}
