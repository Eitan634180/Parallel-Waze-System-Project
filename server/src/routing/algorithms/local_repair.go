package algorithms

import (
	"math"
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
	overlayWeight func(uint32, *model.OverlayEdge) float32,
) ([]entities.Step, bool) {
	if srcIdx >= uint32(len(g.Nodes)) || dstIdx >= uint32(len(g.Nodes)) {
		return nil, false
	}
	dstNode := &g.Nodes[dstIdx]

	costs := make(map[uint32]float32)
	pred := make(map[uint32]OverlayPredecessor)
	cosDstLat := math.Cos(dstNode.Lat * utilities.DegreesToRadians)
	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return utilities.FastHeuristicDistanceM(node.Lat, node.Lon, dstNode.Lat, dstNode.Lon, cosDstLat) / maxSearchSpeedMps
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

		gateIdx := g.Gates[current.idx]
		if gateIdx == -1 {
			continue
		}

		start := g.Overlay.Offsets[gateIdx]
		end := g.Overlay.Offsets[gateIdx+1]
		for edgeIdx := start; edgeIdx < end; edgeIdx++ {
			overlayEdge := g.Overlay.Edges[edgeIdx]
			if current.idx == srcIdx && overlayEdge.DstNode == dstIdx {
				continue
			}

			nextCost := current.g + overlayWeight(edgeIdx, &overlayEdge)
			if nextCost > maxCost {
				continue
			}

			nextIdx := overlayEdge.DstNode
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
	cosDstLat := math.Cos(dstNode.Lat * utilities.DegreesToRadians)
	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return utilities.FastHeuristicDistanceM(node.Lat, node.Lon, dstNode.Lat, dstNode.Lon, cosDstLat) / maxSearchSpeedMps
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

		start, end := g.Base.EdgeRange(current.idx)
		for eid := start; eid < end; eid++ {
			edge := &g.Edges[eid]
			if current.idx == srcIdx && edge.DstNode == dstIdx {
				continue
			}

			nextCost := current.g + wf(edge)
			if nextCost > maxCost {
				continue
			}

			nextIdx := edge.DstNode
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = BasePredecessor{PrevNodeIdx: current.idx, EdgeID: eid}
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
