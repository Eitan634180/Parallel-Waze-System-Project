package traffic

import (
	"context"
	"log"
	"math"
	"runtime"
	"sync"
	"time"

	"nav-system/src/graph/builder"
	"nav-system/src/utilities"
)

const customizationInterval = 5 * time.Second
const slowCustomizationLogThreshold = 500 * time.Millisecond
const noOverlayEdgeIdx = ^uint32(0)

type overlayWeightUpdate struct {
	edgeIdx uint32
	weight  float32
}

type livePQItem struct {
	id   builder.NodeID
	cost float32
}

type shortcutTarget struct {
	toID    builder.NodeID
	edgeIdx uint32
}

type customizationIndex struct {
	crossCellOverlayByBaseEdge []uint32
	shortcutTargetsByBoundary  [][]shortcutTarget
}

var customizationIndexMu sync.RWMutex
var customizationIndexCache = make(map[*builder.Graph]*customizationIndex)

// RunCustomization periodically reweights overlay edges using live traffic.
func RunCustomization(ctx context.Context, g *builder.Graph, store *Store) {
	ticker := time.NewTicker(customizationInterval)
	defer ticker.Stop()
	previousDirtyEdges := make(map[builder.EdgeID]struct{})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentDirtyEdges := snapshotDirtyEdges(store)
			dirtyEdges := unionDirtyEdges(currentDirtyEdges, previousDirtyEdges)
			if len(dirtyEdges) > 0 {
				customizeOverlayWeights(g, store, dirtyEdges)
			}
			previousDirtyEdges = currentDirtyEdges
		}
	}
}

// CustomizeOverlayWeights recomputes all overlay edge weights against the
// current live traffic multipliers in store.
func CustomizeOverlayWeights(g *builder.Graph, store *Store) {
	dirtyEdges := snapshotDirtyEdges(store)
	if len(dirtyEdges) == 0 {
		return
	}
	customizeOverlayWeights(g, store, dirtyEdges)
}

func customizeOverlayWeights(g *builder.Graph, store *Store, dirtyEdges map[builder.EdgeID]struct{}) {
	start := time.Now()
	weights := store.SnapshotWeights()
	index := getCustomizationIndex(g)
	updates := make([]overlayWeightUpdate, 0, len(index.crossCellOverlayByBaseEdge))

	for edgeID := range dirtyEdges {
		if int(edgeID) >= len(index.crossCellOverlayByBaseEdge) {
			continue
		}
		overlayEdgeIdx := index.crossCellOverlayByBaseEdge[edgeID]
		if overlayEdgeIdx == noOverlayEdgeIdx {
			continue
		}

		updates = append(updates, overlayWeightUpdate{
			edgeIdx: overlayEdgeIdx,
			weight:  liveWeightFromSnapshot(weights, edgeID, g.Edges[edgeID].Weight),
		})
	}

	type cellUpdates struct {
		updates []overlayWeightUpdate
	}
	affectedCellIDs := affectedCellIDsForDirtyEdges(g, dirtyEdges)
	cellJobs := make(chan builder.Cell, len(affectedCellIDs))
	cellResults := make(chan cellUpdates, len(affectedCellIDs))

	workerCount := max(runtime.GOMAXPROCS(0), 1)

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cell := range cellJobs {
				cellResults <- cellUpdates{updates: computeCellCustomizationUpdates(g, cell, index, weights)}
			}
		}()
	}

	for _, cellID := range affectedCellIDs {
		if int(cellID) < len(g.Cells) {
			cellJobs <- g.Cells[cellID]
		}
	}
	close(cellJobs)

	go func() {
		wg.Wait()
		close(cellResults)
	}()

	for result := range cellResults {
		updates = append(updates, result.updates...)
	}

	g.OverlayAdj.Mu.Lock()
	for _, update := range updates {
		g.OverlayAdj.OverlayEdges[update.edgeIdx].Weight = update.weight
	}
	g.OverlayAdj.Mu.Unlock()

	elapsed := time.Since(start)
	if elapsed >= slowCustomizationLogThreshold {
		log.Printf("traffic: overlay customization slow (updates=%d duration=%s)", len(updates), elapsed.Round(time.Millisecond))
	}
}

