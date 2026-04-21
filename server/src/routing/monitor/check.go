package monitor

import (
	"time"

	coreconfig "nav-system/src/core/config"
	"nav-system/src/graph/model"
	"nav-system/src/routing"
	routinggeometry "nav-system/src/routing/geometry"
	routingreroute "nav-system/src/routing/reroute"
	"nav-system/src/session"
	trafficstore "nav-system/src/traffic/store"
)

type routeAssessment struct {
	allowReroute       bool
	shouldRerouteNow   bool
	congestionAhead    bool
	offRouteDistanceM  float32
	congestedEdgeCount int
}

func Check(
	s *session.Session,
	snapLat, snapLon float64,
	g *model.Graph,
	store *trafficstore.Store,
	mgr *session.Manager,
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
		routingreroute.DoReroute(s, snapLat, snapLon, g, store, mgr, router, wf, prepareRoute, now, rerouteReasonOffRoute, nil, nil)
		return
	}

	if !assessment.congestionAhead {
		return
	}

	routingreroute.AttemptCongestionReroute(s, snapLat, snapLon, g, store, mgr, router, wf, prepareRoute, now)
}

func pushETAIfDue(s *session.Session, g *model.Graph, store *trafficstore.Store, now time.Time) {
	s.Mu.Lock()
	if now.Sub(s.LastETAPush) < coreconfig.RoutingETAThrottle {
		s.Mu.Unlock()
		return
	}

	eta := ComputeETA(s.Route, s.StepIdx, s.LastLat, s.LastLon, g, store)
	s.ETA = eta
	s.LastETAPush = now
	s.Mu.Unlock()

	etaValue := eta
	_ = s.Send(session.OutMsg{Type: "eta_update", ETASec: &etaValue})
}

func refreshRouteAssessment(s *session.Session, snapLat, snapLon float64, g *model.Graph, store *trafficstore.Store, now time.Time) routeAssessment {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	assessment := routeAssessment{}
	assessment.offRouteDistanceM = routinggeometry.DistanceFromExpectedPath(s.Route, s.StepIdx, snapLat, snapLon, g)
	s.LastOffRouteDistanceM = assessment.offRouteDistanceM

	switch {
	case assessment.offRouteDistanceM > routinggeometry.OffRouteSanityMaxM:
		s.OffRouteViolations = 0
	case assessment.offRouteDistanceM > routinggeometry.OffRouteDistM:
		s.OffRouteViolations++
	default:
		s.OffRouteViolations = 0
	}

	assessment.congestionAhead, assessment.congestedEdgeCount = RemainingCongestionSummary(s.Route, s.StepIdx, store, g)
	s.LastCongestionAhead = assessment.congestionAhead
	s.LastCongestedEdges = assessment.congestedEdgeCount

	assessment.allowReroute = now.Sub(s.LastReroute) >= coreconfig.RoutingRerouteCooldown
	assessment.shouldRerouteNow =
		assessment.offRouteDistanceM <= routinggeometry.OffRouteSanityMaxM &&
			assessment.offRouteDistanceM > routinggeometry.OffRouteDistM &&
			s.OffRouteViolations >= coreconfig.RoutingOffRouteStrikes

	return assessment
}
