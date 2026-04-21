package monitor

import (
	routinggeometry "nav-system/src/routing/geometry"
	"nav-system/src/session"
)

func DebugSnapshot(s *session.Session, speedKmh float32) session.NavigationDebug {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	var currentEdgeID *uint32
	if s.CurrentEdgeID != nil {
		edgeID := *s.CurrentEdgeID
		currentEdgeID = &edgeID
	}

	debug := session.NavigationDebug{
		SessionID:          s.ID,
		StepIndex:          s.StepIdx,
		CurrentEdgeID:      currentEdgeID,
		OffRouteDistanceM:  s.LastOffRouteDistanceM,
		OffRouteThresholdM: routinggeometry.OffRouteDistM,
		OffRouteViolations: s.OffRouteViolations,
		CongestionAhead:    s.LastCongestionAhead,
		CongestedEdges:     s.LastCongestedEdges,
		ETASec:             s.ETA,
		SpeedKmh:           speedKmh,
		LastRerouteReason:  s.LastRerouteReason,
	}
	if !s.LastReroute.IsZero() {
		debug.LastRerouteAtUnixMs = s.LastReroute.UnixMilli()
	}
	return debug
}
