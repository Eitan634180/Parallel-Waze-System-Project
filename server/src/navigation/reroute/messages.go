package reroute

import (
	"nav-system/src/graph/model"
	navigationsessions "nav-system/src/navigation/sessions"
	routingentities "nav-system/src/routing/entities"
	trafficstore "nav-system/src/traffic/store"
)

func buildRoutePayload(route routingentities.Route) navigationsessions.RoutePayload {
	return navigationsessions.RoutePayload{
		ID:              route.ID,
		Steps:           route.Steps,
		TotalDistM:      route.TotalDistM,
		TotalTimeSec:    route.TotalTimeSec,
		CongestionAhead: route.CongestionAhead,
		CongestedEdges:  route.CongestedEdges,
	}
}

func sendRerouteMessage(s *navigationsessions.Session, route routingentities.Route, reason string, oldETA, newETA *float32) {
	payload := buildRoutePayload(route)
	_ = s.Send(navigationsessions.OutMsg{
		Type:          "reroute",
		Route:         &payload,
		RerouteReason: &reason,
		OldETASec:     oldETA,
		NewETASec:     newETA,
	})
}

func sendCurrentSpeedHints(s *navigationsessions.Session, store *trafficstore.Store, g *model.Graph) {
	for _, edgeID32 := range s.RemainingEdges() {
		edgeID := model.EdgeID(edgeID32)
		edge, ok := g.Edge(edgeID)
		if !ok || edge.SpeedKmh <= 0 {
			continue
		}

		recommended := store.RecommendedSpeedKmh(edgeID, edge.SpeedKmh, edge.DistanceM)
		if recommended == edge.SpeedKmh && store.Density(edgeID) == 0 {
			continue
		}

		edgeIDValue := uint32(edgeID)
		recommendedValue := recommended
		_ = s.Send(navigationsessions.OutMsg{
			Type:                "speed_update",
			EdgeID:              &edgeIDValue,
			RecommendedSpeedKmh: &recommendedValue,
		})
	}
}
