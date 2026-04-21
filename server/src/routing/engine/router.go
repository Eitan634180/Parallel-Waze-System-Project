package engine

import (
	"nav-system/src/graph/model"
)

// WeightFunc returns the effective travel time in seconds for an edge.
type WeightFunc func(e *model.Edge) float32

type Snapper interface {
	Snap(lat, lon float64) uint32
}

type Config struct {
	MaxSearchSpeedMps       float32
	AlternativeRoutePenalty float32
}

// BaseWeight is a WeightFunc that always returns the static edge weight.
func BaseWeight(e *model.Edge) float32 { return e.Weight }

// Router holds graph reference and provides route computation.
type Router struct {
	g      *model.Graph
	si     Snapper
	mode   RoutingMode
	config Config
}

// NewRouter constructs a Router.
func NewRouter(g *model.Graph, si Snapper, config Config) *Router {
	return NewRouterWithMode(g, si, RoutingModeHierarchical, config)
}

// NewRouterWithMode constructs a Router with the provided query strategy.
func NewRouterWithMode(g *model.Graph, si Snapper, mode RoutingMode, config Config) *Router {
	if mode == "" {
		mode = RoutingModeHierarchical
	}
	return &Router{g: g, si: si, mode: mode, config: config}
}

func (r *Router) Mode() RoutingMode {
	return r.mode
}

// Compute returns up to k routes from (srcLat, srcLon) to (dstLat, dstLon).
func (r *Router) Compute(srcLat, srcLon, dstLat, dstLon float64, k int, wf WeightFunc) []Route {
	if wf == nil {
		wf = BaseWeight
	}

	srcIdx := r.si.Snap(srcLat, srcLon)
	dstIdx := r.si.Snap(dstLat, dstLon)
	return r.computeFromIndices(srcIdx, dstIdx, k, wf, nil)
}

func (r *Router) ComputeFromIndices(srcIdx, dstIdx uint32, k int, wf WeightFunc) []Route {
	if wf == nil {
		wf = BaseWeight
	}
	return r.computeFromIndices(srcIdx, dstIdx, k, wf, nil)
}

func (r *Router) ComputeFromIndicesWithStats(srcIdx, dstIdx uint32, k int, wf WeightFunc) ([]Route, SearchStats) {
	if wf == nil {
		wf = BaseWeight
	}
	var stats SearchStats
	return r.computeFromIndices(srcIdx, dstIdx, k, wf, &stats), stats
}

func (r *Router) computeFromIndices(srcIdx, dstIdx uint32, k int, wf WeightFunc, stats *SearchStats) []Route {
	if srcIdx == dstIdx {
		return nil
	}

	penalties := make(map[model.EdgeID]float32)
	overlayPenalties := make(map[uint32]float32)
	penalizedWeight := func(edge *model.Edge) float32 {
		weight := wf(edge)
		if penalty, ok := penalties[edge.ID]; ok {
			weight *= penalty
		}
		return weight
	}

	routes := make([]Route, 0, k)
	for i := 0; i < k; i++ {
		var (
			steps            []Step
			usedOverlayEdges []uint32
			ok               bool
		)

		switch r.mode {
		case RoutingModeBaseAStar:
			steps, ok = r.fullGraphAStar(srcIdx, dstIdx, penalizedWeight, stats)
		case RoutingModeBaseDijkstra:
			steps, ok = r.fullGraphDijkstra(srcIdx, dstIdx, penalizedWeight, stats)
		default:
			steps, usedOverlayEdges, ok = r.twoLevelSearch(srcIdx, dstIdx, penalizedWeight, overlayPenalties, stats)
		}
		if !ok {
			break
		}

		routes = append(routes, stepsToRoute(steps))
		for _, step := range steps {
			if step.EdgeID != nil {
				penalties[model.EdgeID(*step.EdgeID)] = r.config.AlternativeRoutePenalty
			}
		}
		for _, overlayEdgeID := range usedOverlayEdges {
			overlayPenalties[overlayEdgeID] = r.config.AlternativeRoutePenalty
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
