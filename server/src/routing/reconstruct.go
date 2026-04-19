package routing

import "nav-system/src/graph/builder"

// reconstructPath builds the full []Step for a two-level route.
func reconstructPath(
	g *builder.Graph,
	srcIdx, dstIdx uint32,
	srcCellID, dstCellID builder.CellID,
	injPred map[uint32]predEntry,
	overlayPred map[uint32]overlayPredEntry,
	egressPred map[uint32]predEntry,
	wf WeightFunc,
) ([]Step, []uint32, bool) {
	srcID := g.Nodes[srcIdx].ID
	dstID := g.Nodes[dstIdx].ID

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

	if len(allSteps) == 0 || allSteps[0].NodeID != srcID {
		firstStep := nodeToStep(g, srcIdx, 0, 0, 0)
		allSteps = append([]Step{firstStep}, allSteps...)
	}
	if len(allSteps) == 0 || allSteps[len(allSteps)-1].NodeID != dstID {
		return nil, nil, false
	}

	return allSteps, overlayEdgeIDs, true
}

// walkBaseBack traces backward through a base-graph predecessor map.
func walkBaseBack(
	g *builder.Graph,
	startIdx uint32,
	pred map[uint32]predEntry,
	wf WeightFunc,
) (steps []Step, terminalIdx uint32) {
	current := startIdx
	for {
		predecessor, ok := pred[current]
		if !ok {
			return steps, current
		}

		edge := &g.Edges[predecessor.edgeID]
		steps = append(steps, nodeToStep(g, current, predecessor.edgeID, edge.DistanceM, wf(edge)))
		current = predecessor.prevNodeIdx
	}
}

// walkOverlayBack traces backward through the overlay predecessor map.
func walkOverlayBack(
	g *builder.Graph,
	startIdx uint32,
	srcCellID builder.CellID,
	overlayPred map[uint32]overlayPredEntry,
	wf WeightFunc,
) (steps []Step, edgeIDs []uint32, terminalIdx uint32) {
	current := startIdx
	for {
		predecessor, ok := overlayPred[current]
		if !ok {
			return steps, edgeIDs, current
		}

		prevIdx := predecessor.prevNodeIdx
		edgeIDs = append(edgeIDs, predecessor.edgeIdx)

		g.OverlayAdj.Mu.RLock()
		if int(predecessor.edgeIdx) >= len(g.OverlayAdj.OverlayEdges) {
			g.OverlayAdj.Mu.RUnlock()
			return nil, nil, ^uint32(0)
		}
		overlayEdge := g.OverlayAdj.OverlayEdges[predecessor.edgeIdx]
		g.OverlayAdj.Mu.RUnlock()

		if overlayEdge.IsCrossCell {
			edgeID, _, timeSec := baseEdgeBetween(g, prevIdx, current, wf)
			steps = append(steps, nodeToStep(g, current, edgeID, g.Edges[edgeID].DistanceM, timeSec))
		} else {
			steps = append(steps, expandCellShortcut(g, prevIdx, current, wf)...)
		}

		current = prevIdx
	}
}

// baseEdgeBetween finds the base-graph edge from fromIdx to toIdx.
func baseEdgeBetween(g *builder.Graph, fromIdx, toIdx uint32, wf WeightFunc) (builder.EdgeID, float32, float32) {
	for _, edgeID := range g.BaseAdj.Neighbours(fromIdx) {
		edge := &g.Edges[edgeID]
		if edge.ToNodeIdx == toIdx {
			return edgeID, edge.DistanceM, wf(edge)
		}
	}
	return 0, 0, 0
}

func nodeToStep(g *builder.Graph, nodeIdx uint32, edgeID builder.EdgeID, distM, timeSec float32) Step {
	node := &g.Nodes[nodeIdx]
	var edgePtr *uint32
	if distM > 0 || timeSec > 0 {
		edgeValue := uint32(edgeID)
		edgePtr = &edgeValue
	}
	return Step{
		NodeID:      node.ID,
		Lat:         node.Lat,
		Lon:         node.Lon,
		EdgeID:      edgePtr,
		DistanceM:   distM,
		BaseTimeSec: timeSec,
	}
}

func reverseSteps(steps []Step) {
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
}

func reverseUint32s(values []uint32) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func dedup(next, prev []Step) []Step {
	if len(prev) == 0 || len(next) == 0 {
		return next
	}
	if next[0].NodeID == prev[len(prev)-1].NodeID {
		return next[1:]
	}
	return next
}

// backtrackBase builds a Step slice in forward order from a base predecessor map.
func backtrackBase(srcIdx, dstIdx uint32, pred map[uint32]predEntry, g *builder.Graph, wf WeightFunc) []Step {
	steps, _ := walkBaseBack(g, dstIdx, pred, wf)
	reverseSteps(steps)
	srcStep := nodeToStep(g, srcIdx, 0, 0, 0)
	return append([]Step{srcStep}, steps...)
}
