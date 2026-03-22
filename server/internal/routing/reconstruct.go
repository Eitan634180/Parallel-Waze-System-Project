package routing

import (
	"container/heap"

	"nav-system/map/builder"
)

// reconstructPath builds the full []Step for a two-level route.
//
// The three predecessor maps come from the three phases in router.go:
//   - injPred:     source-cell Dijkstra  (NodeID -> predEntry)
//   - overlayPred: overlay A*            (NodeID -> overlayPredEntry, boundary nodes only)
//   - egressPred:  dest-cell Dijkstra    (NodeID -> predEntry)
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

	// Step A: walk egressPred backward from dstID until we hit a
	// destination-cell boundary node (the overlay entry point).
	egressSteps, entryBoundaryID := walkBaseBack(g, dstID, egressPred, wf)
	if entryBoundaryID == 0 {
		return nil, nil, false
	}

	// Step B: walk overlayPred backward from entryBoundaryID until we hit a
	// source-cell boundary node (the overlay exit / injection point).
	overlaySteps, overlayEdgeIDs, exitBoundaryID := walkOverlayBack(g, entryBoundaryID, srcCellID, overlayPred, wf)
	if exitBoundaryID == 0 {
		return nil, nil, false
	}

	// Step C: walk injPred backward from exitBoundaryID to srcID.
	srcSteps, termID := walkBaseBack(g, exitBoundaryID, injPred, wf)
	_ = termID

	reverseSteps(srcSteps)
	reverseSteps(overlaySteps)
	reverseSteps(egressSteps)
	reverseUint32s(overlayEdgeIDs)

	all := append(srcSteps, dedup(overlaySteps, srcSteps)...)
	all = append(all, dedup(egressSteps, all)...)

	if len(all) == 0 || all[0].NodeID != srcID {
		firstStep := nodeToStep(g, srcIdx, 0, 0, 0)
		all = append([]Step{firstStep}, all...)
	}
	if len(all) == 0 || all[len(all)-1].NodeID != dstID {
		// dst was already included by egressSteps
	}

	return all, overlayEdgeIDs, true
}

// walkBaseBack traces backward through a base-graph predecessor map.
// Returns the steps in reverse order (last step first) and the terminal nodeID
// where the predecessor chain ended.
func walkBaseBack(
	g *builder.Graph,
	startID builder.NodeID,
	pred map[builder.NodeID]predEntry,
	wf WeightFunc,
) (steps []Step, terminalID builder.NodeID) {
	cur := startID
	for {
		p, ok := pred[cur]
		if !ok {
			return steps, cur
		}
		ni, ok2 := g.NodeIdx[cur]
		if !ok2 {
			return nil, 0
		}
		e := &g.Edges[p.edgeID]
		step := nodeToStep(g, ni, p.edgeID, e.DistanceM, wf(e))
		steps = append(steps, step)
		cur = p.prevNodeID
	}
}

// walkOverlayBack traces backward through the overlay predecessor map.
// Intra-cell shortcuts are expanded to their full base-graph node sequences.
// Returns steps in reverse order, the overlay edge indexes used, and the
// terminal boundary node ID on the source side.
func walkOverlayBack(
	g *builder.Graph,
	startID builder.NodeID,
	srcCellID builder.CellID,
	overlayPred map[builder.NodeID]overlayPredEntry,
	wf WeightFunc,
) (steps []Step, edgeIDs []uint32, terminalID builder.NodeID) {
	cur := startID
	for {
		p, ok := overlayPred[cur]
		if !ok {
			return steps, edgeIDs, cur
		}

		prev := p.prevNodeID
		edgeIDs = append(edgeIDs, p.edgeIdx)
		g.OverlayAdj.Mu.RLock()
		if int(p.edgeIdx) >= len(g.OverlayAdj.OverlayEdges) {
			g.OverlayAdj.Mu.RUnlock()
			return nil, nil, 0
		}
		oe := g.OverlayAdj.OverlayEdges[p.edgeIdx]
		g.OverlayAdj.Mu.RUnlock()

		if oe.IsCrossCell {
			ni, ok3 := g.NodeIdx[cur]
			if !ok3 {
				return nil, nil, 0
			}
			prevIdx, _ := g.NodeIdx[prev]
			edgeID, _, timeSec := baseEdgeBetween(g, prevIdx, cur, wf)
			step := nodeToStep(g, ni, edgeID, g.Edges[edgeID].DistanceM, timeSec)
			steps = append(steps, step)
		} else {
			expanded := expandShortcut(g, prev, cur, wf)
			steps = append(steps, expanded...)
		}

		cur = prev
	}
}

