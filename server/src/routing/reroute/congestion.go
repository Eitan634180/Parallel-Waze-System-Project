package reroute

import (
	"nav-system/src/graph/model"
	"nav-system/src/routing/engine"
	routinggeometry "nav-system/src/routing/geometry"
	trafficstore "nav-system/src/traffic/store"
)

const congestionMultiplier = trafficstore.CongestionThreshold

func routeCongestionSummary(route engine.Route, store *trafficstore.Store, g *model.Graph) (bool, int) {
	count := 0
	for _, step := range route.Steps {
		if step.EdgeID == nil {
			continue
		}

		edgeID := model.EdgeID(*step.EdgeID)
		edge, ok := routinggeometry.EdgeByID(g, edgeID)
		if !ok {
			continue
		}

		liveWeight := store.LiveWeight(edgeID, edge.Weight)
		if liveWeight >= edge.Weight*congestionMultiplier {
			count++
		}
	}

	return count > 0, count
}
