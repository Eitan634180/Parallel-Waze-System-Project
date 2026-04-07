package routing

import "nav-system/src/graph/builder"

// reconstructPath builds the full []Step for a two-level route.
func reconstructPath(
	g *builder.Graph,
	srcIdx, dstIdx uint32,
	srcCellID, dstCellID builder.CellID,
	injPred map[builder.NodeID]predEntry,
	overlayPred map[builder.NodeID]overlayPredEntry,
	egressPred map[builder.NodeID]predEntry,
	wf WeightFunc,
) ([]Step, []uint32, bool) {
	srcID := g.Nodes[srcIdx].ID
	dstID := g.Nodes[dstIdx].ID

	egressSteps, entryBoundaryID := walkBaseBack(g, dstID, egressPred, wf)
	if entryBoundaryID == 0 {
		return nil, nil, false
	}

	overlaySteps, overlayEdgeIDs, exitBoundaryID := walkOverlayBack(g, entryBoundaryID, srcCellID, overlayPred, wf)
	if exitBoundaryID == 0 {
		return nil, nil, false
	}

	srcSteps, _ := walkBaseBack(g, exitBoundaryID, injPred, wf)

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
	startID builder.NodeID,
	pred map[builder.NodeID]predEntry,
	wf WeightFunc,
) (steps []Step, terminalID builder.NodeID) {
	current := startID
	for {
		predecessor, ok := pred[current]
		if !ok {
			return steps, current
		}

		nodeIdx, ok := g.NodeIdx[current]
		if !ok {
			return nil, 0
		}

		edge := &g.Edges[predecessor.edgeID]
		steps = append(steps, nodeToStep(g, nodeIdx, predecessor.edgeID, edge.DistanceM, wf(edge)))
		current = predecessor.prevNodeID
	}
}

// walkOverlayBack traces backward through the overlay predecessor map.
func walkOverlayBack(
	g *builder.Graph,
	startID builder.NodeID,
	srcCellID builder.CellID,
	overlayPred map[builder.NodeID]overlayPredEntry,
	wf WeightFunc,
) (steps []Step, edgeIDs []uint32, terminalID builder.NodeID) {
	current := startID
	for {
		predecessor, ok := overlayPred[current]
		if !ok {
			return steps, edgeIDs, current
		}

		prevNodeID := predecessor.prevNodeID
		edgeIDs = append(edgeIDs, predecessor.edgeIdx)

		g.OverlayAdj.Mu.RLock()
		if int(predecessor.edgeIdx) >= len(g.OverlayAdj.OverlayEdges) {
			g.OverlayAdj.Mu.RUnlock()
			return nil, nil, 0
		}
		overlayEdge := g.OverlayAdj.OverlayEdges[predecessor.edgeIdx]
		g.OverlayAdj.Mu.RUnlock()

		if overlayEdge.IsCrossCell {
			nodeIdx, ok := g.NodeIdx[current]
			if !ok {
				return nil, nil, 0
			}

			prevIdx, ok := g.NodeIdx[prevNodeID]
			if !ok {
				return nil, nil, 0
			}

			edgeID, _, timeSec := baseEdgeBetween(g, prevIdx, current, wf)
			steps = append(steps, nodeToStep(g, nodeIdx, edgeID, g.Edges[edgeID].DistanceM, timeSec))
		} else {
			steps = append(steps, expandCellShortcut(g, prevNodeID, current, wf)...)
		}

		current = prevNodeID
	}
}

func baseEdgeBetween(g *builder.Graph, fromIdx uint32, toID builder.NodeID, wf WeightFunc) (builder.EdgeID, float32, float32) {
	for _, edgeID := range g.BaseAdj.Neighbours(fromIdx) {
		edge := &g.Edges[edgeID]
		if edge.ToNodeID == toID {
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
func backtrackBase(srcIdx, dstIdx uint32, pred map[builder.NodeID]predEntry, g *builder.Graph, wf WeightFunc) []Step {
	dstID := g.Nodes[dstIdx].ID
	steps, _ := walkBaseBack(g, dstID, pred, wf)
	reverseSteps(steps)
	srcStep := nodeToStep(g, srcIdx, 0, 0, 0)
	return append([]Step{srcStep}, steps...)
}
