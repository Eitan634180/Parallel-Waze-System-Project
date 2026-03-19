package routing

import (
	"container/heap"
	"log"
	"math"

	"nav-system/map/builder"
)

// ---------------------------------------------------------------------------
// WeightFunc lets callers substitute live traffic weights for base weights.
// ---------------------------------------------------------------------------

// WeightFunc returns the effective travel time in seconds for an edge.
// Implementations should fall back to e.Weight when no live data exists.
type WeightFunc func(e *builder.Edge) float32

// BaseWeight is a WeightFunc that always returns the static edge weight.
func BaseWeight(e *builder.Edge) float32 { return e.Weight }

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------

// Router holds graph reference and provides route computation.
type Router struct {
	g  *builder.Graph
	si *SnapIndex
}

// NewRouter constructs a Router.
func NewRouter(g *builder.Graph, si *SnapIndex) *Router {
	return &Router{g: g, si: si}
}

// ---------------------------------------------------------------------------
// Compute returns up to k routes from (srcLat,srcLon) to (dstLat,dstLon).
// The first route is optimal under wf; subsequent routes are alternatives
// computed via the edge-penalty heuristic.
// ---------------------------------------------------------------------------

func (r *Router) Compute(srcLat, srcLon, dstLat, dstLon float64, k int, wf WeightFunc) []Route {
	if wf == nil {
		wf = BaseWeight
	}

	srcIdx := r.si.Snap(srcLat, srcLon)
	dstIdx := r.si.Snap(dstLat, dstLon)

	if srcIdx == dstIdx {
		return nil
	}

	// penalty overrides applied on top of wf for alternative routes.
	penalties := make(map[builder.EdgeID]float32)
	overlayPenalties := make(map[uint32]float32)

	penalisedWF := func(e *builder.Edge) float32 {
		w := wf(e)
		if p, ok := penalties[e.ID]; ok {
			w *= p
		}
		return w
	}

	routes := make([]Route, 0, k)
	for i := 0; i < k; i++ {
		steps, usedOverlayEdges, ok := r.twoLevelSearch(srcIdx, dstIdx, penalisedWF, overlayPenalties)
		if !ok {
			log.Printf("[WARNING] Route search on overlay failed")
			steps, ok = r.fullGraphSearch(srcIdx, dstIdx, penalisedWF)
			if !ok {
				break
			}
		}
		route := stepsToRoute(steps, r.g)
		routes = append(routes, route)

		// Penalise every edge on this route to encourage a different path next time.
		for _, s := range steps {
			if s.EdgeID != nil {
				penalties[builder.EdgeID(*s.EdgeID)] = 5.0
			}
		}
		for _, edgeIdx := range usedOverlayEdges {
			overlayPenalties[edgeIdx] = 5.0
		}
	}
	return routes
}

// ---------------------------------------------------------------------------
// Two-level search
// ---------------------------------------------------------------------------

func (r *Router) twoLevelSearch(
	srcIdx, dstIdx uint32,
	wf WeightFunc,
	overlayPenalties map[uint32]float32,
) ([]Step, []uint32, bool) {
	g := r.g
	srcNode := &g.Nodes[srcIdx]
	dstNode := &g.Nodes[dstIdx]

	sameCellSrc := srcNode.CellID
	sameCellDst := dstNode.CellID

	// Fast path: same cell → direct intra-cell Dijkstra.
	if sameCellSrc == sameCellDst {
		steps, ok := r.intraSearch(srcIdx, dstIdx, wf)
		return steps, nil, ok
	}

	// ── Phase 1: source-cell Dijkstra ────────────────────────────────────────
	// Expand from srcIdx within the source cell until all boundary nodes of
	// that cell are settled. No heuristic (multi-target).
	srcBoundary := g.Cells[sameCellSrc].BoundaryNodeIDs
	injCosts, injPred := r.cellDijkstra(srcIdx, srcBoundary, sameCellSrc, wf)
	overlaySeeds := make(map[builder.NodeID]float32, len(srcBoundary))
	for _, nid := range srcBoundary {
		if c, ok := injCosts[nid]; ok {
			overlaySeeds[nid] = c
		}
	}
	if len(overlaySeeds) == 0 {
		///////////////////////////// Somtimes when we have many users at once we fall here! Why?
		return nil, nil, false
	}

	// ── Phase 2: overlay A* ──────────────────────────────────────────────────
	// Seed with injection costs; expand over OverlayAdj toward dstNode.
	dstBoundary := g.Cells[sameCellDst].BoundaryNodeIDs
	dstBoundarySet := make(map[builder.NodeID]struct{}, len(dstBoundary))
	for _, nid := range dstBoundary {
		dstBoundarySet[nid] = struct{}{}
	}

	// heuristic: straight-line distance to dstNode / theoretical max speed
	const maxSpeedMs = 120.0 / 3.6 // 120 km/h in m/s
	heuristic := func(nid builder.NodeID) float32 {
		n := g.NodeByID(nid)
		if n == nil {
			return 0
		}
		return dist2m(n.X, n.Y, dstNode.X, dstNode.Y) / maxSpeedMs
	}

	overlayCosts, overlayPred := r.overlayAStar(overlaySeeds, dstBoundarySet, heuristic, overlayPenalties)

	// ── Phase 3: destination-cell multi-source Dijkstra ─────────────────────
	// Expand from all reached dst-boundary nodes into the dst cell → dstIdx.
	var seeds []seedE
	for _, nid := range dstBoundary {
		c, ok := overlayCosts[nid]
		if !ok {
			continue
		}
		ni, ok2 := g.NodeIdx[nid]
		if !ok2 {
			continue
		}
		seeds = append(seeds, seedE{ni, c})
	}
	if len(seeds) == 0 {
		return nil, nil, false
	}

	egressCosts, egressPred := r.multiSourceCellDijkstra(seeds, dstIdx, sameCellDst, wf)
	_ = egressCosts

	// ── Reconstruct full path ────────────────────────────────────────────────
	steps, usedOverlayEdges, ok := r.reconstruct(srcIdx, dstIdx, sameCellSrc, sameCellDst,
		injPred, overlayPred, egressPred, wf)
	return steps, usedOverlayEdges, ok
}

