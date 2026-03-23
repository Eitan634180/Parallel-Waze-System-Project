package session

import (
	"time"

	"nav-system/internal/routing"
	"nav-system/internal/traffic"
	"nav-system/map/builder"
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
		doReroute(s, snapLat, snapLon, g, store, mgr, router, wf, prepareRoute, now, "off_route", nil, nil)
		return
	}

	if !assessment.congestionAhead {
		return
	}

	attemptCongestionReroute(s, snapLat, snapLon, g, store, mgr, router, wf, prepareRoute, now)
}

func pushETAIfDue(s *Session, g *builder.Graph, store *traffic.Store, now time.Time) {
	s.Mu.Lock()
	if now.Sub(s.LastETAPush) < etaThrottle {
		s.Mu.Unlock()
		return
	}

	eta := computeETALocked(s, g, store)
	s.ETA = eta
	s.LastETAPush = now
	s.Mu.Unlock()

	etaValue := eta
	_ = s.Send(OutMsg{Type: "eta_update", ETASec: &etaValue})
}

func refreshRouteAssessment(s *Session, snapLat, snapLon float64, g *builder.Graph, store *traffic.Store, now time.Time) routeAssessment {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	assessment := routeAssessment{}
	assessment.offRouteDistanceM = distanceFromExpectedPathMLocked(s, snapLat, snapLon, g)
	s.LastOffRouteDistanceM = assessment.offRouteDistanceM

	switch {
	case assessment.offRouteDistanceM > offRouteSanityMaxM:
		s.OffRouteViolations = 0
	case assessment.offRouteDistanceM > offRouteDistM:
		s.OffRouteViolations++
	default:
		s.OffRouteViolations = 0
	}

	assessment.congestionAhead, assessment.congestedEdgeCount = congestionSummaryLocked(s, store, g)
	s.LastCongestionAhead = assessment.congestionAhead
	s.LastCongestedEdges = assessment.congestedEdgeCount

	assessment.allowReroute = now.Sub(s.LastReroute) >= rerouteCooldown
	assessment.shouldRerouteNow =
		assessment.offRouteDistanceM <= offRouteSanityMaxM &&
			assessment.offRouteDistanceM > offRouteDistM &&
			s.OffRouteViolations >= offRouteStrikes

	return assessment
}

func attemptCongestionReroute(
	s *Session,
	snapLat, snapLon float64,
	g *builder.Graph,
	store *traffic.Store,
	mgr *Manager,
	router *routing.Router,
	wf routing.WeightFunc,
	prepareRoute func(routing.Route) routing.Route,
	now time.Time,
) {
	context, ok := captureCongestionContext(s, g, store)
	if !ok {
		return
	}

	candidate, isLocalPatch, ok := buildCongestionCandidate(snapLat, snapLon, router, wf, context)
	if !ok {
		return
	}

	newETA := candidate.TotalTimeSec
	if !isLocalPatch && !shouldAcceptCongestionCandidate(s, candidate, context.oldETA, newETA) {
		return
	}

	reason := "traffic"
	if isLocalPatch {
		reason = "local_patch"
	}

	applyRouteUpdate(s, candidate, g, store, mgr, prepareRoute, now, reason, &context.oldETA, &newETA)
}

func captureCongestionContext(s *Session, g *builder.Graph, store *traffic.Store) (congestionContext, bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	destination, ok := routeDestination(s.Route)
	if !ok {
		return congestionContext{}, false
	}

	context := congestionContext{
		currentRoute: cloneRoute(s.Route),
		destination:  destination,
		oldETA:       computeETALocked(s, g, store),
	}

	triggered, repairStepIdx, congestedCost := checkLocalRepairTriggerLocked(s, store, g)
	if !triggered {
		return context, true
	}

	context.localRepair = localRepairRequest{
		enabled:       true,
		repairStepIdx: repairStepIdx,
		congestedCost: congestedCost,
		fromNodeID:    s.Route.Steps[repairStepIdx-1].NodeID,
		toNodeID:      s.Route.Steps[repairStepIdx].NodeID,
	}
	return context, true
}

