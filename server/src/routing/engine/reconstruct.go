package engine

import (
	"nav-system/src/graph/model"
	"nav-system/src/routing/algorithms"
	"nav-system/src/routing/entities"
	"nav-system/src/utilities"
)

func (r *Router) twoLevelSearch(
	srcIdx, dstIdx uint32,
	wf WeightFunc,
	overlayPenalties map[uint32]float32,
	stats *entities.SearchStats,
) ([]entities.Step, []uint32, bool) {
	g := r.g
	srcNode := &g.Nodes[srcIdx]
	dstNode := &g.Nodes[dstIdx]

	srcCellID := srcNode.CellID
	dstCellID := dstNode.CellID
	if srcCellID == dstCellID {
		steps, ok := algorithms.IntraSearch(g, srcIdx, dstIdx, wf, stats)
		return steps, nil, ok
	}

	srcBoundary := g.Cells[srcCellID].BoundaryNodeIdxs
	injectionCosts, injectionPred := algorithms.CellDijkstra(g, srcIdx, srcBoundary, srcCellID, wf, stats)

	overlaySeeds := make(map[uint32]float32, len(srcBoundary))
	for _, idx := range srcBoundary {
		if cost, ok := injectionCosts[idx]; ok {
			overlaySeeds[idx] = cost
		}
	}
	if len(overlaySeeds) == 0 {
		return nil, nil, false
	}

	dstBoundary := g.Cells[dstCellID].BoundaryNodeIdxs
	dstBoundarySet := make(map[uint32]struct{}, len(dstBoundary))
	for _, idx := range dstBoundary {
		dstBoundarySet[idx] = struct{}{}
	}

	heuristic := func(idx uint32) float32 {
		node := &g.Nodes[idx]
		return utilities.Distance(node.X, node.Y, dstNode.X, dstNode.Y) / r.config.MaxSearchSpeedMps
	}

	overlayCosts, overlayPred := algorithms.OverlayAStar(g, overlaySeeds, dstBoundarySet, heuristic, overlayPenalties, stats)

	seeds := make([]algorithms.Seed, 0, len(dstBoundary))
	for _, idx := range dstBoundary {
		cost, ok := overlayCosts[idx]
		if !ok {
			continue
		}
		seeds = append(seeds, algorithms.Seed{NodeIdx: idx, Cost: cost})
	}
	if len(seeds) == 0 {
		return nil, nil, false
	}

	_, egressPred := algorithms.MultiSourceCellDijkstra(g, seeds, dstIdx, dstCellID, wf, stats)
	return reconstructPath(r.g, srcIdx, dstIdx, srcCellID, dstCellID, injectionPred, overlayPred, egressPred, wf)
}

func reconstructPath(
	g *model.Graph,
	srcIdx, dstIdx uint32,
	srcCellID, dstCellID model.CellID,
	injPred map[uint32]algorithms.BasePredecessor,
	overlayPred map[uint32]algorithms.OverlayPredecessor,
	egressPred map[uint32]algorithms.BasePredecessor,
	wf WeightFunc,
) ([]entities.Step, []uint32, bool) {
	egressSteps, entryBoundaryIdx := walkBaseBack(g, dstIdx, egressPred, wf)
	if entryBoundaryIdx == ^uint32(0) {
		return nil, nil, false
	}

	overlaySteps, overlayEdgeIDs, exitBoundaryIdx := walkOverlayBack(g, entryBoundaryIdx, srcCellID, overlayPred, wf)
	if exitBoundaryIdx == ^uint32(0) {
		return nil, nil, false
	}

	srcSteps, _ := walkBaseBack(g, exitBoundaryIdx, injPred, wf)

	reverseSteps(srcSteps)
	reverseSteps(overlaySteps)
	reverseSteps(egressSteps)
	reverseUint32s(overlayEdgeIDs)

	allSteps := append(srcSteps, dedup(overlaySteps, srcSteps)...)
	allSteps = append(allSteps, dedup(egressSteps, allSteps)...)

	if len(allSteps) == 0 || allSteps[0].NodeIdx != srcIdx {
		firstStep := nodeToStep(g, srcIdx, 0, 0, 0)
		allSteps = append([]entities.Step{firstStep}, allSteps...)
	}
	if len(allSteps) == 0 || allSteps[len(allSteps)-1].NodeIdx != dstIdx {
		return nil, nil, false
	}

	return allSteps, overlayEdgeIDs, true
}

func walkBaseBack(
	g *model.Graph,
	startIdx uint32,
	pred map[uint32]algorithms.BasePredecessor,
	wf WeightFunc,
) (steps []entities.Step, terminalIdx uint32) {
	current := startIdx
	for {
		predecessor, ok := pred[current]
		if !ok {
			return steps, current
		}

		edge := &g.Edges[predecessor.EdgeID]
		steps = append(steps, nodeToStep(g, current, predecessor.EdgeID, edge.DistanceM, wf(edge)))
		current = predecessor.PrevNodeIdx
	}
}

func walkOverlayBack(
	g *model.Graph,
	startIdx uint32,
	srcCellID model.CellID,
	overlayPred map[uint32]algorithms.OverlayPredecessor,
	wf WeightFunc,
) (steps []entities.Step, edgeIDs []uint32, terminalIdx uint32) {
	current := startIdx
	for {
		predecessor, ok := overlayPred[current]
		if !ok {
			return steps, edgeIDs, current
		}

		prevIdx := predecessor.PrevNodeIdx
		edgeIDs = append(edgeIDs, predecessor.EdgeIdx)

		g.Overlay.Mu.RLock()
		if int(predecessor.EdgeIdx) >= len(g.Overlay.OverlayEdges) {
			g.Overlay.Mu.RUnlock()
			return nil, nil, ^uint32(0)
		}
		overlayEdge := g.Overlay.OverlayEdges[predecessor.EdgeIdx]
		g.Overlay.Mu.RUnlock()

		if overlayEdge.IsCrossCell {
			edgeID, _, timeSec := baseEdgeBetween(g, prevIdx, current, wf)
			steps = append(steps, nodeToStep(g, current, edgeID, g.Edges[edgeID].DistanceM, timeSec))
		} else {
			steps = append(steps, algorithms.ExpandCellShortcut(g, prevIdx, current, wf)...)
		}

		current = prevIdx
	}
}

func baseEdgeBetween(g *model.Graph, fromIdx, toIdx uint32, wf WeightFunc) (model.EdgeID, float32, float32) {
	for _, edgeID := range g.Base.Neighbours(fromIdx) {
		edge := &g.Edges[edgeID]
		if edge.ToNodeIdx == toIdx {
			return edgeID, edge.DistanceM, wf(edge)
		}
	}
	return 0, 0, 0
}

func nodeToStep(g *model.Graph, nodeIdx uint32, edgeID model.EdgeID, distM, timeSec float32) entities.Step {
	node := &g.Nodes[nodeIdx]
	var edgePtr *uint32
	if distM > 0 || timeSec > 0 {
		edgeValue := uint32(edgeID)
		edgePtr = &edgeValue
	}
	return entities.Step{
		NodeIdx:     nodeIdx,
		Lat:         node.Lat,
		Lon:         node.Lon,
		EdgeID:      edgePtr,
		DistanceM:   distM,
		BaseTimeSec: timeSec,
	}
}

func reverseSteps(steps []entities.Step) {
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
}

func reverseUint32s(values []uint32) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func dedup(next, prev []entities.Step) []entities.Step {
	if len(prev) == 0 || len(next) == 0 {
		return next
	}
	if next[0].NodeIdx == prev[len(prev)-1].NodeIdx {
		return next[1:]
	}
	return next
}