// ---------------------------------------------------------------------------
// Phase 1 & 3 helper: intra-cell Dijkstra (single source, stops at targets)
// ---------------------------------------------------------------------------

type predEntry struct {
	prevNodeID builder.NodeID
	edgeID     builder.EdgeID
}

type overlayPredEntry struct {
	prevNodeID builder.NodeID
	edgeIdx    uint32
}

func (r *Router) cellDijkstra(
	srcInternalIdx uint32,
	targetNodeIDs []builder.NodeID,
	cellID builder.CellID,
	wf WeightFunc,
) (costs map[builder.NodeID]float32, pred map[builder.NodeID]predEntry) {
	g := r.g
	costs = make(map[builder.NodeID]float32, len(targetNodeIDs)+1)
	pred = make(map[builder.NodeID]predEntry, len(targetNodeIDs))

	targetSet := make(map[builder.NodeID]struct{}, len(targetNodeIDs))
	for _, nid := range targetNodeIDs {
		targetSet[nid] = struct{}{}
	}
	remaining := len(targetSet)

	srcID := g.Nodes[srcInternalIdx].ID
	costs[srcID] = 0

	pq := &ijPQ{}
	heap.Push(pq, ijItem{id: srcID, cost: 0})

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(ijItem)
		if c, ok := costs[cur.id]; ok && cur.cost > c {
			continue // stale
		}

		if _, isTarget := targetSet[cur.id]; isTarget {
			remaining--
			delete(targetSet, cur.id)
			if remaining == 0 {
				break
			}
		}

		curIdx, ok := g.NodeIdx[cur.id]
		if !ok {
			continue
		}

		for _, eid := range g.BaseAdj.Neighbours(curIdx) {
			e := &g.Edges[eid]
			toIdx, ok2 := g.NodeIdx[e.ToNodeID]
			if !ok2 {
				continue
			}
			// Stay within cell.
			if g.Nodes[toIdx].CellID != cellID {
				continue
			}
			nc := cur.cost + wf(e)
			if existing, has := costs[e.ToNodeID]; !has || nc < existing {
				costs[e.ToNodeID] = nc
				pred[e.ToNodeID] = predEntry{cur.id, eid}
				heap.Push(pq, ijItem{id: e.ToNodeID, cost: nc})
			}
		}
	}
	return costs, pred
}

// intraSearch runs a point-to-point Dijkstra within a single cell.
func (r *Router) intraSearch(srcIdx, dstIdx uint32, wf WeightFunc) ([]Step, bool) {
	g := r.g
	cellID := g.Nodes[srcIdx].CellID
	dstID := g.Nodes[dstIdx].ID

	costs := make(map[builder.NodeID]float32, 64)
	pred := make(map[builder.NodeID]predEntry, 64)
	srcID := g.Nodes[srcIdx].ID
	costs[srcID] = 0

	pq := &ijPQ{}
	heap.Push(pq, ijItem{id: srcID, cost: 0})

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(ijItem)
		if c, ok := costs[cur.id]; ok && cur.cost > c {
			continue
		}
		if cur.id == dstID {
			break
		}
		curIdx, ok := g.NodeIdx[cur.id]
		if !ok {
			continue
		}
		for _, eid := range g.BaseAdj.Neighbours(curIdx) {
			e := &g.Edges[eid]
			toIdx, ok2 := g.NodeIdx[e.ToNodeID]
			if !ok2 {
				continue
			}
			if g.Nodes[toIdx].CellID != cellID {
				continue
			}
			nc := cur.cost + wf(e)
			if existing, has := costs[e.ToNodeID]; !has || nc < existing {
				costs[e.ToNodeID] = nc
				pred[e.ToNodeID] = predEntry{cur.id, eid}
				heap.Push(pq, ijItem{id: e.ToNodeID, cost: nc})
			}
		}
	}

	if _, reached := costs[dstID]; !reached {
		return nil, false
	}
	return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
}