func buildCongestionCandidate(
	snapLat, snapLon float64,
	router *routing.Router,
	wf routing.WeightFunc,
	context congestionContext,
) (routing.Route, bool, bool) {
	if context.localRepair.enabled {
		patchSteps, ok := router.LocalRepairOverlay(
			context.localRepair.fromNodeID,
			context.localRepair.toNodeID,
			context.localRepair.congestedCost,
			localRepairMaxHops,
			wf,
		)
		if ok && len(patchSteps) > 0 {
			return rebuildPatchedRoute(context.currentRoute, context.localRepair.repairStepIdx, patchSteps), true, true
		}
	}

	routes := router.Compute(snapLat, snapLon, context.destination.Lat, context.destination.Lon, 1, wf)
	if len(routes) == 0 {
		return routing.Route{}, false, false
	}
	return routes[0], false, true
}

func shouldAcceptCongestionCandidate(s *Session, candidate routing.Route, oldETA, newETA float32) bool {
	etaGain := oldETA - newETA
	return oldETA > 0 &&
		(etaGain/oldETA >= rerouteSpeedupMin || etaGain >= rerouteMinGainSec) &&
		!sameRemainingRoute(s, candidate)
}

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

	newRoutes := router.Compute(lat, lon, destination.Lat, destination.Lon, 1, wf)
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

func checkLocalRepairTriggerLocked(s *Session, store *traffic.Store, g *builder.Graph) (bool, int, float32) {
	for i := maxInt(1, s.StepIdx); i < len(s.Route.Steps); i++ {
		step := s.Route.Steps[i]
		if step.EdgeID == nil {
			continue
		}

		edgeID := builder.EdgeID(*step.EdgeID)
		edge, ok := graphEdge(g, edgeID)
		if !ok {
			continue
		}

		liveWeight := store.LiveWeight(edgeID, edge.Weight, edge.SpeedKmh, edge.DistanceM)
		if liveWeight < edge.Weight*severeCongestionMultiplier || liveWeight-edge.Weight < severeCongestionMinDelay {
			continue
		}

		u := g.NodeByID(s.Route.Steps[i-1].NodeID)
		v := g.NodeByID(s.Route.Steps[i].NodeID)
		if u != nil && v != nil && u.CellID != v.CellID {
			return true, i, liveWeight
		}
	}

	return false, -1, 0
}

func rebuildPatchedRoute(oldRoute routing.Route, repairStepIdx int, patch []routing.Step) routing.Route {
	rawSteps := make([]routing.Step, 0, len(oldRoute.Steps)+len(patch))

	for i := 0; i < repairStepIdx; i++ {
		step := oldRoute.Steps[i]
		if i == 0 {
			step.DistanceM = 0
			step.BaseTimeSec = 0
		} else {
			step.DistanceM = oldRoute.Steps[i].DistanceM - oldRoute.Steps[i-1].DistanceM
			step.BaseTimeSec = oldRoute.Steps[i].BaseTimeSec - oldRoute.Steps[i-1].BaseTimeSec
		}
		rawSteps = append(rawSteps, step)
	}

	rawSteps = append(rawSteps, patch...)

	for i := repairStepIdx + 1; i < len(oldRoute.Steps); i++ {
		step := oldRoute.Steps[i]
		step.DistanceM = oldRoute.Steps[i].DistanceM - oldRoute.Steps[i-1].DistanceM
		step.BaseTimeSec = oldRoute.Steps[i].BaseTimeSec - oldRoute.Steps[i-1].BaseTimeSec
		rawSteps = append(rawSteps, step)
	}

	var totalDistance float32
	var totalTime float32
	for i := range rawSteps {
		if i > 0 {
			totalDistance += rawSteps[i].DistanceM
			totalTime += rawSteps[i].BaseTimeSec
		}
		rawSteps[i].DistanceM = totalDistance
		rawSteps[i].BaseTimeSec = totalTime
	}

	return routing.Route{
		ID:           oldRoute.ID,
		Steps:        rawSteps,
		TotalDistM:   totalDistance,
		TotalTimeSec: totalTime,
	}
}

func cloneRoute(route routing.Route) routing.Route {
	cloned := route
	cloned.Steps = append([]routing.Step(nil), route.Steps...)
	return cloned
}
