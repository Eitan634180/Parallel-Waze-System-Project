package offroute

import (
	"nav-system/src/graph/model"
	"nav-system/src/routing/engine"
	routinggeometry "nav-system/src/routing/geometry"
	"nav-system/src/utilities"
)

func DistanceFromExpectedPath(route engine.Route, stepIdx int, lat, lon float64, g *model.Graph) float32 {
	if len(route.Steps) == 0 {
		return 0
	}

	px, py := utilities.ProjectAtReferenceLat(lat, lon, g.ProjectionRefLat)
	best := distanceFromExpectedProjection(route, stepIdx, g, px, py)

	if lat < utilities.MinLatitude || lat > utilities.MaxLatitude {
		swappedX, swappedY := utilities.ProjectAtReferenceLat(lon, lat, g.ProjectionRefLat)
		swappedBest := distanceFromExpectedProjection(route, stepIdx, g, swappedX, swappedY)
		if swappedBest < best {
			return swappedBest
		}
	}

	return best
}

func distanceFromExpectedProjection(route engine.Route, stepIdx int, g *model.Graph, px, py float32) float32 {
	if len(route.Steps) == 1 {
		return distanceToRouteNode(g, route.Steps[0], px, py)
	}

	if stepIdx < 0 || stepIdx >= len(route.Steps) {
		lastIndex := maxInt(0, minInt(stepIdx, len(route.Steps)-1))
		return distanceToRouteNode(g, route.Steps[lastIndex], px, py)
	}

	start := maxInt(1, stepIdx-OffRouteWindow)
	end := minInt(len(route.Steps)-1, stepIdx+OffRouteWindow)
	best := minDistanceToSegmentRange(route, g, px, py, start, end)
	if best <= OffRouteDistM {
		return best
	}

	fullBest := minDistanceToSegmentRange(route, g, px, py, 1, len(route.Steps)-1)
	if fullBest >= 0 && (best < 0 || fullBest < best) {
		return fullBest
	}
	if best >= 0 {
		return best
	}
	return 0
}

func distanceToRouteNode(g *model.Graph, step engine.Step, px, py float32) float32 {
	node := routinggeometry.NodeByIndex(g, step.NodeIdx)
	if node == nil {
		return 0
	}
	return utilities.Distance(px, py, node.X, node.Y)
}

func minDistanceToSegmentRange(route engine.Route, g *model.Graph, px, py float32, start, end int) float32 {
	best := float32(-1)
	for idx := start; idx <= end; idx++ {
		prev := routinggeometry.NodeByIndex(g, route.Steps[idx-1].NodeIdx)
		next := routinggeometry.NodeByIndex(g, route.Steps[idx].NodeIdx)
		if prev == nil || next == nil {
			continue
		}

		dist := utilities.DistancePointToSegment(px, py, prev.X, prev.Y, next.X, next.Y)
		if best < 0 || dist < best {
			best = dist
		}
	}
	return best
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