// expandShortcut runs a bounded Dijkstra from srcID to dstID inside their
// shared cell and returns the inner steps in reverse order.
func expandShortcut(g *builder.Graph, srcID, dstID builder.NodeID, wf WeightFunc) []Step {
	srcIdx, ok1 := g.NodeIdx[srcID]
	dstIdx, ok2 := g.NodeIdx[dstID]
	if !ok1 || !ok2 {
		return nil
	}
	cellID := g.Nodes[srcIdx].CellID

	costs := make(map[builder.NodeID]float32)
	pred := make(map[builder.NodeID]predEntry)
	costs[srcID] = 0

	pq := &ijPQ{}
	heap.Push(pq, ijItem{id: srcID, cost: 0})

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(ijItem)
		if c, ok := costs[cur.id]; ok && cur.cost > c {
			continue
		}
		if cur.id == dstID {
			break
		}
		curIdx, ok := g.NodeIdx[cur.id]
		if !ok {
			continue
		}
		for _, eid := range g.BaseAdj.Neighbours(curIdx) {
			e := &g.Edges[eid]
			toIdx, ok3 := g.NodeIdx[e.ToNodeID]
			if !ok3 {
				continue
			}
			if g.Nodes[toIdx].CellID != cellID {
				continue
			}
			nc := cur.cost + wf(e)
			if existing, has := costs[e.ToNodeID]; !has || nc < existing {
				costs[e.ToNodeID] = nc
				pred[e.ToNodeID] = predEntry{cur.id, eid}
				heap.Push(pq, ijItem{id: e.ToNodeID, cost: nc})
			}
		}
	}

	steps, _ := walkBaseBack(g, dstID, pred, wf)
	_ = dstIdx
	return steps
}

func baseEdgeBetween(g *builder.Graph, fromIdx uint32, toID builder.NodeID, wf WeightFunc) (builder.EdgeID, float32, float32) {
	for _, eid := range g.BaseAdj.Neighbours(fromIdx) {
		e := &g.Edges[eid]
		if e.ToNodeID == toID {
			return eid, e.DistanceM, wf(e)
		}
	}
	return 0, 0, 0
}

func nodeToStep(g *builder.Graph, ni uint32, edgeID builder.EdgeID, distM, timeSec float32) Step {
	n := &g.Nodes[ni]
	var edgePtr *uint32
	if distM > 0 || timeSec > 0 {
		edgeVal := uint32(edgeID)
		edgePtr = &edgeVal
	}
	return Step{
		NodeID:      n.ID,
		Lat:         n.Lat,
		Lon:         n.Lon,
		EdgeID:      edgePtr,
		DistanceM:   distM,
		BaseTimeSec: timeSec,
	}
}

func reverseSteps(s []Step) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func reverseUint32s(v []uint32) {
	for i, j := 0, len(v)-1; i < j; i, j = i+1, j-1 {
		v[i], v[j] = v[j], v[i]
	}
}

// dedup returns only the steps from next whose NodeID is not the last NodeID in prev.
func dedup(next, prev []Step) []Step {
	if len(prev) == 0 || len(next) == 0 {
		return next
	}
	if next[0].NodeID == prev[len(prev)-1].NodeID {
		return next[1:]
	}
	return next
}

// backtrackBase builds a Step slice (forward order) from a base pred map,
// used by intraSearch.
func backtrackBase(srcIdx, dstIdx uint32, pred map[builder.NodeID]predEntry, g *builder.Graph, wf WeightFunc) []Step {
	dstID := g.Nodes[dstIdx].ID
	steps, _ := walkBaseBack(g, dstID, pred, wf)
	reverseSteps(steps)
	srcStep := nodeToStep(g, srcIdx, 0, 0, 0)
	return append([]Step{srcStep}, steps...)
}
