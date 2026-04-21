package monitor

import (
	navigationoffroute "nav-system/src/navigation/internal/offroute"
	navigationsession "nav-system/src/navigation/session"
)

func DebugSnapshot(s *navigationsession.Session, speedKmh float32) navigationsession.NavigationDebug {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	var currentEdgeID *uint32
	if s.CurrentEdgeID != nil {
		edgeID := *s.CurrentEdgeID
		currentEdgeID = &edgeID
	}

	debug := navigationsession.NavigationDebug{
		SessionID:          s.ID,
		StepIndex:          s.StepIdx,
		CurrentEdgeID:      currentEdgeID,
		OffRouteDistanceM:  s.LastOffRouteDistanceM,
		OffRouteThresholdM: navigationoffroute.OffRouteDistM,
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
