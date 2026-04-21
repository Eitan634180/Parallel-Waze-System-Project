package engine

import (
	"nav-system/src/graph/model"
	"nav-system/src/core/utilities"
)

type predEntry struct {
	prevNodeIdx uint32
	edgeID      model.EdgeID
}

type overlayPredEntry struct {
	prevNodeIdx uint32
	edgeIdx     uint32
}

type seedE struct {
	nodeIdx uint32
	cost    float32
}

func cellDijkstra(
	g *model.Graph,
	srcInternalIdx uint32,
	targetIdxs []uint32,
	cellID model.CellID,
	wf WeightFunc,
	stats *SearchStats,
) (costs map[uint32]float32, pred map[uint32]predEntry) {
	costs = make(map[uint32]float32, len(targetIdxs)+1)
	pred = make(map[uint32]predEntry, len(targetIdxs))

	targetSet := make(map[uint32]struct{}, len(targetIdxs))
	for _, idx := range targetIdxs {
		targetSet[idx] = struct{}{}
	}
	remaining := len(targetSet)

	costs[srcInternalIdx] = 0

	pq := utilities.NewHeap(func(a, b ijItem) bool { return a.cost < b.cost })
	pq.Push(ijItem{idx: srcInternalIdx, cost: 0})

	for pq.Len() > 0 {
		current := pq.Pop()

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
		if best, ok := costs[current.idx]; ok && current.cost > best {
			continue
		}
		if stats != nil {
			stats.recordVisitedNode()
		}

		if _, isTarget := targetSet[current.idx]; isTarget {
			remaining--
			delete(targetSet, current.idx)
			if remaining == 0 {
				break
			}
		}

		for _, edgeID := range g.BaseAdj.Neighbours(current.idx) {
			edge := &g.Edges[edgeID]
			nextIdx := edge.ToNodeIdx
			if g.Nodes[nextIdx].CellID != cellID {
				continue
			}

			nextCost := current.cost + wf(edge)
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = predEntry{prevNodeIdx: current.idx, edgeID: edgeID}
				pq.Push(ijItem{idx: nextIdx, cost: nextCost})
			}
		}
	}

	return costs, pred
}

func intraSearch(g *model.Graph, srcIdx, dstIdx uint32, wf WeightFunc, stats *SearchStats) ([]Step, bool) {
	cellID := g.Nodes[srcIdx].CellID

	costs, pred := cellDijkstra(g, srcIdx, []uint32{dstIdx}, cellID, wf, stats)
	if _, reached := costs[dstIdx]; !reached {
		return nil, false
	}

	return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
}

func multiSourceCellDijkstra(
	g *model.Graph,
	seeds []seedE,
	dstInternalIdx uint32,
	cellID model.CellID,
	wf WeightFunc,
	stats *SearchStats,
) (costs map[uint32]float32, pred map[uint32]predEntry) {
	costs = make(map[uint32]float32, len(seeds)+16)
	pred = make(map[uint32]predEntry, len(seeds)+16)

	pq := utilities.NewHeap(func(a, b ijItem) bool { return a.cost < b.cost })
	for _, seed := range seeds {
		costs[seed.nodeIdx] = seed.cost
		pq.Push(ijItem{idx: seed.nodeIdx, cost: seed.cost})
	}

	for pq.Len() > 0 {
		current := pq.Pop()

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
		if best, ok := costs[current.idx]; ok && current.cost > best {
			continue
		}
		if stats != nil {
			stats.recordVisitedNode()
		}

		if current.idx == dstInternalIdx {
			break
		}

		for _, edgeID := range g.BaseAdj.Neighbours(current.idx) {
			edge := &g.Edges[edgeID]
			nextIdx := edge.ToNodeIdx
			if g.Nodes[nextIdx].CellID != cellID {
				continue
			}

			nextCost := current.cost + wf(edge)
			if best, seen := costs[nextIdx]; !seen || nextCost < best {
				costs[nextIdx] = nextCost
				pred[nextIdx] = predEntry{prevNodeIdx: current.idx, edgeID: edgeID}
				pq.Push(ijItem{idx: nextIdx, cost: nextCost})
			}
		}
	}

	return costs, pred
}

func expandCellShortcut(g *model.Graph, srcIdx, dstIdx uint32, wf WeightFunc) []Step {
	cellID := g.Nodes[srcIdx].CellID
	costs, pred := cellDijkstra(g, srcIdx, []uint32{dstIdx}, cellID, wf, nil)
	if _, reached := costs[dstIdx]; !reached {
		return nil
	}

	steps, _ := walkBaseBack(g, dstIdx, pred, wf)
	return steps
}
