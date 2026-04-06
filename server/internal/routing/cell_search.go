package routing

import (
	"nav-system/internal/graph/builder"
	"nav-system/internal/utilities"
)

type predEntry struct {
	prevNodeID builder.NodeID
	edgeID     builder.EdgeID
}

type overlayPredEntry struct {
	prevNodeID builder.NodeID
	edgeIdx    uint32
}

type seedE struct {
	nodeIdx uint32
	cost    float32
}

func cellDijkstra(
	g *builder.Graph,
	srcInternalIdx uint32,
	targetNodeIDs []builder.NodeID,
	cellID builder.CellID,
	wf WeightFunc,
) (costs map[builder.NodeID]float32, pred map[builder.NodeID]predEntry) {
	costs = make(map[builder.NodeID]float32, len(targetNodeIDs)+1)
	pred = make(map[builder.NodeID]predEntry, len(targetNodeIDs))

	targetSet := make(map[builder.NodeID]struct{}, len(targetNodeIDs))
	for _, nodeID := range targetNodeIDs {
		targetSet[nodeID] = struct{}{}
	}
	remaining := len(targetSet)

	sourceID := g.Nodes[srcInternalIdx].ID
	costs[sourceID] = 0

	pq := utilities.NewHeap(func(a, b ijItem) bool { return a.cost < b.cost })
	pq.Push(ijItem{id: sourceID, idx: srcInternalIdx, cost: 0})

	for pq.Len() > 0 {
		current := pq.Pop()

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
		if best, ok := costs[current.id]; ok && current.cost > best {
			continue
		}

		if _, isTarget := targetSet[current.id]; isTarget {
			remaining--
			delete(targetSet, current.id)
			if remaining == 0 {
				break
			}
		}

		currentIdx := current.idx
		for _, edgeID := range g.BaseAdj.Neighbours(currentIdx) {
			edge := &g.Edges[edgeID]
			nextIdx := edge.ToNodeIdx
			if g.Nodes[nextIdx].CellID != cellID {
				continue
			}

			nextCost := current.cost + wf(edge)
			if best, seen := costs[edge.ToNodeID]; !seen || nextCost < best {
				costs[edge.ToNodeID] = nextCost
				pred[edge.ToNodeID] = predEntry{prevNodeID: current.id, edgeID: edgeID}
				pq.Push(ijItem{id: edge.ToNodeID, idx: nextIdx, cost: nextCost})
			}
		}
	}

	return costs, pred
}

func intraSearch(g *builder.Graph, srcIdx, dstIdx uint32, wf WeightFunc) ([]Step, bool) {
	cellID := g.Nodes[srcIdx].CellID
	dstID := g.Nodes[dstIdx].ID

	costs, pred := cellDijkstra(g, srcIdx, []builder.NodeID{dstID}, cellID, wf)
	if _, reached := costs[dstID]; !reached {
		return nil, false
	}

	return backtrackBase(srcIdx, dstIdx, pred, g, wf), true
}

func multiSourceCellDijkstra(
	g *builder.Graph,
	seeds []seedE,
	dstInternalIdx uint32,
	cellID builder.CellID,
	wf WeightFunc,
) (costs map[builder.NodeID]float32, pred map[builder.NodeID]predEntry) {
	costs = make(map[builder.NodeID]float32, len(seeds)+16)
	pred = make(map[builder.NodeID]predEntry, len(seeds)+16)
	dstID := g.Nodes[dstInternalIdx].ID

	pq := utilities.NewHeap(func(a, b ijItem) bool { return a.cost < b.cost })
	for _, seed := range seeds {
		nodeID := g.Nodes[seed.nodeIdx].ID
		costs[nodeID] = seed.cost
		pq.Push(ijItem{id: nodeID, idx: seed.nodeIdx, cost: seed.cost})
	}

	for pq.Len() > 0 {
		current := pq.Pop()

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
		if best, ok := costs[current.id]; ok && current.cost > best {
			continue
		}

		if current.id == dstID {
			break
		}

		currentIdx := current.idx
		for _, edgeID := range g.BaseAdj.Neighbours(currentIdx) {
			edge := &g.Edges[edgeID]
			nextIdx := edge.ToNodeIdx
			if g.Nodes[nextIdx].CellID != cellID {
				continue
			}

			nextCost := current.cost + wf(edge)
			if best, seen := costs[edge.ToNodeID]; !seen || nextCost < best {
				costs[edge.ToNodeID] = nextCost
				pred[edge.ToNodeID] = predEntry{prevNodeID: current.id, edgeID: edgeID}
				pq.Push(ijItem{id: edge.ToNodeID, idx: nextIdx, cost: nextCost})
			}
		}
	}

	return costs, pred
}

func expandCellShortcut(g *builder.Graph, srcID, dstID builder.NodeID, wf WeightFunc) []Step {
	srcIdx, srcOK := g.NodeIdx[srcID]
	_, dstOK := g.NodeIdx[dstID]
	if !srcOK || !dstOK {
		return nil
	}

	cellID := g.Nodes[srcIdx].CellID
	costs, pred := cellDijkstra(g, srcIdx, []builder.NodeID{dstID}, cellID, wf)
	if _, reached := costs[dstID]; !reached {
		return nil
	}

	steps, _ := walkBaseBack(g, dstID, pred, wf)
	return steps
}
