package session

import (
	"time"

	"nav-system/internal/graph/builder"
	"nav-system/internal/routing"
	"nav-system/internal/traffic"
)

type routeAssessment struct {
	allowReroute       bool
	shouldRerouteNow   bool
	congestionAhead    bool
	offRouteDistanceM  float32
	congestedEdgeCount int
}

type congestionContext struct {
	currentRoute routing.Route
	destination  routing.Step
	oldETA       float32
	localRepair  localRepairRequest
}

type localRepairRequest struct {
	enabled       bool
	repairStepIdx int
	congestedCost float32
	fromNodeID    builder.NodeID
	toNodeID      builder.NodeID
	isCrossCell   bool
}

// Check evaluates reroute conditions and pushes ETA / reroute events.
func Check(
	s *Session,
	snapLat, snapLon float64,
	g *builder.Graph,
	store *traffic.Store,
	mgr *Manager,
	router *routing.Router,
	wf routing.WeightFunc,
	prepareRoute func(routing.Route) routing.Route,
) {
	now := time.Now()
	pushETAIfDue(s, g, store, now)

	assessment := refreshRouteAssessment(s, snapLat, snapLon, g, store, now)
	if !assessment.allowReroute {
		return
	}

	if assessment.shouldRerouteNow {
		doReroute(s, snapLat, snapLon, g, store, mgr, router, wf, prepareRoute, now, rerouteReasonOffRoute, nil, nil)
		return
	}

	if !assessment.congestionAhead {
		return
	}

	attemptCongestionReroute(s, snapLat, snapLon, g, store, mgr, router, wf, prepareRoute, now)
}
