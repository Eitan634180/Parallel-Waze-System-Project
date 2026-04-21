package reroute

import (
	navigationsession "nav-system/src/navigation/session"
	"nav-system/src/graph/model"
	"nav-system/src/routing"
	routinggeometry "nav-system/src/routing/geometry"
	trafficstore "nav-system/src/traffic/store"
)

func buildRoutePayload(route routing.Route) navigationsession.RoutePayload {
	return navigationsession.RoutePayload{
		ID:              route.ID,
		Steps:           route.Steps,
		TotalDistM:      route.TotalDistM,
		TotalTimeSec:    route.TotalTimeSec,
		CongestionAhead: route.CongestionAhead,
		CongestedEdges:  route.CongestedEdges,
	}
}

func sendRerouteMessage(s *navigationsession.Session, route routing.Route, reason string, oldETA, newETA *float32) {
	payload := buildRoutePayload(route)
	_ = s.Send(navigationsession.OutMsg{
		Type:          "reroute",
		Route:         &payload,
		RerouteReason: &reason,
		OldETASec:     oldETA,
		NewETASec:     newETA,
	})
}

func sendCurrentSpeedHints(s *navigationsession.Session, store *trafficstore.Store, g *model.Graph) {
	for _, edgeID32 := range s.RemainingEdges() {
		edgeID := model.EdgeID(edgeID32)
		edge, ok := routinggeometry.EdgeByID(g, edgeID)
		if !ok || edge.SpeedKmh <= 0 {
			continue
		}

		recommended := store.RecommendedSpeedKmh(edgeID, edge.SpeedKmh, edge.DistanceM)
		if recommended == edge.SpeedKmh && store.Density(edgeID) == 0 {
			continue
		}

		edgeIDValue := uint32(edgeID)
		recommendedValue := recommended
		_ = s.Send(navigationsession.OutMsg{
			Type:                "speed_update",
			EdgeID:              &edgeIDValue,
			RecommendedSpeedKmh: &recommendedValue,
		})
	}
}
