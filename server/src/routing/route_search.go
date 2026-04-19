package routing

import (
	"nav-system/src/utilities"
	"nav-system/src/graph/builder"
)

func (r *Router) twoLevelSearch(
	srcIdx, dstIdx uint32,
	wf WeightFunc,
	overlayPenalties map[uint32]float32,
	stats *SearchStats,
) ([]Step, []uint32, bool) {
	g := r.g
	srcNode := &g.Nodes[srcIdx]
	dstNode := &g.Nodes[dstIdx]

	srcCellID := srcNode.CellID
	dstCellID := dstNode.CellID
	if srcCellID == dstCellID {
		steps, ok := intraSearch(g, srcIdx, dstIdx, wf, stats)
		return steps, nil, ok
	}

	// Dijkstra to source cell boundary nodes
	srcBoundary := g.Cells[srcCellID].BoundaryNodeIDs
	injectionCosts, injectionPred := cellDijkstra(g, srcIdx, srcBoundary, srcCellID, wf, stats)

	// Build overlay seeds keyed by base-graph node index.
	overlaySeeds := make(map[uint32]float32, len(srcBoundary))
	for _, nodeID := range srcBoundary {
		idx, ok := g.NodeIdx[nodeID]
		if !ok {
			continue
		}
		if cost, ok := injectionCosts[idx]; ok {
			overlaySeeds[idx] = cost
		}
	}
	if len(overlaySeeds) == 0 {
		return nil, nil, false
	}

	dstBoundary := g.Cells[dstCellID].BoundaryNodeIDs
	dstBoundarySet := make(map[uint32]struct{}, len(dstBoundary))
	for _, nodeID := range dstBoundary {
		if idx, ok := g.NodeIdx[nodeID]; ok {
			dstBoundarySet[idx] = struct{}{}
		}
	}

	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return utilities.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / maxSearchSpeedMps
	}

	// A* on overlay graph
	overlayCosts, overlayPred := r.overlayAStar(overlaySeeds, dstBoundarySet, heuristic, overlayPenalties, stats)

	// Dijkstra from dest cell boundary nodes
	seeds := make([]seedE, 0, len(dstBoundary))
	for _, nodeID := range dstBoundary {
		nodeIdx, ok := g.NodeIdx[nodeID]
		if !ok {
			continue
		}
		cost, ok := overlayCosts[nodeIdx]
		if !ok {
			continue
		}
		seeds = append(seeds, seedE{nodeIdx: nodeIdx, cost: cost})
	}
	if len(seeds) == 0 {
		return nil, nil, false
	}

	_, egressPred := multiSourceCellDijkstra(g, seeds, dstIdx, dstCellID, wf, stats)
	return r.reconstruct(srcIdx, dstIdx, srcCellID, dstCellID, injectionPred, overlayPred, egressPred, wf)
}

// fullGraphAStar runs a global A* on the base graph.
func (r *Router) fullGraphAStar(srcIdx, dstIdx uint32, wf WeightFunc, stats *SearchStats) ([]Step, bool) {
	g := r.g
	dstNode := &g.Nodes[dstIdx]

	costs := make(map[uint32]float32, 256)
	pred := make(map[uint32]predEntry, 256)
	costs[srcIdx] = 0

	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return utilities.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / maxSearchSpeedMps
	}

	pq := utilities.NewHeap(func(a, b astarItem) bool { return a.f < b.f })
	pq.Push(astarItem{idx: srcIdx, f: heuristic(srcIdx), g: 0})

	for pq.Len() > 0 {
		current := pq.Pop()

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
		if best, ok := costs[current.idx]; ok && current.g > best {
			continue
		}
		stats.recordVisitedNode()

		if current.idx == dstIdx {
			return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
		}

		for _, edgeID := range g.BaseAdj.Neighbours(current.idx) {
			edge := &g.Edges[edgeID]
			nextIdx := edge.ToNodeIdx

			nextCost := current.g + wf(edge)
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = predEntry{prevNodeIdx: current.idx, edgeID: edgeID}
				pq.Push(astarItem{
					idx: nextIdx,
					g:   nextCost,
					f:   nextCost + heuristic(nextIdx),
				})
			}
		}
	}

	return nil, false
}

func (r *Router) fullGraphDijkstra(srcIdx, dstIdx uint32, wf WeightFunc, stats *SearchStats) ([]Step, bool) {
	g := r.g

	costs := make(map[uint32]float32, 256)
	pred := make(map[uint32]predEntry, 256)
	costs[srcIdx] = 0

	pq := utilities.NewHeap(func(a, b ijItem) bool { return a.cost < b.cost })
	pq.Push(ijItem{idx: srcIdx, cost: 0})

	for pq.Len() > 0 {
		current := pq.Pop()
		if best, ok := costs[current.idx]; ok && current.cost > best {
			continue
		}
		stats.recordVisitedNode()

		if current.idx == dstIdx {
			return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
		}

		for _, edgeID := range g.BaseAdj.Neighbours(current.idx) {
			edge := &g.Edges[edgeID]
			nextCost := current.cost + wf(edge)
			nextIdx := edge.ToNodeIdx
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = predEntry{prevNodeIdx: current.idx, edgeID: edgeID}
				pq.Push(ijItem{idx: nextIdx, cost: nextCost})
			}
		}
	}

	return nil, false
}

func (r *Router) overlayAStar(
	injectionCosts map[uint32]float32,
	dstSet map[uint32]struct{},
	heuristic func(uint32) float32,
	penalties map[uint32]float32,
	stats *SearchStats,
) (costs map[uint32]float32, pred map[uint32]overlayPredEntry) {
	g := r.g
	costs = make(map[uint32]float32, len(injectionCosts)+len(dstSet))
	pred = make(map[uint32]overlayPredEntry, len(injectionCosts)+len(dstSet))

	pq := utilities.NewHeap(func(a, b astarItem) bool { return a.f < b.f })
	for nodeIdx, cost := range injectionCosts {
		costs[nodeIdx] = cost
		pq.Push(astarItem{idx: nodeIdx, f: cost + heuristic(nodeIdx), g: cost})
	}

	settledDestinations := 0
	totalDestinations := len(dstSet)

	for pq.Len() > 0 {
		current := pq.Pop()

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
		if best, ok := costs[current.idx]; ok && current.g > best {
			continue
		}
		stats.recordVisitedNode()

		if _, isDestination := dstSet[current.idx]; isDestination {
			settledDestinations++
			delete(dstSet, current.idx)
			if settledDestinations == totalDestinations {
				break
			}
		}

		nodeID := g.Nodes[current.idx].ID
		boundaryIdx, ok := g.BoundaryNodeIdx[nodeID]
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

			nextIdx := overlayEdge.ToNodeIdx
			nextCost := current.g + weight
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = overlayPredEntry{
					prevNodeIdx: current.idx,
					edgeIdx:     edgeIdx,
				}
				pq.Push(astarItem{
					idx: nextIdx,
					g:   nextCost,
					f:   nextCost + heuristic(nextIdx),
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
	injPred map[uint32]predEntry,
	overlayPred map[uint32]overlayPredEntry,
	egressPred map[uint32]predEntry,
	wf WeightFunc,
) ([]Step, []uint32, bool) {
	return reconstructPath(r.g, srcIdx, dstIdx, srcCellID, dstCellID, injPred, overlayPred, egressPred, wf)
}
