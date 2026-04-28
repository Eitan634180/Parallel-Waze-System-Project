package algorithms

import (
	"nav-system/src/graph/model"
	"nav-system/src/routing/entities"
)

func walkBaseBack(
	g *model.Graph,
	startIdx uint32,
	pred map[uint32]BasePredecessor,
	wf func(*model.Edge) float32,
) (steps []entities.Step, terminalIdx uint32) {
	current := startIdx
	for {
		predecessor, ok := pred[current]
		if !ok {
			return steps, current
		}

		edge := &g.Edges[predecessor.EdgeID]
		steps = append(steps, nodeToStep(g, current, predecessor.EdgeID, edge.Length, wf(edge)))
		current = predecessor.PrevNodeIdx
	}
}

func walkOverlayBack(
	g *model.Graph,
	startIdx uint32,
	overlayPred map[uint32]OverlayPredecessor,
	wf func(*model.Edge) float32,
) (steps []entities.Step, terminalIdx uint32) {
	current := startIdx
	for {
		predecessor, ok := overlayPred[current]
		if !ok {
			return steps, current
		}

		prevIdx := predecessor.PrevNodeIdx

		if g.Nodes[prevIdx].CellID != g.Nodes[current].CellID {
			edgeID, _, timeSec := baseEdgeBetween(g, prevIdx, current, wf)
			steps = append(steps, nodeToStep(g, current, edgeID, g.Edges[edgeID].Length, timeSec))
		} else {
			steps = append(steps, ExpandCellShortcut(g, prevIdx, current, wf)...)
		}

		current = prevIdx
	}
}

func backtrackBase(
	srcIdx, dstIdx uint32,
	pred map[uint32]BasePredecessor,
	g *model.Graph,
	wf func(*model.Edge) float32,
) []entities.Step {
	steps, _ := walkBaseBack(g, dstIdx, pred, wf)
	reverseSteps(steps)
	srcStep := nodeToStep(g, srcIdx, 0, 0, 0)
	return append([]entities.Step{srcStep}, steps...)
}

func baseEdgeBetween(g *model.Graph, fromIdx, toIdx uint32, wf func(*model.Edge) float32) (model.EdgeID, float32, float32) {
	start, end := g.Base.EdgeRange(fromIdx)
	for eid := start; eid < end; eid++ {
		edge := &g.Edges[eid]
		if edge.DstNode == toIdx {
			return eid, edge.Length, wf(edge)
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
