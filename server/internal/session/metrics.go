package session

import (
	"nav-system/internal/geo"
	"nav-system/internal/graph/builder"
	"nav-system/internal/routing"
	"nav-system/internal/traffic"
)

func computeETALocked(s *Session, g *builder.Graph, store *traffic.Store) float32 {
	var total float32

	for index, step := range s.remainingStepsLocked() {
		if step.EdgeID == nil {
			continue
		}

		edgeID := builder.EdgeID(*step.EdgeID)
		edge, ok := graphEdge(g, edgeID)
		if !ok {
			continue
		}

		weight := store.LiveWeight(edgeID, edge.Weight, edge.SpeedKmh, edge.DistanceM)
		if index == 0 {
			weight *= remainingFractionOnCurrentEdge(s, step, edge, g)
		}
		total += weight
	}

	return total
}

func remainingFractionOnCurrentEdge(s *Session, step routing.Step, edge *builder.Edge, g *builder.Graph) float32 {
	if s.StepIdx <= 0 || s.StepIdx >= len(s.Route.Steps) || edge.DistanceM <= 0 {
		return 1
	}

	px, py := geo.Project(s.LastLat, s.LastLon)
	node := g.NodeByID(builder.NodeID(step.NodeID))
	if node == nil {
		return 1
	}

	distLeft := geo.Distance(px, py, node.X, node.Y)
	if distLeft >= edge.DistanceM {
		return 1
	}
	return distLeft / edge.DistanceM
}

func congestionSummaryLocked(s *Session, store *traffic.Store, g *builder.Graph) (bool, int) {
	return routeCongestionSummary(s.remainingStepsLocked(), store, g)
}

func RouteCongestionSummary(route routing.Route, store *traffic.Store, g *builder.Graph) (bool, int) {
	return routeCongestionSummary(route.Steps, store, g)
}

func routeCongestionSummary(steps []routing.Step, store *traffic.Store, g *builder.Graph) (bool, int) {
	count := 0
	for _, step := range steps {
		if step.EdgeID == nil {
			continue
		}

		edgeID := builder.EdgeID(*step.EdgeID)
		edge, ok := graphEdge(g, edgeID)
		if !ok {
			continue
		}

		liveWeight := store.LiveWeight(edgeID, edge.Weight, edge.SpeedKmh, edge.DistanceM)
		if liveWeight >= edge.Weight*congestionMultiplier {
			count++
		}
	}

	return count > 0, count
}

func graphEdge(g *builder.Graph, edgeID builder.EdgeID) (*builder.Edge, bool) {
	if int(edgeID) < 0 || int(edgeID) >= len(g.Edges) {
		return nil, false
	}
	return &g.Edges[edgeID], true
}
