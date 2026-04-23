package analysis

import (
	"nav-system/src/graph/model"
	"nav-system/src/routing/engine"
	trafficstore "nav-system/src/traffic/store"
	"nav-system/src/utilities"
)

func ComputeETA(route engine.Route, stepIdx int, lastLat, lastLon float64, g *model.Graph, store *trafficstore.Store) float32 {
	var total float32

	for index, step := range RemainingSteps(route, stepIdx) {
		if step.EdgeID == nil {
			continue
		}

		edgeID := model.EdgeID(*step.EdgeID)
		edge, ok := g.Edge(edgeID)
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

func RemainingCongestionSummary(route engine.Route, stepIdx int, store *trafficstore.Store, g *model.Graph) (bool, int) {
	return StepCongestionSummary(RemainingSteps(route, stepIdx), store, g)
}

func RouteCongestionSummary(route engine.Route, store *trafficstore.Store, g *model.Graph) (bool, int) {
	return StepCongestionSummary(route.Steps, store, g)
}

func RemainingSteps(route engine.Route, stepIdx int) []engine.Step {
	switch {
	case stepIdx < 0:
		return route.Steps
	case stepIdx >= len(route.Steps):
		return nil
	default:
		return route.Steps[stepIdx:]
	}
}

func StepCongestionSummary(steps []engine.Step, store *trafficstore.Store, g *model.Graph) (bool, int) {
	count := 0
	for _, step := range steps {
		if step.EdgeID == nil {
			continue
		}

		edgeID := model.EdgeID(*step.EdgeID)
		edge, ok := g.Edge(edgeID)
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

func remainingFractionOnCurrentEdge(route engine.Route, stepIdx int, lastLat, lastLon float64, step engine.Step, edge *model.Edge, g *model.Graph) float32 {
	if stepIdx <= 0 || stepIdx >= len(route.Steps) || edge.DistanceM <= 0 {
		return 1
	}

	px, py := utilities.ProjectAtReferenceLat(lastLat, lastLon, g.ProjectionRefLat)
	node := g.Node(step.NodeIdx)
	if node == nil {
		return 1
	}

	distLeft := utilities.Distance(px, py, node.X, node.Y)
	if distLeft >= edge.DistanceM {
		return 1
	}
	return distLeft / edge.DistanceM
}