// fullGraphSearch runs a global A* on the base graph.
// It is a fallback for cases where the partitioned search fails to find a path.
func (r *Router) fullGraphSearch(srcIdx, dstIdx uint32, wf WeightFunc) ([]Step, bool) {
	g := r.g
	srcID := g.Nodes[srcIdx].ID
	dstID := g.Nodes[dstIdx].ID
	dstNode := &g.Nodes[dstIdx]

	costs := make(map[builder.NodeID]float32, 256)
	pred := make(map[builder.NodeID]predEntry, 256)
	costs[srcID] = 0

	const maxSpeedMs = 120.0 / 3.6
	heuristic := func(idx uint32) float32 {
		n := &g.Nodes[idx]
		return dist2m(n.X, n.Y, dstNode.X, dstNode.Y) / maxSpeedMs
	}

	pq := &astarPQ{}
	heap.Push(pq, astarItem{id: srcID, f: heuristic(srcIdx), g: 0})

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(astarItem)
		if c, ok := costs[cur.id]; ok && cur.g > c {
			continue
		}
		if cur.id == dstID {
			return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
		}

		curIdx, ok := g.NodeIdx[cur.id]
		if !ok {
			continue
		}
		for _, eid := range g.BaseAdj.Neighbours(curIdx) {
			e := &g.Edges[eid]
			toIdx, ok2 := g.NodeIdx[e.ToNodeID]
			if !ok2 {
				continue
			}
			nc := cur.g + wf(e)
			if existing, has := costs[e.ToNodeID]; !has || nc < existing {
				costs[e.ToNodeID] = nc
				pred[e.ToNodeID] = predEntry{cur.id, eid}
				heap.Push(pq, astarItem{
					id: e.ToNodeID,
					g:  nc,
					f:  nc + heuristic(toIdx),
				})
			}
		}
	}

	return nil, false
}

// ---------------------------------------------------------------------------
// Phase 2: overlay A*
// ---------------------------------------------------------------------------

func (r *Router) overlayAStar(
	injCosts map[builder.NodeID]float32,
	dstSet map[builder.NodeID]struct{},
	h func(builder.NodeID) float32,
	penalties map[uint32]float32,
) (costs map[builder.NodeID]float32, pred map[builder.NodeID]overlayPredEntry) {
	g := r.g
	costs = make(map[builder.NodeID]float32, len(injCosts)+len(dstSet))
	pred = make(map[builder.NodeID]overlayPredEntry, len(injCosts)+len(dstSet))

	pq := &astarPQ{}
	for nid, c := range injCosts {
		costs[nid] = c
		heap.Push(pq, astarItem{id: nid, f: c + h(nid), g: c})
	}

	settled := 0
	total := len(dstSet)

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(astarItem)
		if c, ok := costs[cur.id]; ok && cur.g > c {
			continue // stale
		}

		if _, isDst := dstSet[cur.id]; isDst {
			settled++
			delete(dstSet, cur.id)
			if settled == total {
				break
			}
		}

		bIdx, ok := g.BoundaryNodeIdx[cur.id]
		if !ok {
			continue
		}
		g.OverlayAdj.Mu.RLock()
		base := g.OverlayAdj.Offsets[bIdx]
		for i, oe := range g.OverlayAdj.Neighbours(bIdx) {
			edgeIdx := base + uint32(i)
			weight := oe.Weight
			if p, ok := penalties[edgeIdx]; ok {
				weight *= p
			}
			nc := cur.g + weight
			if existing, has := costs[oe.ToNodeID]; !has || nc < existing {
				costs[oe.ToNodeID] = nc
				pred[oe.ToNodeID] = overlayPredEntry{prevNodeID: cur.id, edgeIdx: edgeIdx}
				heap.Push(pq, astarItem{id: oe.ToNodeID, f: nc + h(oe.ToNodeID), g: nc})
			}
		}
		g.OverlayAdj.Mu.RUnlock()
	}
	return costs, pred
}

// ---------------------------------------------------------------------------
// Phase 3: multi-source cell Dijkstra
// ---------------------------------------------------------------------------

type seedE struct {
	nodeIdx uint32
	cost    float32
}

