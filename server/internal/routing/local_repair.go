package routing

import (
	"container/heap"

	"nav-system/internal/geo"
	"nav-system/internal/graph/builder"
)

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
