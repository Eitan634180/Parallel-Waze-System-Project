package store

import (
	"context"
	"log"
	"math"
	"runtime"
	"sync"
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/traffic"
	"nav-system/src/utilities"
)

const noOverlayEdgeIdx = ^uint32(0)

type overlayWeightUpdate struct {
	edgeIdx uint32
	weight  float32
}

type livePQItem struct {
	idx  uint32
	cost float32
}

type shortcutTarget struct {
	toIdx   uint32
	edgeIdx uint32
}

type customizationIndex struct {
	crossCellOverlayByBaseEdge []uint32
	shortcutTargetsByBoundary  [][]shortcutTarget
}

type Customizer struct {
	g                 *model.Graph
	index             *customizationIndex
	mu                sync.Mutex
	weightMultipliers []float32
}

type liveDijkstraScratch struct {
	dist      []float32
	seenEpoch []uint32
	epoch     uint32
	heap      *utilities.Heap[livePQItem]
}

func NewCustomizer(g *model.Graph) *Customizer {
	return &Customizer{
		g:     g,
		index: buildCustomizationIndex(g),
	}
}

// Run periodically customizes overlay weights.
func (c *Customizer) Run(ctx context.Context, store *Store) {
	ticker := time.NewTicker(traffic.CustomizationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Customize(store)
		}
	}
}

