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

	for pq.Len() > 0 {
		current := pq.Pop()
		if best, ok := costs[current.idx]; ok && current.g > best {
			continue
		}
		stats.RecordVisitedNode()

		if current.idx == dstIdx {
			return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
		}

		for _, edgeID := range g.Base.Neighbours(current.idx) {
			edge := &g.Edges[edgeID]
			nextIdx := edge.ToNodeIdx

			nextCost := current.g + wf(edge)
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = BasePredecessor{PrevNodeIdx: current.idx, EdgeID: edgeID}
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

	for pq.Len() > 0 {
		current := pq.Pop()
		if best, ok := costs[current.idx]; ok && current.cost > best {
			continue
		}
		stats.RecordVisitedNode()

		if current.idx == dstIdx {
			return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
		}

		for _, edgeID := range g.Base.Neighbours(current.idx) {
			edge := &g.Edges[edgeID]
			nextCost := current.cost + wf(edge)
			nextIdx := edge.ToNodeIdx
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = BasePredecessor{PrevNodeIdx: current.idx, EdgeID: edgeID}
				pq.Push(ijItem{idx: nextIdx, cost: nextCost})
			}
		}
	}

	return nil, false
}

func OverlayAStar(
	g *model.Graph,
	injectionCosts map[uint32]float32,
	dstSet map[uint32]struct{},
	heuristic func(uint32) float32,
	overlayWeight func(uint32, float32) float32,
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

	for pq.Len() > 0 {
		current := pq.Pop()
		if best, ok := costs[current.idx]; ok && current.g > best {
			continue
		}
		stats.RecordVisitedNode()

		if _, isDestination := dstSet[current.idx]; isDestination {
			settledDestinations++
			delete(dstSet, current.idx)
			if settledDestinations == totalDestinations {
				break
			}
		}

		boundaryIdx := g.GateNodeIdx[current.idx]
		if boundaryIdx == -1 {
			continue
		}

		baseEdgeIdx := g.Overlay.Offsets[boundaryIdx]
		for i, overlayEdge := range g.Overlay.Neighbours(uint32(boundaryIdx)) {
			edgeIdx := baseEdgeIdx + uint32(i)
			weight := overlayWeight(edgeIdx, overlayEdge.Weight)
			if penalty, ok := penalties[edgeIdx]; ok {
				weight *= penalty
			}

			nextIdx := overlayEdge.ToNodeIdx
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

	return costs, pred
}
