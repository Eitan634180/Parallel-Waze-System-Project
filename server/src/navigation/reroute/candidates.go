package reroute

import (
	"time"

	coreconfig "nav-system/src/core/config"
	navigationmanager "nav-system/src/navigation/manager"
	"nav-system/src/navigation/internal/routeutil"
	navigationsession "nav-system/src/navigation/session"
	"nav-system/src/graph/model"
	"nav-system/src/routing"
	"nav-system/src/routing/engine"
	trafficstore "nav-system/src/traffic/store"
)

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
	fromNodeIdx   uint32
	toNodeIdx     uint32
	isCrossCell   bool
}

func AttemptCongestionReroute(
	s *navigationsession.Session,
	snapLat, snapLon float64,
	g *model.Graph,
	store *trafficstore.Store,
	mgr *navigationmanager.Manager,
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

	ApplyRouteUpdate(s, candidate, g, store, mgr, prepareRoute, now, reason, &context.oldETA, &newETA, nil)
}

func captureCongestionContext(s *navigationsession.Session, g *model.Graph, store *trafficstore.Store) (congestionContext, bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	destination, ok := engine.Destination(s.Route)
	if !ok {
		return congestionContext{}, false
	}

	context := congestionContext{
		currentRoute: s.Route,
		destination:  destination,
		oldETA:       routeutil.ComputeETA(s.Route, s.StepIdx, s.LastLat, s.LastLon, g, store),
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
				coreconfig.RoutingLocalRepairMaxHops,
				wf,
			)
		} else {
			patchSteps, ok = router.LocalRepairOriginal(
				context.localRepair.fromNodeIdx,
				context.localRepair.toNodeIdx,
				context.localRepair.congestedCost,
				coreconfig.RoutingLocalRepairOriginalMaxHops,
				wf,
			)
		}

		if ok && len(patchSteps) > 0 {
			return engine.RebuildPatchedRoute(context.currentRoute, context.localRepair.repairStepIdx, patchSteps), true, true
		}
	}

	routes := router.Compute(snapLat, snapLon, context.destination.Lat, context.destination.Lon, 1, wf)
	if len(routes) == 0 {
		return routing.Route{}, false, false
	}
	return routes[0], false, true
}

func shouldAcceptCongestionCandidate(s *navigationsession.Session, candidate routing.Route, oldETA, newETA float32) bool {
	etaGain := oldETA - newETA
	return oldETA > 0 &&
		(etaGain/oldETA >= coreconfig.RoutingRerouteSpeedupMin || etaGain >= coreconfig.RoutingRerouteMinGainSec) &&
		!engine.SameRemainingRoute(s.Route, s.StepIdx, candidate)
}
