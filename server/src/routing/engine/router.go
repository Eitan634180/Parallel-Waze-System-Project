package engine

import (
	"nav-system/src/graph/model"
	rootconfig "nav-system/src/routing"
	"nav-system/src/routing/algorithms"
	"nav-system/src/routing/entities"
)

// WeightFunc returns the effective travel time in seconds for an edge.
type WeightFunc func(e *model.Edge) float32
type OverlayWeightFunc func(edgeIdx uint32, staticWeight float32) float32

type Snapper interface {
	Snap(lat, lon float64) uint32
}

type Config struct {
	MaxSearchSpeedMps       float32
	AlternativeRoutePenalty float32
}

// BaseWeight is a WeightFunc that always returns the static edge weight.
func BaseWeight(e *model.Edge) float32 { return e.BaseWeight }

// BaseOverlayWeight returns the static overlay weight.
func BaseOverlayWeight(_ uint32, staticWeight float32) float32 { return staticWeight }

// Router holds graph reference and provides route computation.
type Router struct {
	g      *model.Graph
	si     Snapper
	mode   entities.RoutingMode
	config Config
	overlayWeight OverlayWeightFunc
}

// NewRouter constructs a Router.
func NewRouter(g *model.Graph, si Snapper) *Router {
	return NewRouterWithMode(g, si, entities.RoutingModeHierarchical)
}

// NewRouterWithMode constructs a Router with the provided query strategy.
func NewRouterWithMode(g *model.Graph, si Snapper, mode entities.RoutingMode) *Router {
	if mode == "" {
		mode = entities.RoutingModeHierarchical
	}
	return &Router{
		g:    g,
		si:   si,
		mode: mode,
		overlayWeight: BaseOverlayWeight,
		config: Config{
			MaxSearchSpeedMps:       rootconfig.MaxSearchSpeedMps,
			AlternativeRoutePenalty: rootconfig.AlternativeRoutePenalty,
		},
	}
}

func (r *Router) Mode() entities.RoutingMode {
	return r.mode
}

func (r *Router) SetOverlayWeightFunc(overlayWeight OverlayWeightFunc) {
	if overlayWeight == nil {
		overlayWeight = BaseOverlayWeight
	}
	r.overlayWeight = overlayWeight
}

// Compute returns up to k routes from (srcLat, srcLon) to (dstLat, dstLon).
func (r *Router) Compute(srcLat, srcLon, dstLat, dstLon float64, k int, wf WeightFunc) []entities.Route {
	if wf == nil {
		wf = BaseWeight
	}

	srcIdx := r.si.Snap(srcLat, srcLon)
	dstIdx := r.si.Snap(dstLat, dstLon)
	return r.computeFromIndices(srcIdx, dstIdx, k, wf, nil)
}

func (r *Router) ComputeFromIndices(srcIdx, dstIdx uint32, k int, wf WeightFunc) []entities.Route {
	if wf == nil {
		wf = BaseWeight
	}
	return r.computeFromIndices(srcIdx, dstIdx, k, wf, nil)
}

func (r *Router) ComputeFromIndicesWithStats(srcIdx, dstIdx uint32, k int, wf WeightFunc) ([]entities.Route, entities.SearchStats) {
	if wf == nil {
		wf = BaseWeight
	}
	var stats entities.SearchStats
	return r.computeFromIndices(srcIdx, dstIdx, k, wf, &stats), stats
}

func (r *Router) computeFromIndices(srcIdx, dstIdx uint32, k int, wf WeightFunc, stats *entities.SearchStats) []entities.Route {
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

	routes := make([]entities.Route, 0, k)
	for i := 0; i < k; i++ {
		var (
			steps            []entities.Step
			usedOverlayEdges []uint32
			ok               bool
		)

		switch r.mode {
		case entities.RoutingModeBaseAStar:
			steps, ok = algorithms.FullGraphAStar(r.g, srcIdx, dstIdx, penalizedWeight, r.config.MaxSearchSpeedMps, stats)
		case entities.RoutingModeBaseDijkstra:
			steps, ok = algorithms.FullGraphDijkstra(r.g, srcIdx, dstIdx, penalizedWeight, stats)
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

func (r *Router) LocalRepairOverlay(
	srcIdx, dstIdx uint32,
	maxCost float32,
	maxHops int,
	wf WeightFunc,
) ([]entities.Step, bool) {
	if wf == nil {
		wf = BaseWeight
	}
	return algorithms.LocalRepairOverlay(r.g, srcIdx, dstIdx, maxCost, maxHops, r.config.MaxSearchSpeedMps, wf, r.overlayWeight)
}

func (r *Router) LocalRepairOriginal(
	srcIdx, dstIdx uint32,
	maxCost float32,
	maxHops int,
	wf WeightFunc,
) ([]entities.Step, bool) {
	if wf == nil {
		wf = BaseWeight
	}
	return algorithms.LocalRepairOriginal(r.g, srcIdx, dstIdx, maxCost, maxHops, r.config.MaxSearchSpeedMps, wf)
}

func stepsToRoute(steps []entities.Step) entities.Route {
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

	return entities.Route{
		Steps:        steps,
		TotalDistM:   totalDistance,
		TotalTimeSec: totalTime,
	}
}
