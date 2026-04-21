package engine

import (
	coreconfig "nav-system/src/core/config"
	"nav-system/src/utilities"
)

// LocalRepairOverlay searches for a short overlay detour around a congested cross-cell edge.
func (r *Router) LocalRepairOverlay(
	srcIdx, dstIdx uint32,
	maxCost float32,
	maxHops int,
	wf WeightFunc,
) ([]Step, bool) {
	g := r.g
	if srcIdx >= uint32(len(g.Nodes)) || dstIdx >= uint32(len(g.Nodes)) {
		return nil, false
	}
	dstNode := &g.Nodes[dstIdx]

	costs := make(map[uint32]float32)
	pred := make(map[uint32]overlayPredEntry)
	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return utilities.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / coreconfig.RoutingMaxSearchSpeedMps
	}

	costs[srcIdx] = 0
	pq := utilities.NewHeap(func(a, b localAstarItem) bool { return a.f < b.f })
	pq.Push(localAstarItem{idx: srcIdx, f: heuristic(srcIdx), g: 0, hops: 0})

	for pq.Len() > 0 {
		current := pq.Pop()

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
		if best, ok := costs[current.idx]; ok && current.g > best {
			continue
		}

		if current.idx == dstIdx {
			steps, _, terminalIdx := walkOverlayBack(g, dstIdx, 0, pred, wf)
			if terminalIdx != srcIdx {
				return nil, false
			}
			reverseSteps(steps)
			return steps, true
		}
		if current.hops >= maxHops {
			continue
		}

		boundaryIdx := g.BoundaryNodeIdx[current.idx]
		if boundaryIdx == -1 {
			continue
		}

		g.OverlayAdj.Mu.RLock()
		baseEdgeIdx := g.OverlayAdj.Offsets[boundaryIdx]
		for i, overlayEdge := range g.OverlayAdj.Neighbours(uint32(boundaryIdx)) {
			edgeIdx := baseEdgeIdx + uint32(i)
			if current.idx == srcIdx && overlayEdge.ToNodeIdx == dstIdx {
				continue
			}

			nextCost := current.g + overlayEdge.Weight
			if nextCost > maxCost {
				continue
			}

			nextIdx := overlayEdge.ToNodeIdx
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = overlayPredEntry{
					prevNodeIdx: current.idx,
					edgeIdx:     edgeIdx,
				}
				pq.Push(localAstarItem{
					idx:  nextIdx,
					g:    nextCost,
					f:    nextCost + heuristic(nextIdx),
					hops: current.hops + 1,
				})
			}
		}
		g.OverlayAdj.Mu.RUnlock()
	}

	return nil, false
}

// LocalRepairOriginal searches for a short detour around a congested intra-cell edge on the base graph.
func (r *Router) LocalRepairOriginal(
	srcIdx, dstIdx uint32,
	maxCost float32,
	maxHops int,
	wf WeightFunc,
) ([]Step, bool) {
	g := r.g
	if srcIdx >= uint32(len(g.Nodes)) || dstIdx >= uint32(len(g.Nodes)) {
		return nil, false
	}
	dstNode := &g.Nodes[dstIdx]

	costs := make(map[uint32]float32)
	pred := make(map[uint32]predEntry)
	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return utilities.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / coreconfig.RoutingMaxSearchSpeedMps
	}

	costs[srcIdx] = 0
	pq := utilities.NewHeap(func(a, b localAstarItem) bool { return a.f < b.f })
	pq.Push(localAstarItem{idx: srcIdx, f: heuristic(srcIdx), g: 0, hops: 0})

	for pq.Len() > 0 {
		current := pq.Pop()

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
		if best, ok := costs[current.idx]; ok && current.g > best {
			continue
		}

		if current.idx == dstIdx {
			steps, terminalIdx := walkBaseBack(g, dstIdx, pred, wf)
			if terminalIdx != srcIdx {
				return nil, false
			}
			reverseSteps(steps)
			return steps, true
		}
		if current.hops >= maxHops {
			continue
		}

		for _, edgeID := range g.BaseAdj.Neighbours(current.idx) {
			edge := &g.Edges[edgeID]
			if current.idx == srcIdx && edge.ToNodeIdx == dstIdx {
				continue
			}

			nextCost := current.g + wf(edge)
			if nextCost > maxCost {
				continue
			}

			nextIdx := edge.ToNodeIdx
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = predEntry{prevNodeIdx: current.idx, edgeID: edgeID}
				pq.Push(localAstarItem{
					idx:  nextIdx,
					g:    nextCost,
					f:    nextCost + heuristic(nextIdx),
					hops: current.hops + 1,
				})
			}
		}
	}

	return nil, false
}
