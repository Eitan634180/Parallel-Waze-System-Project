package monitor

import (
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/navigation"
	navigationoffroute "nav-system/src/navigation/internal/offroute"
	"nav-system/src/navigation/internal/routeutil"
	navigationmanager "nav-system/src/navigation/manager"
	navigationreroute "nav-system/src/navigation/reroute"
	navigationsession "nav-system/src/navigation/session"
	"nav-system/src/routing"
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
	s *navigationsession.Session,
	snapLat, snapLon float64,
	g *model.Graph,
	store *trafficstore.Store,
	mgr *navigationmanager.Manager,
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
		navigationreroute.DoReroute(s, snapLat, snapLon, g, store, mgr, router, wf, prepareRoute, now, navigation.RerouteReasonOffRoute, nil, nil)
		return
	}

	if !assessment.congestionAhead {
		return
	}

	navigationreroute.AttemptCongestionReroute(s, snapLat, snapLon, g, store, mgr, router, wf, prepareRoute, now)
}

func pushETAIfDue(s *navigationsession.Session, g *model.Graph, store *trafficstore.Store, now time.Time) {
	s.Mu.Lock()
	if now.Sub(s.LastETAPush) < navigation.ETAThrottle {
		s.Mu.Unlock()
		return
	}

	eta := routeutil.ComputeETA(s.Route, s.StepIdx, s.LastLat, s.LastLon, g, store)
	s.ETA = eta
	s.LastETAPush = now
	s.Mu.Unlock()

	etaValue := eta
	_ = s.Send(navigationsession.OutMsg{Type: "eta_update", ETASec: &etaValue})
}

func refreshRouteAssessment(s *navigationsession.Session, snapLat, snapLon float64, g *model.Graph, store *trafficstore.Store, now time.Time) routeAssessment {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	assessment := routeAssessment{}
	assessment.offRouteDistanceM = navigationoffroute.DistanceFromExpectedPath(s.Route, s.StepIdx, snapLat, snapLon, g)
	s.LastOffRouteDistanceM = assessment.offRouteDistanceM

	switch {
	case assessment.offRouteDistanceM > navigationoffroute.OffRouteSanityMaxM:
		s.OffRouteViolations = 0
	case assessment.offRouteDistanceM > navigationoffroute.OffRouteDistM:
		s.OffRouteViolations++
	default:
		s.OffRouteViolations = 0
	}

	assessment.congestionAhead, assessment.congestedEdgeCount = RemainingCongestionSummary(s.Route, s.StepIdx, store, g)
	s.LastCongestionAhead = assessment.congestionAhead
	s.LastCongestedEdges = assessment.congestedEdgeCount

	assessment.allowReroute = now.Sub(s.LastReroute) >= navigation.RerouteCooldown
	assessment.shouldRerouteNow =
		assessment.offRouteDistanceM <= navigationoffroute.OffRouteSanityMaxM &&
			assessment.offRouteDistanceM > navigationoffroute.OffRouteDistM &&
			s.OffRouteViolations >= navigation.OffRouteStrikes

	return assessment
}
