package routing

import "nav-system/src/graph/builder"

// WeightFunc returns the effective travel time in seconds for an edge.
type WeightFunc func(e *builder.Edge) float32

// BaseWeight is a WeightFunc that always returns the static edge weight.
func BaseWeight(e *builder.Edge) float32 { return e.Weight }

// Router holds graph reference and provides route computation.
type Router struct {
	g  *builder.Graph
	si *SnapIndex
}

// NewRouter constructs a Router.
func NewRouter(g *builder.Graph, si *SnapIndex) *Router {
	return &Router{g: g, si: si}
}

// Compute returns up to k routes from (srcLat, srcLon) to (dstLat, dstLon).
func (r *Router) Compute(srcLat, srcLon, dstLat, dstLon float64, k int, wf WeightFunc) []Route {
	if wf == nil {
		wf = BaseWeight
	}

	srcIdx := r.si.Snap(srcLat, srcLon)
	dstIdx := r.si.Snap(dstLat, dstLon)
	if srcIdx == dstIdx {
		return nil
	}

	penalties := make(map[builder.EdgeID]float32)
	overlayPenalties := make(map[uint32]float32)
	penalizedWeight := func(edge *builder.Edge) float32 {
		weight := wf(edge)
		if penalty, ok := penalties[edge.ID]; ok {
			weight *= penalty
		}
		return weight
	}

	routes := make([]Route, 0, k)
	for i := 0; i < k; i++ {
		steps, usedOverlayEdges, ok := r.twoLevelSearch(srcIdx, dstIdx, penalizedWeight, overlayPenalties)
		if !ok {
			break
		}

		routes = append(routes, stepsToRoute(steps))
		for _, step := range steps {
			if step.EdgeID != nil {
				penalties[builder.EdgeID(*step.EdgeID)] = alternativeRoutePenalty
			}
		}
		for _, overlayEdgeID := range usedOverlayEdges {
			overlayPenalties[overlayEdgeID] = alternativeRoutePenalty
		}
	}

	return routes
}

func stepsToRoute(steps []Step) Route {
	var totalDistance float32
	var totalTime float32
	for i := range steps {
		if i > 0 {
			totalDistance += steps[i].DistanceM
			totalTime += steps[i].BaseTimeSec
		}
	}

	var cumulativeDistance float32
	var cumulativeTime float32
	for i := range steps {
		if i > 0 {
			cumulativeDistance += steps[i].DistanceM
			cumulativeTime += steps[i].BaseTimeSec
		}
		steps[i].DistanceM = cumulativeDistance
		steps[i].BaseTimeSec = cumulativeTime
	}

	return Route{
		Steps:        steps,
		TotalDistM:   totalDistance,
		TotalTimeSec: totalTime,
	}
}
