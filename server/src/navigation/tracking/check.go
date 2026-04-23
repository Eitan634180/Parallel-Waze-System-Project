package tracking

import (
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/navigation"
	navigationanalysis "nav-system/src/navigation/analysis"
	navigationreroute "nav-system/src/navigation/reroute"
	navigationsessions "nav-system/src/navigation/sessions"
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

func checkSession(
	s *navigationsessions.Session,
	snapLat, snapLon float64,
	g *model.Graph,
	store *trafficstore.Store,
	mgr *navigationsessions.Manager,
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

func pushETAIfDue(s *navigationsessions.Session, g *model.Graph, store *trafficstore.Store, now time.Time) {
	s.Mu.Lock()
	if now.Sub(s.LastETAPush) < navigation.ETAThrottle {
		s.Mu.Unlock()
		return
	}

	eta := navigationanalysis.ComputeETA(s.Route, s.StepIdx, s.LastLat, s.LastLon, g, store)
	s.ETA = eta
	s.LastETAPush = now
	s.Mu.Unlock()

	etaValue := eta
	_ = s.Send(navigationsessions.OutMsg{Type: "eta_update", ETASec: &etaValue})
}

func refreshRouteAssessment(s *navigationsessions.Session, snapLat, snapLon float64, g *model.Graph, store *trafficstore.Store, now time.Time) routeAssessment {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	assessment := routeAssessment{}
	assessment.offRouteDistanceM = navigationanalysis.DistanceFromExpectedPath(s.Route, s.StepIdx, snapLat, snapLon, g)
	s.LastOffRouteDistanceM = assessment.offRouteDistanceM

	switch {
	case assessment.offRouteDistanceM > navigation.OffRouteSanityMaxM:
		s.OffRouteViolations = 0
	case assessment.offRouteDistanceM > navigation.OffRouteDistanceM:
		s.OffRouteViolations++
	default:
		s.OffRouteViolations = 0
	}

	assessment.congestionAhead, assessment.congestedEdgeCount = navigationanalysis.RemainingCongestionSummary(s.Route, s.StepIdx, store, g)
	s.LastCongestionAhead = assessment.congestionAhead
	s.LastCongestedEdges = assessment.congestedEdgeCount

	assessment.allowReroute = now.Sub(s.LastReroute) >= navigation.RerouteCooldown
	assessment.shouldRerouteNow =
		assessment.offRouteDistanceM <= navigation.OffRouteSanityMaxM &&
			assessment.offRouteDistanceM > navigation.OffRouteDistanceM &&
			s.OffRouteViolations >= navigation.OffRouteStrikes

	return assessment
}
