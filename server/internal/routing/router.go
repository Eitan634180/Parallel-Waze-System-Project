package routing

import (
	"container/heap"
	"log"

	"nav-system/internal/geo"
	"nav-system/map/builder"
)

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
			steps, ok = r.fullGraphSearch(srcIdx, dstIdx, penalizedWeight)
			if !ok {
				log.Printf("[routing] route search failed")
				break
			}
			log.Printf("[routing] overlay search failed; falling back to full-graph A*")
		}

		routes = append(routes, stepsToRoute(steps, r.g))
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

func (r *Router) twoLevelSearch(
	srcIdx, dstIdx uint32,
	wf WeightFunc,
	overlayPenalties map[uint32]float32,
) ([]Step, []uint32, bool) {
	g := r.g
	srcNode := &g.Nodes[srcIdx]
	dstNode := &g.Nodes[dstIdx]

	srcCellID := srcNode.CellID
	dstCellID := dstNode.CellID
	if srcCellID == dstCellID {
		steps, ok := intraSearch(g, srcIdx, dstIdx, wf)
		return steps, nil, ok
	}

	srcBoundary := g.Cells[srcCellID].BoundaryNodeIDs
	injectionCosts, injectionPred := cellDijkstra(g, srcIdx, srcBoundary, srcCellID, wf)
	overlaySeeds := make(map[builder.NodeID]float32, len(srcBoundary))
	for _, nodeID := range srcBoundary {
		if cost, ok := injectionCosts[nodeID]; ok {
			overlaySeeds[nodeID] = cost
		}
	}
	if len(overlaySeeds) == 0 {
		return nil, nil, false
	}

	dstBoundary := g.Cells[dstCellID].BoundaryNodeIDs
	dstBoundarySet := make(map[builder.NodeID]struct{}, len(dstBoundary))
	for _, nodeID := range dstBoundary {
		dstBoundarySet[nodeID] = struct{}{}
	}

	heuristic := func(nodeID builder.NodeID) float32 {
		node := g.NodeByID(nodeID)
		if node == nil {
			return 0
		}
		return geo.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / maxSearchSpeedMps
	}

	overlayCosts, overlayPred := r.overlayAStar(overlaySeeds, dstBoundarySet, heuristic, overlayPenalties)

	seeds := make([]seedE, 0, len(dstBoundary))
	for _, nodeID := range dstBoundary {
		cost, ok := overlayCosts[nodeID]
		if !ok {
			continue
		}

		nodeIdx, ok := g.NodeIdx[nodeID]
		if !ok {
			continue
		}
		seeds = append(seeds, seedE{nodeIdx: nodeIdx, cost: cost})
	}
	if len(seeds) == 0 {
		return nil, nil, false
	}

	_, egressPred := multiSourceCellDijkstra(g, seeds, dstIdx, dstCellID, wf)
	return r.reconstruct(srcIdx, dstIdx, srcCellID, dstCellID, injectionPred, overlayPred, egressPred, wf)
}

// fullGraphSearch runs a global A* on the base graph when the overlay search fails.
func (r *Router) fullGraphSearch(srcIdx, dstIdx uint32, wf WeightFunc) ([]Step, bool) {
	g := r.g
	srcID := g.Nodes[srcIdx].ID
	dstID := g.Nodes[dstIdx].ID
	dstNode := &g.Nodes[dstIdx]

	costs := make(map[builder.NodeID]float32, 256)
	pred := make(map[builder.NodeID]predEntry, 256)
	costs[srcID] = 0

	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return geo.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / maxSearchSpeedMps
	}

	pq := &astarPQ{}
	heap.Push(pq, astarItem{id: srcID, f: heuristic(srcIdx), g: 0})

	for pq.Len() > 0 {
		current := heap.Pop(pq).(astarItem)
		if best, ok := costs[current.id]; ok && current.g > best {
			continue
		}
		if current.id == dstID {
			return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
		}

		currentIdx, ok := g.NodeIdx[current.id]
		if !ok {
			continue
		}

		for _, edgeID := range g.BaseAdj.Neighbours(currentIdx) {
			edge := &g.Edges[edgeID]
			nextIdx, ok := g.NodeIdx[edge.ToNodeID]
			if !ok {
				continue
			}

			nextCost := current.g + wf(edge)
			if best, seen := costs[edge.ToNodeID]; !seen || nextCost < best {
				costs[edge.ToNodeID] = nextCost
				pred[edge.ToNodeID] = predEntry{prevNodeID: current.id, edgeID: edgeID}
				heap.Push(pq, astarItem{
					id: edge.ToNodeID,
					g:  nextCost,
					f:  nextCost + heuristic(nextIdx),
				})
			}
		}
	}

	return nil, false
}

