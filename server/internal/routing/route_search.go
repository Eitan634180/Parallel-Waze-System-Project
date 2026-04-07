package routing

import (
	"nav-system/internal/geo"
	"nav-system/internal/graph/builder"
	"nav-system/internal/utilities"
)

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

	// Dijkstra to source cell boundary nodes
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

	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return geo.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / maxSearchSpeedMps
	}

	// A* on overlay graph
	overlayCosts, overlayPred := r.overlayAStar(overlaySeeds, dstBoundarySet, heuristic, overlayPenalties)

	// Dijkstra from dest cell boundary nodes
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

	pq := utilities.NewHeap(func(a, b astarItem) bool { return a.f < b.f })
	pq.Push(astarItem{id: srcID, idx: srcIdx, f: heuristic(srcIdx), g: 0})

	for pq.Len() > 0 {
		current := pq.Pop()

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
		if best, ok := costs[current.id]; ok && current.g > best {
			continue
		}

		if current.id == dstID {
			return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
		}

		currentIdx := current.idx
		for _, edgeID := range g.BaseAdj.Neighbours(currentIdx) {
			edge := &g.Edges[edgeID]
			nextIdx := edge.ToNodeIdx

			nextCost := current.g + wf(edge)
			if best, seen := costs[edge.ToNodeID]; !seen || nextCost < best {
				costs[edge.ToNodeID] = nextCost
				pred[edge.ToNodeID] = predEntry{prevNodeID: current.id, edgeID: edgeID}
				pq.Push(astarItem{
					id:  edge.ToNodeID,
					idx: nextIdx,
					g:   nextCost,
					f:   nextCost + heuristic(nextIdx),
				})
			}
		}
	}

	return nil, false
}

func (r *Router) overlayAStar(
	injectionCosts map[builder.NodeID]float32,
	dstSet map[builder.NodeID]struct{},
	heuristic func(uint32) float32,
	penalties map[uint32]float32,
) (costs map[builder.NodeID]float32, pred map[builder.NodeID]overlayPredEntry) {
	g := r.g
	costs = make(map[builder.NodeID]float32, len(injectionCosts)+len(dstSet))
	pred = make(map[builder.NodeID]overlayPredEntry, len(injectionCosts)+len(dstSet))

	pq := utilities.NewHeap(func(a, b astarItem) bool { return a.f < b.f })
	for nodeID, cost := range injectionCosts {
		costs[nodeID] = cost
		nodeIdx := g.NodeIdx[nodeID]
		pq.Push(astarItem{id: nodeID, idx: nodeIdx, f: cost + heuristic(nodeIdx), g: cost})
	}

	settledDestinations := 0
	totalDestinations := len(dstSet)

	for pq.Len() > 0 {
		current := pq.Pop()

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
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
				pq.Push(astarItem{
					id:  overlayEdge.ToNodeID,
					idx: overlayEdge.ToNodeIdx,
					g:   nextCost,
					f:   nextCost + heuristic(overlayEdge.ToNodeIdx),
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
