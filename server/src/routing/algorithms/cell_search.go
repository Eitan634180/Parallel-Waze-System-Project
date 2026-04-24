package algorithms

import (
	"nav-system/src/graph/model"
	"nav-system/src/routing"
	"nav-system/src/routing/entities"
	"nav-system/src/utilities"
)

func CellDijkstra(
	g *model.Graph,
	srcInternalIdx uint32,
	targetIdxs []uint32,
	cellID model.CellID,
	wf func(*model.Edge) float32,
	stats *entities.SearchStats,
) (costs map[uint32]float32, pred map[uint32]BasePredecessor) {
	costs = make(map[uint32]float32, len(targetIdxs)+1)
	pred = make(map[uint32]BasePredecessor, len(targetIdxs))

	targetSet := make(map[uint32]struct{}, len(targetIdxs))
	for _, idx := range targetIdxs {
		targetSet[idx] = struct{}{}
	}
	remaining := len(targetSet)

	costs[srcInternalIdx] = 0

	pq := utilities.NewHeap(func(a, b ijItem) bool { return a.cost < b.cost })
	pq.Push(ijItem{idx: srcInternalIdx, cost: 0})

	visitedCount := 0
	for pq.Len() > 0 {
		current := pq.Pop()
		if best, ok := costs[current.idx]; ok && current.cost > best {
			continue
		}
		visitedCount++

		if _, isTarget := targetSet[current.idx]; isTarget {
			remaining--
			delete(targetSet, current.idx)
			if remaining == 0 {
				break
			}
		}

		for _, edgeID := range g.Base.Neighbours(current.idx) {
			edge := &g.Edges[edgeID]
			nextIdx := edge.ToNodeIdx
			if g.Nodes[nextIdx].CellID != cellID {
				continue
			}

			nextCost := current.cost + wf(edge)
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = BasePredecessor{PrevNodeIdx: current.idx, EdgeID: edgeID}
				pq.Push(ijItem{idx: nextIdx, cost: nextCost})
			}
		}
	}

	stats.RecordVisitedNodes(visitedCount)
	return costs, pred
}

func IntraSearch(
	g *model.Graph,
	srcIdx, dstIdx uint32,
	wf func(*model.Edge) float32,
	stats *entities.SearchStats,
) ([]entities.Step, bool) {
	cellID := g.Nodes[srcIdx].CellID

	costs, pred := CellDijkstra(g, srcIdx, []uint32{dstIdx}, cellID, wf, stats)
	if _, reached := costs[dstIdx]; !reached {
		return nil, false
	}

	return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
}

func MultiSourceCellDijkstra(
	g *model.Graph,
	seeds []Seed,
	dstInternalIdx uint32,
	cellID model.CellID,
	wf func(*model.Edge) float32,
	stats *entities.SearchStats,
) (costs map[uint32]float32, pred map[uint32]BasePredecessor) {
	costs = make(map[uint32]float32, len(seeds)+routing.MultiSourceSearchCapacitySlack)
	pred = make(map[uint32]BasePredecessor, len(seeds)+routing.MultiSourceSearchCapacitySlack)

	pq := utilities.NewHeap(func(a, b ijItem) bool { return a.cost < b.cost })
	for _, seed := range seeds {
		costs[seed.NodeIdx] = seed.Cost
		pq.Push(ijItem{idx: seed.NodeIdx, cost: seed.Cost})
	}

	visitedCount := 0
	for pq.Len() > 0 {
		current := pq.Pop()
		if best, ok := costs[current.idx]; ok && current.cost > best {
			continue
		}
		visitedCount++

		if current.idx == dstInternalIdx {
			break
		}

		for _, edgeID := range g.Base.Neighbours(current.idx) {
			edge := &g.Edges[edgeID]
			nextIdx := edge.ToNodeIdx
			if g.Nodes[nextIdx].CellID != cellID {
				continue
			}

			nextCost := current.cost + wf(edge)
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = BasePredecessor{PrevNodeIdx: current.idx, EdgeID: edgeID}
				pq.Push(ijItem{idx: nextIdx, cost: nextCost})
			}
		}
	}

	stats.RecordVisitedNodes(visitedCount)
	return costs, pred
}

func ExpandCellShortcut(
	g *model.Graph,
	srcIdx, dstIdx uint32,
	wf func(*model.Edge) float32,
) []entities.Step {
	cellID := g.Nodes[srcIdx].CellID
	costs, pred := CellDijkstra(g, srcIdx, []uint32{dstIdx}, cellID, wf, nil)
	if _, reached := costs[dstIdx]; !reached {
		return nil
	}

	steps, _ := walkBaseBack(g, dstIdx, pred, wf)
	return steps
}
