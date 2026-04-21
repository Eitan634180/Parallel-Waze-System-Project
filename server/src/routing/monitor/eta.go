package monitor

import (
	"nav-system/src/core/utilities"
	"nav-system/src/graph/model"
	"nav-system/src/routing/engine"
	routinggeometry "nav-system/src/routing/geometry"
	trafficstore "nav-system/src/traffic/store"
)

func ComputeETA(route engine.Route, stepIdx int, lastLat, lastLon float64, g *model.Graph, store *trafficstore.Store) float32 {
	var total float32

	for index, step := range remainingSteps(route, stepIdx) {
		if step.EdgeID == nil {
			continue
		}

		edgeID := model.EdgeID(*step.EdgeID)
		edge, ok := routinggeometry.EdgeByID(g, edgeID)
		if !ok {
			continue
		}

		weight := store.LiveWeight(edgeID, edge.Weight)
		if index == 0 {
			weight *= remainingFractionOnCurrentEdge(route, stepIdx, lastLat, lastLon, step, edge, g)
		}
		total += weight
	}

	return total
}

func remainingFractionOnCurrentEdge(route engine.Route, stepIdx int, lastLat, lastLon float64, step engine.Step, edge *model.Edge, g *model.Graph) float32 {
	if stepIdx <= 0 || stepIdx >= len(route.Steps) || edge.DistanceM <= 0 {
		return 1
	}

	px, py := utilities.ProjectAtReferenceLat(lastLat, lastLon, g.ProjectionRefLat)
	node := routinggeometry.NodeByIndex(g, step.NodeIdx)
	if node == nil {
		return 1
	}

	distLeft := utilities.Distance(px, py, node.X, node.Y)
	if distLeft >= edge.DistanceM {
		return 1
	}
	return distLeft / edge.DistanceM
}
