package algorithms

import (
	"nav-system/src/graph/model"
	"nav-system/src/routing"
	"nav-system/src/routing/entities"
	"nav-system/src/utilities"
)

func FullGraphAStar(
	g *model.Graph,
	srcIdx, dstIdx uint32,
	wf func(*model.Edge) float32,
	maxSearchSpeedMps float32,
	stats *entities.SearchStats,
) ([]entities.Step, bool) {
	dstNode := &g.Nodes[dstIdx]

	costs := make(map[uint32]float32, routing.FullGraphSearchMapCapacity)
	pred := make(map[uint32]BasePredecessor, routing.FullGraphSearchMapCapacity)
	costs[srcIdx] = 0

	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return utilities.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / maxSearchSpeedMps
	}

	pq := utilities.NewHeap(func(a, b astarItem) bool { return a.f < b.f })
	pq.Push(astarItem{idx: srcIdx, f: heuristic(srcIdx), g: 0})

	visitedCount := 0
	for pq.Len() > 0 {
		current := pq.Pop()
		if best, ok := costs[current.idx]; ok && current.g > best {
			continue
		}
		visitedCount++

		if current.idx == dstIdx {
			stats.RecordVisitedNodes(visitedCount)
			return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
		}

		start, end := g.Base.EdgeRange(current.idx)
		for eid := start; eid < end; eid++ {
			edge := &g.Edges[eid]
			nextIdx := edge.DstNode

			nextCost := current.g + wf(edge)
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = BasePredecessor{PrevNodeIdx: current.idx, EdgeID: eid}
				pq.Push(astarItem{
					idx: nextIdx,
					g:   nextCost,
					f:   nextCost + heuristic(nextIdx),
				})
			}
		}
	}

	stats.RecordVisitedNodes(visitedCount)
	return nil, false
}

func FullGraphDijkstra(
	g *model.Graph,
	srcIdx, dstIdx uint32,
	wf func(*model.Edge) float32,
	stats *entities.SearchStats,
) ([]entities.Step, bool) {
	costs := make(map[uint32]float32, routing.FullGraphSearchMapCapacity)
	pred := make(map[uint32]BasePredecessor, routing.FullGraphSearchMapCapacity)
	costs[srcIdx] = 0

	pq := utilities.NewHeap(func(a, b ijItem) bool { return a.cost < b.cost })
	pq.Push(ijItem{idx: srcIdx, cost: 0})

	visitedCount := 0
	for pq.Len() > 0 {
		current := pq.Pop()
		if best, ok := costs[current.idx]; ok && current.cost > best {
			continue
		}
		visitedCount++

		if current.idx == dstIdx {
			stats.RecordVisitedNodes(visitedCount)
			return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
		}

		start, end := g.Base.EdgeRange(current.idx)
		for eid := start; eid < end; eid++ {
			edge := &g.Edges[eid]
			nextCost := current.cost + wf(edge)
			nextIdx := edge.DstNode
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = BasePredecessor{PrevNodeIdx: current.idx, EdgeID: eid}
				pq.Push(ijItem{idx: nextIdx, cost: nextCost})
			}
		}
	}

	stats.RecordVisitedNodes(visitedCount)
	return nil, false
}

func OverlayAStar(
	g *model.Graph,
	injectionCosts map[uint32]float32,
	dstSet map[uint32]struct{},
	heuristic func(uint32) float32,
	overlayWeight func(uint32, *model.OverlayEdge) float32,
	penalties map[uint32]float32,
	stats *entities.SearchStats,
) (costs map[uint32]float32, pred map[uint32]OverlayPredecessor) {
	costs = make(map[uint32]float32, len(injectionCosts)+len(dstSet))
	pred = make(map[uint32]OverlayPredecessor, len(injectionCosts)+len(dstSet))

	pq := utilities.NewHeap(func(a, b astarItem) bool { return a.f < b.f })
	for nodeIdx, cost := range injectionCosts {
		costs[nodeIdx] = cost
		pq.Push(astarItem{idx: nodeIdx, f: cost + heuristic(nodeIdx), g: cost})
	}

	settledDestinations := 0
	totalDestinations := len(dstSet)

	visitedCount := 0
	for pq.Len() > 0 {
		current := pq.Pop()
		if best, ok := costs[current.idx]; ok && current.g > best {
			continue
		}
		visitedCount++

		if _, isDestination := dstSet[current.idx]; isDestination {
			settledDestinations++
			delete(dstSet, current.idx)
			if settledDestinations == totalDestinations {
				break
			}
		}

		gateIdx := g.Gates[current.idx]
		if gateIdx == -1 {
			continue
		}

		start := g.Overlay.Offsets[gateIdx]
		end := g.Overlay.Offsets[gateIdx+1]
		for edgeIdx := start; edgeIdx < end; edgeIdx++ {
			overlayEdge := g.Overlay.Edges[edgeIdx]
			weight := overlayWeight(edgeIdx, &overlayEdge)
			if penalty, ok := penalties[edgeIdx]; ok {
				weight *= penalty
			}

			nextIdx := overlayEdge.DstNode
			nextCost := current.g + weight
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = OverlayPredecessor{
					PrevNodeIdx: current.idx,
					EdgeIdx:     edgeIdx,
				}
				pq.Push(astarItem{
					idx: nextIdx,
					g:   nextCost,
					f:   nextCost + heuristic(nextIdx),
				})
			}
		}
	}

	stats.RecordVisitedNodes(visitedCount)
	return costs, pred
}