func (r *Router) overlayAStar(
	injectionCosts map[builder.NodeID]float32,
	dstSet map[builder.NodeID]struct{},
	heuristic func(builder.NodeID) float32,
	penalties map[uint32]float32,
) (costs map[builder.NodeID]float32, pred map[builder.NodeID]overlayPredEntry) {
	g := r.g
	costs = make(map[builder.NodeID]float32, len(injectionCosts)+len(dstSet))
	pred = make(map[builder.NodeID]overlayPredEntry, len(injectionCosts)+len(dstSet))

	pq := &astarPQ{}
	for nodeID, cost := range injectionCosts {
		costs[nodeID] = cost
		heap.Push(pq, astarItem{id: nodeID, f: cost + heuristic(nodeID), g: cost})
	}

	settledDestinations := 0
	totalDestinations := len(dstSet)

	for pq.Len() > 0 {
		current := heap.Pop(pq).(astarItem)
		if best, ok := costs[current.id]; ok && current.g > best {
			continue
		}

		if _, isDestination := dstSet[current.id]; isDestination {
			settledDestinations++
			delete(dstSet, current.id)
			if settledDestinations == totalDestinations {
				break
			}
		}

		boundaryIdx, ok := g.BoundaryNodeIdx[current.id]
		if !ok {
			continue
		}

		g.OverlayAdj.Mu.RLock()
		baseEdgeIdx := g.OverlayAdj.Offsets[boundaryIdx]
		for i, overlayEdge := range g.OverlayAdj.Neighbours(boundaryIdx) {
			edgeIdx := baseEdgeIdx + uint32(i)
			weight := overlayEdge.Weight
			if penalty, ok := penalties[edgeIdx]; ok {
				weight *= penalty
			}

			nextCost := current.g + weight
			if best, seen := costs[overlayEdge.ToNodeID]; !seen || nextCost < best {
				costs[overlayEdge.ToNodeID] = nextCost
				pred[overlayEdge.ToNodeID] = overlayPredEntry{
					prevNodeID: current.id,
					edgeIdx:    edgeIdx,
				}
				heap.Push(pq, astarItem{
					id: overlayEdge.ToNodeID,
					g:  nextCost,
					f:  nextCost + heuristic(overlayEdge.ToNodeID),
				})
			}
		}
		g.OverlayAdj.Mu.RUnlock()
	}

	return costs, pred
}

func (r *Router) reconstruct(
	srcIdx, dstIdx uint32,
	srcCellID, dstCellID builder.CellID,
	injPred map[builder.NodeID]predEntry,
	overlayPred map[builder.NodeID]overlayPredEntry,
	egressPred map[builder.NodeID]predEntry,
	wf WeightFunc,
) ([]Step, []uint32, bool) {
	return reconstructPath(r.g, srcIdx, dstIdx, srcCellID, dstCellID, injPred, overlayPred, egressPred, wf)
}

func stepsToRoute(steps []Step, _ *builder.Graph) Route {
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
	item := old[n-1]
	*pq = old[:n-1]
	return item
}

// astarItem carries both the f-score (g+h) and g-score.
type astarItem struct {
	id builder.NodeID
	f  float32
	g  float32
}

type astarPQ []astarItem

func (pq astarPQ) Len() int            { return len(pq) }
func (pq astarPQ) Less(i, j int) bool  { return pq[i].f < pq[j].f }
func (pq astarPQ) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *astarPQ) Push(x interface{}) { *pq = append(*pq, x.(astarItem)) }
func (pq *astarPQ) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}

type localAstarItem struct {
	id   builder.NodeID
	f    float32
	g    float32
	hops int
}

type localAstarPQ []localAstarItem

func (pq localAstarPQ) Len() int            { return len(pq) }
func (pq localAstarPQ) Less(i, j int) bool  { return pq[i].f < pq[j].f }
func (pq localAstarPQ) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *localAstarPQ) Push(x interface{}) { *pq = append(*pq, x.(localAstarItem)) }
func (pq *localAstarPQ) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}

// LocalRepairOverlay searches for a short overlay detour around a congested cross-cell edge.
func (r *Router) LocalRepairOverlay(
	srcNodeID, dstNodeID builder.NodeID,
	maxCost float32,
	maxHops int,
	wf WeightFunc,
) ([]Step, bool) {
	g := r.g
	if _, ok := g.BoundaryNodeIdx[srcNodeID]; !ok {
		return nil, false
	}

	dstNode := g.NodeByID(dstNodeID)
	if dstNode == nil {
		return nil, false
	}

	costs := make(map[builder.NodeID]float32)
	pred := make(map[builder.NodeID]overlayPredEntry)
	heuristic := func(nodeID builder.NodeID) float32 {
		node := g.NodeByID(nodeID)
		if node == nil {
			return 0
		}
		return geo.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / maxSearchSpeedMps
	}

	costs[srcNodeID] = 0
	pq := &localAstarPQ{}
	heap.Push(pq, localAstarItem{id: srcNodeID, f: heuristic(srcNodeID), g: 0, hops: 0})

	for pq.Len() > 0 {
		current := heap.Pop(pq).(localAstarItem)
		if best, ok := costs[current.id]; ok && current.g > best {
			continue
		}
		if current.id == dstNodeID {
			steps, _, terminalID := walkOverlayBack(g, dstNodeID, 0, pred, wf)
			if terminalID != srcNodeID {
				return nil, false
			}
			reverseSteps(steps)
			return steps, true
		}
		if current.hops >= maxHops {
			continue
		}

		boundaryIdx, ok := g.BoundaryNodeIdx[current.id]
		if !ok {
			continue
		}

		g.OverlayAdj.Mu.RLock()
		baseEdgeIdx := g.OverlayAdj.Offsets[boundaryIdx]
		for i, overlayEdge := range g.OverlayAdj.Neighbours(boundaryIdx) {
			edgeIdx := baseEdgeIdx + uint32(i)
			if current.id == srcNodeID && overlayEdge.ToNodeID == dstNodeID {
				continue
			}

			nextCost := current.g + overlayEdge.Weight
			if nextCost > maxCost {
				continue
			}

			if best, seen := costs[overlayEdge.ToNodeID]; !seen || nextCost < best {
				costs[overlayEdge.ToNodeID] = nextCost
				pred[overlayEdge.ToNodeID] = overlayPredEntry{
					prevNodeID: current.id,
					edgeIdx:    edgeIdx,
				}
				heap.Push(pq, localAstarItem{
					id:   overlayEdge.ToNodeID,
					g:    nextCost,
					f:    nextCost + heuristic(overlayEdge.ToNodeID),
					hops: current.hops + 1,
				})
			}
		}
		g.OverlayAdj.Mu.RUnlock()
	}

	return nil, false
}
