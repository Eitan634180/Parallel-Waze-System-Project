package session

import (
	"time"

	"nav-system/internal/graph/builder"
	"nav-system/internal/routing"
	"nav-system/internal/traffic"
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

	triggered, repairStepIdx, congestedCost, isCrossCell := checkLocalRepairTriggerLocked(s, store, g)
	if !triggered {
		return context, true
	}

	context.localRepair = localRepairRequest{
		enabled:       true,
		repairStepIdx: repairStepIdx,
		congestedCost: congestedCost,
		fromNodeID:    s.Route.Steps[repairStepIdx-1].NodeID,
		toNodeID:      s.Route.Steps[repairStepIdx].NodeID,
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
				context.localRepair.fromNodeID,
				context.localRepair.toNodeID,
				context.localRepair.congestedCost,
				localRepairMaxHops,
				wf,
			)
		} else {
			patchSteps, ok = router.LocalRepairOriginal(
				context.localRepair.fromNodeID,
				context.localRepair.toNodeID,
				context.localRepair.congestedCost,
				20, // maxHops for original graph search
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
