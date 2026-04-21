package monitor

import (
	"nav-system/src/graph/model"
	"nav-system/src/routing/engine"
	routinggeometry "nav-system/src/routing/geometry"
	trafficstore "nav-system/src/traffic/store"
)

func RemainingCongestionSummary(route engine.Route, stepIdx int, store *trafficstore.Store, g *model.Graph) (bool, int) {
	return stepCongestionSummary(remainingSteps(route, stepIdx), store, g)
}

func RouteCongestionSummary(route engine.Route, store *trafficstore.Store, g *model.Graph) (bool, int) {
	return stepCongestionSummary(route.Steps, store, g)
}

func stepCongestionSummary(steps []engine.Step, store *trafficstore.Store, g *model.Graph) (bool, int) {
	count := 0
	for _, step := range steps {
		if step.EdgeID == nil {
			continue
		}

		edgeID := model.EdgeID(*step.EdgeID)
	edge, ok := routinggeometry.EdgeByID(g, edgeID)
	if !ok {
		continue
	}

	liveWeight := store.LiveWeight(edgeID, edge.Weight)
	if liveWeight >= edge.Weight*trafficstore.CongestionThreshold {
		count++
	}
	}

	return count > 0, count
}
