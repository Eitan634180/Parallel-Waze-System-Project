package analysis

import (
	"nav-system/src/navigation"
	navigationsessions "nav-system/src/navigation/sessions"
)

func DebugSnapshot(s *navigationsessions.Session, speedKmh float32) navigationsessions.NavigationDebug {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	var currentEdgeID *uint32
	if s.CurrentEdgeID != nil {
		edgeID := *s.CurrentEdgeID
		currentEdgeID = &edgeID
	}

	debug := navigationsessions.NavigationDebug{
		SessionID:          s.ID,
		StepIndex:          s.StepIdx,
		CurrentEdgeID:      currentEdgeID,
		OffRouteDistanceM:  s.LastOffRouteDistanceM,
		OffRouteThresholdM: navigation.OffRouteDistanceM,
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
