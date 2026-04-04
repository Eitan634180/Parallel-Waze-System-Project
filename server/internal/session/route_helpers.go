package session

import (
	"nav-system/internal/graph/builder"
	"nav-system/internal/routing"
	"nav-system/internal/traffic"
)

func sameRemainingRoute(s *Session, candidate routing.Route) bool {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	currentStepIdx := s.StepIdx
	if currentStepIdx < 0 {
		currentStepIdx = 0
	}

	currentEdges := remainingEdgeSequence(s.Route, currentStepIdx)
	candidateEdges := remainingEdgeSequence(candidate, InitialStepIndex(candidate))

	if len(candidateEdges) == 0 {
		return true
	}
	if len(candidateEdges) > len(currentEdges) {
		return false
	}

	offset := len(currentEdges) - len(candidateEdges)
	if offset > 2 {
		return false
	}

	for i := range candidateEdges {
		if currentEdges[offset+i] != candidateEdges[i] {
			return false
		}
	}
	return true
}

func remainingEdgeSequence(route routing.Route, stepIdx int) []uint32 {
	if stepIdx < 0 {
		stepIdx = 0
	}
	if stepIdx >= len(route.Steps) {
		return nil
	}
	return stepEdgeIDs(route.Steps[stepIdx:])
}

func buildRoutePayload(route routing.Route) RoutePayload {
	return RoutePayload{
		ID:              route.ID,
		Steps:           route.Steps,
		TotalDistM:      route.TotalDistM,
		TotalTimeSec:    route.TotalTimeSec,
		CongestionAhead: route.CongestionAhead,
		CongestedEdges:  route.CongestedEdges,
	}
}

func sendRerouteMessage(s *Session, route routing.Route, reason string, oldETA, newETA *float32) {
	payload := buildRoutePayload(route)
	_ = s.Send(OutMsg{
		Type:          "reroute",
		Route:         &payload,
		RerouteReason: &reason,
		OldETASec:     oldETA,
		NewETASec:     newETA,
	})
}

func sendCurrentSpeedHints(s *Session, store *traffic.Store, g *builder.Graph) {
	for _, edgeID32 := range s.RemainingEdges() {
		edgeID := builder.EdgeID(edgeID32)
		edge, ok := graphEdge(g, edgeID)
		if !ok || edge.SpeedKmh <= 0 {
			continue
		}

		recommended := store.RecommendedSpeedKmh(edgeID, edge.SpeedKmh, edge.DistanceM)
		if recommended == edge.SpeedKmh && store.Density(edgeID) == 0 {
			continue
		}

		edgeIDValue := uint32(edgeID)
		recommendedValue := recommended
		_ = s.Send(OutMsg{
			Type:                "speed_update",
			EdgeID:              &edgeIDValue,
			RecommendedSpeedKmh: &recommendedValue,
		})
	}
}
