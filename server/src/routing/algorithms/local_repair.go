package algorithms

import (
	"nav-system/src/graph/model"
	"nav-system/src/routing/entities"
	"nav-system/src/utilities"
)

func LocalRepairOverlay(
	g *model.Graph,
	srcIdx, dstIdx uint32,
	maxCost float32,
	maxHops int,
	maxSearchSpeedMps float32,
	wf func(*model.Edge) float32,
) ([]entities.Step, bool) {
	if srcIdx >= uint32(len(g.Nodes)) || dstIdx >= uint32(len(g.Nodes)) {
		return nil, false
	}
	dstNode := &g.Nodes[dstIdx]

	costs := make(map[uint32]float32)
	pred := make(map[uint32]OverlayPredecessor)
	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return utilities.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / maxSearchSpeedMps
	}

	costs[srcIdx] = 0
	pq := utilities.NewHeap(func(a, b localAstarItem) bool { return a.f < b.f })
	pq.Push(localAstarItem{idx: srcIdx, f: heuristic(srcIdx), g: 0, hops: 0})

	for pq.Len() > 0 {
		current := pq.Pop()
		if best, ok := costs[current.idx]; ok && current.g > best {
			continue
		}

		if current.idx == dstIdx {
			steps, terminalIdx := walkOverlayBack(g, dstIdx, pred, wf)
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

		g.Overlay.Mu.RLock()
		baseEdgeIdx := g.Overlay.Offsets[boundaryIdx]
		for i, overlayEdge := range g.Overlay.Neighbours(uint32(boundaryIdx)) {
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
				pred[nextIdx] = OverlayPredecessor{
					PrevNodeIdx: current.idx,
					EdgeIdx:     edgeIdx,
				}
				pq.Push(localAstarItem{
					idx:  nextIdx,
					g:    nextCost,
					f:    nextCost + heuristic(nextIdx),
					hops: current.hops + 1,
				})
			}
		}
		g.Overlay.Mu.RUnlock()
	}

	return nil, false
}

func LocalRepairOriginal(
	g *model.Graph,
	srcIdx, dstIdx uint32,
	maxCost float32,
	maxHops int,
	maxSearchSpeedMps float32,
	wf func(*model.Edge) float32,
) ([]entities.Step, bool) {
	if srcIdx >= uint32(len(g.Nodes)) || dstIdx >= uint32(len(g.Nodes)) {
		return nil, false
	}
	dstNode := &g.Nodes[dstIdx]

	costs := make(map[uint32]float32)
	pred := make(map[uint32]BasePredecessor)
	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return utilities.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / maxSearchSpeedMps
	}

	costs[srcIdx] = 0
	pq := utilities.NewHeap(func(a, b localAstarItem) bool { return a.f < b.f })
	pq.Push(localAstarItem{idx: srcIdx, f: heuristic(srcIdx), g: 0, hops: 0})

	for pq.Len() > 0 {
		current := pq.Pop()
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

		for _, edgeID := range g.Base.Neighbours(current.idx) {
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
				pred[nextIdx] = BasePredecessor{PrevNodeIdx: current.idx, EdgeID: edgeID}
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
