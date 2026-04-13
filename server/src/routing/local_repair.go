package routing

import (
	"nav-system/src/utilities"
	"nav-system/src/graph/builder"
)

// LocalRepairOverlay searches for a short overlay detour around a congested cross-cell edge.
func (r *Router) LocalRepairOverlay(
	srcNodeID, dstNodeID builder.NodeID,
	maxCost float32,
	maxHops int,
	wf WeightFunc,
) ([]Step, bool) {
	g := r.g
	srcIdx, ok := g.NodeIdx[srcNodeID]
	if !ok {
		return nil, false
	}

	dstNode := g.NodeByID(dstNodeID)
	if dstNode == nil {
		return nil, false
	}

	costs := make(map[builder.NodeID]float32)
	pred := make(map[builder.NodeID]overlayPredEntry)
	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return utilities.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / maxSearchSpeedMps
	}

	costs[srcNodeID] = 0
	pq := utilities.NewHeap(func(a, b localAstarItem) bool { return a.f < b.f })
	pq.Push(localAstarItem{id: srcNodeID, idx: srcIdx, f: heuristic(srcIdx), g: 0, hops: 0})

	for pq.Len() > 0 {
		current := pq.Pop()

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
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
				pq.Push(localAstarItem{
					id:   overlayEdge.ToNodeID,
					g:    nextCost,
					f:    nextCost + heuristic(overlayEdge.ToNodeIdx),
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
	srcNodeID, dstNodeID builder.NodeID,
	maxCost float32,
	maxHops int,
	wf WeightFunc,
) ([]Step, bool) {
	g := r.g
	srcIdx, ok := g.NodeIdx[srcNodeID]
	if !ok {
		return nil, false
	}

	dstNode := g.NodeByID(dstNodeID)
	if dstNode == nil {
		return nil, false
	}

	costs := make(map[builder.NodeID]float32)
	pred := make(map[builder.NodeID]predEntry)
	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return utilities.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / maxSearchSpeedMps
	}

	costs[srcNodeID] = 0
	pq := utilities.NewHeap(func(a, b localAstarItem) bool { return a.f < b.f })
	pq.Push(localAstarItem{id: srcNodeID, idx: srcIdx, f: heuristic(srcIdx), g: 0, hops: 0})

	for pq.Len() > 0 {
		current := pq.Pop()

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
		if best, ok := costs[current.id]; ok && current.g > best {
			continue
		}

		if current.id == dstNodeID {
			steps, terminalID := walkBaseBack(g, dstNodeID, pred, wf)
			if terminalID != srcNodeID {
				return nil, false
			}
			reverseSteps(steps)
			return steps, true
		}
		if current.hops >= maxHops {
			continue
		}

		currentIdx := current.idx
		for _, edgeID := range g.BaseAdj.Neighbours(currentIdx) {
			edge := &g.Edges[edgeID]
			if current.id == srcNodeID && edge.ToNodeID == dstNodeID {
				continue
			}

			nextCost := current.g + wf(edge)
			if nextCost > maxCost {
				continue
			}

			if best, seen := costs[edge.ToNodeID]; !seen || nextCost < best {
				costs[edge.ToNodeID] = nextCost
				pred[edge.ToNodeID] = predEntry{
					prevNodeID: current.id,
					edgeID:     edgeID,
				}
				pq.Push(localAstarItem{
					id:   edge.ToNodeID,
					idx:  edge.ToNodeIdx,
					g:    nextCost,
					f:    nextCost + heuristic(edge.ToNodeIdx),
					hops: current.hops + 1,
				})
			}
		}
	}

	return nil, false
}