func computeCellCustomizationUpdates(
	g *builder.Graph,
	cell builder.Cell,
	index *customizationIndex,
	weights map[builder.EdgeID]float32,
) []overlayWeightUpdate {
	if len(cell.BoundaryNodeIDs) < 2 {
		return nil
	}

	updates := make([]overlayWeightUpdate, 0, len(cell.BoundaryNodeIDs))
	for _, srcID := range cell.BoundaryNodeIDs {
		srcBIdx, ok := g.BoundaryNodeIdx[srcID]
		if !ok {
			continue
		}
		targets := index.shortcutTargetsByBoundary[srcBIdx]
		if len(targets) == 0 {
			continue
		}

		dists := liveCellDijkstra(g, srcID, cell.BoundaryNodeIDs, cell.ID, weights)
		for _, target := range targets {
			weight, ok := dists[target.toID]
			if !ok {
				continue
			}
			updates = append(updates, overlayWeightUpdate{
				edgeIdx: target.edgeIdx,
				weight:  weight,
			})
		}
	}
	return updates
}

func snapshotDirtyEdges(store *Store) map[builder.EdgeID]struct{} {
	store.mu.RLock()
	defer store.mu.RUnlock()

	dirty := make(map[builder.EdgeID]struct{}, len(store.dirty))
	for edgeID := range store.dirty {
		dirty[edgeID] = struct{}{}
	}
	return dirty
}

func unionDirtyEdges(current, previous map[builder.EdgeID]struct{}) map[builder.EdgeID]struct{} {
	if len(current) == 0 && len(previous) == 0 {
		return nil
	}

	combined := make(map[builder.EdgeID]struct{}, len(current)+len(previous))
	for edgeID := range current {
		combined[edgeID] = struct{}{}
	}
	for edgeID := range previous {
		combined[edgeID] = struct{}{}
	}
	return combined
}

func affectedCellIDsForDirtyEdges(g *builder.Graph, dirtyEdges map[builder.EdgeID]struct{}) []builder.CellID {
	if len(dirtyEdges) == 0 {
		return nil
	}

	affected := make(map[builder.CellID]struct{}, len(dirtyEdges))
	for edgeID := range dirtyEdges {
		if int(edgeID) >= len(g.Edges) {
			continue
		}

		edge := &g.Edges[edgeID]
		fromIdx, ok := g.NodeIdx[edge.FromNodeID]
		if !ok {
			continue
		}

		fromCellID := g.Nodes[fromIdx].CellID
		toCellID := g.Nodes[edge.ToNodeIdx].CellID
		if fromCellID == toCellID {
			affected[fromCellID] = struct{}{}
		}
	}

	cellIDs := make([]builder.CellID, 0, len(affected))
	for cellID := range affected {
		cellIDs = append(cellIDs, cellID)
	}
	return cellIDs
}