func (r *Router) multiSourceCellDijkstra(
	seeds []seedE,
	dstInternalIdx uint32,
	cellID builder.CellID,
	wf WeightFunc,
) (costs map[builder.NodeID]float32, pred map[builder.NodeID]predEntry) {
	g := r.g
	costs = make(map[builder.NodeID]float32, len(seeds)+16)
	pred = make(map[builder.NodeID]predEntry, len(seeds)+16)
	dstID := g.Nodes[dstInternalIdx].ID

	pq := &ijPQ{}
	for _, s := range seeds {
		nid := g.Nodes[s.nodeIdx].ID
		costs[nid] = s.cost
		heap.Push(pq, ijItem{id: nid, cost: s.cost})
	}

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(ijItem)
		if c, ok := costs[cur.id]; ok && cur.cost > c {
			continue
		}
		if cur.id == dstID {
			break
		}
		curIdx, ok := g.NodeIdx[cur.id]
		if !ok {
			continue
		}
		for _, eid := range g.BaseAdj.Neighbours(curIdx) {
			e := &g.Edges[eid]
			toIdx, ok2 := g.NodeIdx[e.ToNodeID]
			if !ok2 {
				continue
			}
			if g.Nodes[toIdx].CellID != cellID {
				continue
			}
			nc := cur.cost + wf(e)
			if existing, has := costs[e.ToNodeID]; !has || nc < existing {
				costs[e.ToNodeID] = nc
				pred[e.ToNodeID] = predEntry{cur.id, eid}
				heap.Push(pq, ijItem{id: e.ToNodeID, cost: nc})
			}
		}
	}
	return costs, pred
}

// ---------------------------------------------------------------------------
// Full path reconstruction (delegates to reconstruct.go)
// ---------------------------------------------------------------------------

func (r *Router) reconstruct(
	srcIdx, dstIdx uint32,
	srcCellID, dstCellID builder.CellID,
	injPred map[builder.NodeID]predEntry,
	overlayPred map[builder.NodeID]overlayPredEntry,
	egressPred map[builder.NodeID]predEntry,
	wf WeightFunc,
) ([]Step, []uint32, bool) {
	return reconstructPath(r.g, srcIdx, dstIdx, srcCellID, dstCellID,
		injPred, overlayPred, egressPred, wf)
}

// ---------------------------------------------------------------------------
// stepsToRoute converts a Step slice to a Route with cumulative metrics.
// ---------------------------------------------------------------------------

func stepsToRoute(steps []Step, _ *builder.Graph) Route {
	var totalD, totalT float32
	for i := range steps {
		if i > 0 {
			totalD += steps[i].DistanceM
			totalT += steps[i].BaseTimeSec
		}
	}
	// Make each step's DistanceM and BaseTimeSec cumulative.
	var cumD, cumT float32
	for i := range steps {
		if i > 0 {
			cumD += steps[i].DistanceM
			cumT += steps[i].BaseTimeSec
		}
		steps[i].DistanceM = cumD
		steps[i].BaseTimeSec = cumT
	}
	return Route{
		Steps:        steps,
		TotalDistM:   totalD,
		TotalTimeSec: totalT,
	}
}

// ---------------------------------------------------------------------------
// Helpers: Euclidean distance in metres between projected coords.
// ---------------------------------------------------------------------------

func dist2m(ax, ay, bx, by float32) float32 {
	dx := ax - bx
	dy := ay - by
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

// ---------------------------------------------------------------------------
// Priority queues
// ---------------------------------------------------------------------------

// ijItem is used for plain Dijkstra (no heuristic).
type ijItem struct {
	id   builder.NodeID
	cost float32
}

type ijPQ []ijItem

func (pq ijPQ) Len() int            { return len(pq) }
func (pq ijPQ) Less(i, j int) bool  { return pq[i].cost < pq[j].cost }
func (pq ijPQ) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *ijPQ) Push(x interface{}) { *pq = append(*pq, x.(ijItem)) }
func (pq *ijPQ) Pop() interface{} {
	old := *pq
	n := len(old)
	x := old[n-1]
	*pq = old[:n-1]
	return x
}

// astarItem carries both the f-score (g+h) and g-score.
type astarItem struct {
	id builder.NodeID
	f  float32 // g + h  (priority)
	g  float32 // cost so far
}

type astarPQ []astarItem

func (pq astarPQ) Len() int            { return len(pq) }
func (pq astarPQ) Less(i, j int) bool  { return pq[i].f < pq[j].f }
func (pq astarPQ) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *astarPQ) Push(x interface{}) { *pq = append(*pq, x.(astarItem)) }
func (pq *astarPQ) Pop() interface{} {
	old := *pq
	n := len(old)
	x := old[n-1]
	*pq = old[:n-1]
	return x
}