// Customize recomputes overlay weights against the current live traffic.
func (c *Customizer) Customize(store *Store) {
	start := time.Now()

	pendingEdges := store.SwapPending()
	if len(pendingEdges) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	g := c.g
	index := c.index
	c.weightMultipliers = store.SnapshotWeightMultipliers(c.weightMultipliers, len(g.Edges))
	weights := c.weightMultipliers
	updates := make([]overlayWeightUpdate, 0, len(pendingEdges))
	affectedCellIDs := make([]model.CellID, 0, len(g.Cells))

	coverage := 0.0
	if len(g.Edges) > 0 {
		coverage = float64(len(pendingEdges)) / float64(len(g.Edges))
	}

	if coverage >= traffic.FullAffectedCellsDirtyCoverage {
		for _, cell := range g.Cells {
			affectedCellIDs = append(affectedCellIDs, cell.ID)
		}
	} else {
		cellSeen := make([]bool, len(g.Cells))
		for _, edgeID := range pendingEdges {
			if int(edgeID) >= len(g.Edges) {
				continue
			}

			edge := &g.Edges[edgeID]
			fromCellID := g.Nodes[edge.FromNodeIdx].CellID
			toCellID := g.Nodes[edge.ToNodeIdx].CellID
			if fromCellID == toCellID && int(fromCellID) < len(cellSeen) && !cellSeen[fromCellID] {
				cellSeen[fromCellID] = true
				affectedCellIDs = append(affectedCellIDs, fromCellID)
			}
		}
	}

	for _, edgeID := range pendingEdges {
		if int(edgeID) >= len(index.crossCellOverlayByBaseEdge) {
			continue
		}

		overlayEdgeIdx := index.crossCellOverlayByBaseEdge[edgeID]
		if overlayEdgeIdx == noOverlayEdgeIdx {
			continue
		}

		updates = append(updates, overlayWeightUpdate{
			edgeIdx: overlayEdgeIdx,
			weight:  liveWeightFromSnapshot(weights, edgeID, g.Edges[edgeID].BaseWeight),
		})
	}

	type cellUpdates struct {
		updates []overlayWeightUpdate
	}
	cellJobs := make(chan model.CellID, len(affectedCellIDs))
	cellResults := make(chan cellUpdates, len(affectedCellIDs))

	workerCount := max(runtime.GOMAXPROCS(0), 1)
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scratch := newLiveDijkstraScratch(len(g.Nodes))
			// The work done in every cell is small enough that splitting it into boundary node jobs costs more than it saves.
			for cellID := range cellJobs {
				updates := computeCellCustomizationUpdates(g, cellID, index, weights, scratch)
				cellResults <- cellUpdates{updates: updates}
			}
		}()
	}

	for _, cellID := range affectedCellIDs {
		if int(cellID) < len(g.Cells) {
			cellJobs <- cellID
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

	g.Overlay.Mu.Lock()
	for _, update := range updates {
		g.Overlay.OverlayEdges[update.edgeIdx].Weight = update.weight
	}
	g.Overlay.Mu.Unlock()

	elapsed := time.Since(start)
	if elapsed >= traffic.SlowCustomizationLogThreshold {
		log.Printf("traffic: overlay customization slow (updates=%d duration=%s)", len(updates), elapsed.Round(time.Millisecond))
	}
}

func computeCellCustomizationUpdates(
	g *model.Graph,
	cellID model.CellID,
	index *customizationIndex,
	weights []float32,
	scratch *liveDijkstraScratch,
) []overlayWeightUpdate {
	if int(cellID) >= len(g.Cells) {
		return nil
	}

	cell := &g.Cells[cellID]
	if len(cell.BoundaryNodeIdxs) < 2 {
		return nil
	}

	updates := make([]overlayWeightUpdate, 0, len(cell.BoundaryNodeIdxs))
	for _, srcIdx := range cell.BoundaryNodeIdxs {
		srcBoundaryIdx := g.BoundaryNodeIdx[srcIdx]
		if srcBoundaryIdx == -1 {
			continue
		}

		targets := index.shortcutTargetsByBoundary[srcBoundaryIdx]
		if len(targets) == 0 {
			continue
		}

		liveCellDijkstra(g, cellID, srcIdx, targets, weights, scratch)
		for _, target := range targets {
			weight, ok := scratch.cost(target.toIdx)
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

func liveCellDijkstra(
	g *model.Graph,
	cellID model.CellID,
	srcIdx uint32,
	targets []shortcutTarget,
	weights []float32,
	scratch *liveDijkstraScratch,
) {
	scratch.begin()
	scratch.set(srcIdx, 0)

	targetSet := make(map[uint32]struct{}, len(targets))
	for _, target := range targets {
		targetSet[target.toIdx] = struct{}{}
	}
	remaining := len(targetSet)

	scratch.heap.Reset()
	scratch.heap.Push(livePQItem{idx: srcIdx, cost: 0})

	for scratch.heap.Len() > 0 {
		cur := scratch.heap.Pop()

		// Skip stale queue entries after a better path has already been recorded.
		best, hasBest := scratch.cost(cur.idx)
		if !hasBest || cur.cost > best {
			continue
		}

		if _, isTarget := targetSet[cur.idx]; isTarget {
			remaining--
			delete(targetSet, cur.idx)
			if remaining == 0 {
				break
			}
		}

		for _, eid := range g.Base.Neighbours(cur.idx) {
			e := &g.Edges[eid]
			if g.Nodes[e.ToNodeIdx].CellID != cellID {
				continue
			}

			newCost := best + liveWeightFromSnapshot(weights, eid, e.BaseWeight)
			if existing, has := scratch.cost(e.ToNodeIdx); !has || newCost < existing {
				scratch.set(e.ToNodeIdx, newCost)
				scratch.heap.Push(livePQItem{idx: e.ToNodeIdx, cost: newCost})
			}
		}
	}
}

func liveWeightFromSnapshot(weights []float32, id model.EdgeID, baseSec float32) float32 {
	if int(id) >= len(weights) {
		return baseSec
	}
	return baseSec * weights[id]
}

func buildCustomizationIndex(g *model.Graph) *customizationIndex {
	index := &customizationIndex{
		crossCellOverlayByBaseEdge: make([]uint32, len(g.Edges)),
		shortcutTargetsByBoundary:  make([][]shortcutTarget, len(g.BoundaryBaseIdxs)),
	}
	for i := range index.crossCellOverlayByBaseEdge {
		index.crossCellOverlayByBaseEdge[i] = noOverlayEdgeIdx
	}

	g.Overlay.Mu.RLock()
	defer g.Overlay.Mu.RUnlock()

	for overlayEdgeIdx, overlayEdge := range g.Overlay.OverlayEdges {
		if !overlayEdge.IsCrossCell {
			fromBoundaryIdx := g.BoundaryNodeIdx[overlayEdge.FromNodeIdx]
			if fromBoundaryIdx != -1 {
				index.shortcutTargetsByBoundary[fromBoundaryIdx] = append(
					index.shortcutTargetsByBoundary[fromBoundaryIdx],
					shortcutTarget{toIdx: overlayEdge.ToNodeIdx, edgeIdx: uint32(overlayEdgeIdx)},
				)
			}
			continue
		}

		baseEdgeID, ok := baseEdgeIDBetweenNodeIdxs(g, overlayEdge.FromNodeIdx, overlayEdge.ToNodeIdx)
		if !ok || int(baseEdgeID) >= len(index.crossCellOverlayByBaseEdge) {
			continue
		}
		index.crossCellOverlayByBaseEdge[baseEdgeID] = uint32(overlayEdgeIdx)
	}

	return index
}

func baseEdgeIDBetweenNodeIdxs(g *model.Graph, fromIdx, toIdx uint32) (model.EdgeID, bool) {
	for _, eid := range g.Base.Neighbours(fromIdx) {
		if g.Edges[eid].ToNodeIdx == toIdx {
			return eid, true
		}
	}
	return 0, false
}

func newLiveDijkstraScratch(nodeCount int) *liveDijkstraScratch {
	return &liveDijkstraScratch{
		dist:      make([]float32, nodeCount),
		seenEpoch: make([]uint32, nodeCount),
		heap:      utilities.NewHeap(func(a, b livePQItem) bool { return a.cost < b.cost }),
	}
}

func (s *liveDijkstraScratch) begin() {
	s.epoch++
	if s.epoch != 0 {
		return
	}

	clear(s.seenEpoch)
	s.epoch = 1
}

func (s *liveDijkstraScratch) cost(idx uint32) (float32, bool) {
	if s.seenEpoch[idx] != s.epoch {
		return math.MaxFloat32, false
	}
	return s.dist[idx], true
}

func (s *liveDijkstraScratch) set(idx uint32, cost float32) {
	s.seenEpoch[idx] = s.epoch
	s.dist[idx] = cost
}