func liveCellDijkstra(
	g *builder.Graph,
	srcID builder.NodeID,
	targetNodeIDs []builder.NodeID,
	cellID builder.CellID,
	weights map[builder.EdgeID]float32,
) map[builder.NodeID]float32 {
	const inf = float32(math.MaxFloat32)

	dist := make(map[builder.NodeID]float32)
	dist[srcID] = 0

	targetSet := make(map[builder.NodeID]struct{}, len(targetNodeIDs))
	for _, nid := range targetNodeIDs {
		targetSet[nid] = struct{}{}
	}
	remaining := len(targetSet)

	pq := utilities.NewHeap(func(a, b livePQItem) bool { return a.cost < b.cost })
	pq.Push(livePQItem{id: srcID, cost: 0})

	for pq.Len() > 0 {
		cur := pq.Pop()

		// Skip stale queue entries after a better path has already been recorded.
		best, hasBest := dist[cur.id]
		if !hasBest || cur.cost > best {
			continue
		}

		if _, isTarget := targetSet[cur.id]; isTarget {
			remaining--
			delete(targetSet, cur.id)
			if remaining == 0 {
				break
			}
		}

		curIdx, ok := g.NodeIdx[cur.id]
		if !ok {
			continue
		}

		for _, eid := range g.BaseAdj.Neighbours(curIdx) {
			e := &g.Edges[eid]
			toID := e.ToNodeID
			if g.Nodes[e.ToNodeIdx].CellID != cellID {
				continue
			}

			newCost := best + liveWeightFromSnapshot(weights, eid, e.Weight)
			if existing, has := dist[toID]; !has || newCost < existing {
				dist[toID] = newCost
				pq.Push(livePQItem{id: toID, cost: newCost})
			}
		}
	}

	result := make(map[builder.NodeID]float32, len(targetNodeIDs))
	for _, nid := range targetNodeIDs {
		if d, ok := dist[nid]; ok && d < inf {
			result[nid] = d
		}
	}
	return result
}

func liveWeightFromSnapshot(weights map[builder.EdgeID]float32, id builder.EdgeID, baseSec float32) float32 {
	multiplier, ok := weights[id]
	if !ok {
		return baseSec
	}
	return baseSec * multiplier
}

func getCustomizationIndex(g *builder.Graph) *customizationIndex {
	customizationIndexMu.RLock()
	cached := customizationIndexCache[g]
	customizationIndexMu.RUnlock()
	if cached != nil {
		return cached
	}

	built := buildCustomizationIndex(g)
	customizationIndexMu.Lock()
	if existing := customizationIndexCache[g]; existing != nil {
		customizationIndexMu.Unlock()
		return existing
	}
	customizationIndexCache[g] = built
	customizationIndexMu.Unlock()
	return built
}

func buildCustomizationIndex(g *builder.Graph) *customizationIndex {
	index := &customizationIndex{
		crossCellOverlayByBaseEdge: make([]uint32, len(g.Edges)),
		shortcutTargetsByBoundary:  make([][]shortcutTarget, len(g.BoundaryNodes)),
	}
	for i := range index.crossCellOverlayByBaseEdge {
		index.crossCellOverlayByBaseEdge[i] = noOverlayEdgeIdx
	}

	g.OverlayAdj.Mu.RLock()
	defer g.OverlayAdj.Mu.RUnlock()

	for overlayEdgeIdx, overlayEdge := range g.OverlayAdj.OverlayEdges {
		if !overlayEdge.IsCrossCell {
			fromBoundaryIdx, ok := g.BoundaryNodeIdx[overlayEdge.FromNodeID]
			if ok {
				index.shortcutTargetsByBoundary[fromBoundaryIdx] = append(
					index.shortcutTargetsByBoundary[fromBoundaryIdx],
					shortcutTarget{toID: overlayEdge.ToNodeID, edgeIdx: uint32(overlayEdgeIdx)},
				)
			}
			continue
		}

		baseEdgeID, ok := baseEdgeIDBetweenNodeIDs(g, overlayEdge.FromNodeID, overlayEdge.ToNodeID)
		if !ok || int(baseEdgeID) >= len(index.crossCellOverlayByBaseEdge) {
			continue
		}
		index.crossCellOverlayByBaseEdge[baseEdgeID] = uint32(overlayEdgeIdx)
	}

	return index
}

func baseEdgeIDBetweenNodeIDs(g *builder.Graph, fromID, toID builder.NodeID) (builder.EdgeID, bool) {
	fromIdx, ok := g.NodeIdx[fromID]
	if !ok {
		return 0, false
	}
	for _, eid := range g.BaseAdj.Neighbours(fromIdx) {
		if g.Edges[eid].ToNodeID == toID {
			return eid, true
		}
	}
	return 0, false
}
