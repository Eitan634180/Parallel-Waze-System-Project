package reroute

import (
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/navigation"
	navigationanalysis "nav-system/src/navigation/analysis"
	navigationsessions "nav-system/src/navigation/sessions"
	routingengine "nav-system/src/routing/engine"
	routingentities "nav-system/src/routing/entities"
	trafficstore "nav-system/src/traffic/store"
)

type congestionContext struct {
	currentRoute routingentities.Route
	currentStep  int
	destination  routingentities.Step
	oldETA       float32
	version      SessionVersion
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
	s *navigationsessions.Session,
	snapLat, snapLon float64,
	g *model.Graph,
	store *trafficstore.Store,
	mgr *navigationsessions.Manager,
	router *routingengine.Router,
	wf routingengine.WeightFunc,
	prepareRoute func(routingentities.Route) routingentities.Route,
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
	if !isLocalPatch && !shouldAcceptCongestionCandidate(candidate, context, newETA) {
		return
	}

	reason := navigation.RerouteReasonTraffic
	if isLocalPatch {
		reason = navigation.RerouteReasonLocalPatch
	}

	ApplyRouteUpdate(s, candidate, g, store, mgr, prepareRoute, now, reason, &context.oldETA, &newETA, &context.version)
}

func captureCongestionContext(s *navigationsessions.Session, g *model.Graph, store *trafficstore.Store) (congestionContext, bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	destination, ok := s.Route.Destination()
	if !ok {
		return congestionContext{}, false
	}

	context := congestionContext{
		currentRoute: s.Route,
		currentStep:  s.StepIdx,
		destination:  destination,
		oldETA:       navigationanalysis.ComputeETA(s.Route, s.StepIdx, s.LastLat, s.LastLon, g, store),
		version:      SessionVersion{StepIdx: s.StepIdx, RouteRevision: s.RouteRevision},
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
	router *routingengine.Router,
	wf routingengine.WeightFunc,
	context congestionContext,
) (routingentities.Route, bool, bool) {
	if context.localRepair.enabled {
		var patchSteps []routingentities.Step
		var ok bool
		if context.localRepair.isCrossCell {
			patchSteps, ok = router.LocalRepairOverlay(
				context.localRepair.fromNodeIdx,
				context.localRepair.toNodeIdx,
				context.localRepair.congestedCost,
				navigation.LocalRepairMaxHops,
				wf,
			)
		} else {
			patchSteps, ok = router.LocalRepairOriginal(
				context.localRepair.fromNodeIdx,
				context.localRepair.toNodeIdx,
				context.localRepair.congestedCost,
				navigation.LocalRepairOriginalMaxHops,
				wf,
			)
		}

		if ok && len(patchSteps) > 0 {
			return context.currentRoute.Patched(context.localRepair.repairStepIdx, patchSteps), true, true
		}
	}

	routes := router.Compute(snapLat, snapLon, context.destination.Lat, context.destination.Lon, 1, wf)
	if len(routes) == 0 {
		return routingentities.Route{}, false, false
	}
	return routes[0], false, true
}

func shouldAcceptCongestionCandidate(candidate routingentities.Route, context congestionContext, newETA float32) bool {
	etaGain := context.oldETA - newETA
	return context.oldETA > 0 &&
		(etaGain/context.oldETA >= navigation.RerouteSpeedupMin || etaGain >= navigation.RerouteMinGainSec) &&
		!context.currentRoute.SameRemaining(context.currentStep, candidate)
}
