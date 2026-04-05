package session

import (
	"nav-system/internal/geo"
	"nav-system/internal/graph/builder"
	"nav-system/internal/routing"
)

const (
	minLatitude  = -90
	maxLatitude  = 90
	minLongitude = -180
	maxLongitude = 180
)

func distanceFromExpectedPathMLocked(s *Session, lat, lon float64, g *builder.Graph) float32 {
	if len(s.Route.Steps) == 0 {
		return 0
	}

	px, py := geo.Project(lat, lon)
	best := distanceFromExpectedProjectionMLocked(s, g, px, py)

	if lat < minLatitude || lat > maxLatitude {
		swappedX, swappedY := geo.Project(lon, lat)
		swappedBest := distanceFromExpectedProjectionMLocked(s, g, swappedX, swappedY)
		if swappedBest < best {
			return swappedBest
		}
	}

	return best
}

func distanceFromExpectedProjectionMLocked(s *Session, g *builder.Graph, px, py float32) float32 {
	if len(s.Route.Steps) == 1 {
		return distanceToRouteNode(g, s.Route.Steps[0], px, py)
	}

	if s.StepIdx < 0 || s.StepIdx >= len(s.Route.Steps) {
		lastIndex := maxInt(0, minInt(s.StepIdx, len(s.Route.Steps)-1))
		return distanceToRouteNode(g, s.Route.Steps[lastIndex], px, py)
	}

	start := maxInt(1, s.StepIdx-offRouteWindow)
	end := minInt(len(s.Route.Steps)-1, s.StepIdx+offRouteWindow)
	best := minDistanceToSegmentRangeLocked(s, g, px, py, start, end)
	if best <= offRouteDistM {
		return best
	}

	fullBest := minDistanceToSegmentRangeLocked(s, g, px, py, 1, len(s.Route.Steps)-1)
	if fullBest >= 0 && (best < 0 || fullBest < best) {
		return fullBest
	}
	if best >= 0 {
		return best
	}
	return 0
}

func distanceToRouteNode(g *builder.Graph, step routing.Step, px, py float32) float32 {
	node := g.NodeByID(builder.NodeID(step.NodeID))
	if node == nil {
		return 0
	}
	return geo.Distance(px, py, node.X, node.Y)
}

func minDistanceToSegmentRangeLocked(s *Session, g *builder.Graph, px, py float32, start, end int) float32 {
	best := float32(-1)
	for idx := start; idx <= end; idx++ {
		prev := g.NodeByID(builder.NodeID(s.Route.Steps[idx-1].NodeID))
		next := g.NodeByID(builder.NodeID(s.Route.Steps[idx].NodeID))
		if prev == nil || next == nil {
			continue
		}

		dist := geo.DistancePointToSegment(px, py, prev.X, prev.Y, next.X, next.Y)
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
