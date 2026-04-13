package session

import (
	"time"

	"nav-system/src/graph/builder"
	"nav-system/src/traffic"
)

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
