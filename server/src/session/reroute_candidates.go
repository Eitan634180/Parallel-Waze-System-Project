package session

import (
	"time"

	"nav-system/src/graph/builder"
	"nav-system/src/routing"
	"nav-system/src/traffic"
)

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

	reason := rerouteReasonTraffic
	if isLocalPatch {
		reason = rerouteReasonLocalPatch
	}

	applyRouteUpdate(s, candidate, g, store, mgr, prepareRoute, now, reason, &context.oldETA, &newETA, nil)
}

func captureCongestionContext(s *Session, g *builder.Graph, store *traffic.Store) (congestionContext, bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	destination, ok := routeDestination(s.Route)
	if !ok {
		return congestionContext{}, false
	}

	context := congestionContext{
		currentRoute: s.Route,
		destination:  destination,
		oldETA:       computeETALocked(s, g, store),
	}

	triggered, repairStepIdx, congestedCost, isCrossCell := checkLocalRepairTriggerLocked(s, store, g)
	if !triggered {
		return context, true
	}

	context.localRepair = localRepairRequest{
		enabled:       true,
		repairStepIdx: repairStepIdx,
		congestedCost: congestedCost,
		fromNodeIdx:   s.Route.Steps[repairStepIdx-1].NodeIdx,
		toNodeIdx:     s.Route.Steps[repairStepIdx].NodeIdx,
		isCrossCell:   isCrossCell,
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
		var patchSteps []routing.Step
		var ok bool
		if context.localRepair.isCrossCell {
			patchSteps, ok = router.LocalRepairOverlay(
				context.localRepair.fromNodeIdx,
				context.localRepair.toNodeIdx,
				context.localRepair.congestedCost,
				localRepairMaxHops,
				wf,
			)
		} else {
			patchSteps, ok = router.LocalRepairOriginal(
				context.localRepair.fromNodeIdx,
				context.localRepair.toNodeIdx,
				context.localRepair.congestedCost,
				localRepairOriginalMaxHops,
				wf,
			)
		}

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
