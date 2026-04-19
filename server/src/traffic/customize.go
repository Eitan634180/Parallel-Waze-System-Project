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
const fullAffectedCellsDirtyCoverage = 0.25

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
	crossCellBaseEdgeIDs       []builder.EdgeID
	shortcutTargetsByBoundary  [][]shortcutTarget
	edgeSourceCellIDs          []builder.CellID
	edgeIsIntraCell            []bool
	intraCellAdj               builder.AdjacencyList
	weightSnapshotMu           sync.Mutex
	weightMultipliers          []float32
	cellBoundaryIdxs  [][]uint32
}

type liveDijkstraScratch struct {
	dist      []float32
	seenEpoch []uint32
	epoch     uint32
	heap      *utilities.Heap[livePQItem]
}

var customizationIndexMu sync.RWMutex
var customizationIndexCache = make(map[*builder.Graph]*customizationIndex)

// RunCustomization periodically reweights overlay edges using live traffic.
func RunCustomization(ctx context.Context, g *builder.Graph, store *Store) {
	ticker := time.NewTicker(customizationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			CustomizeOverlayWeights(g, store)
		}
	}
}

// CustomizeOverlayWeights recomputes all overlay edge weights against the
// current live traffic multipliers in store.
func CustomizeOverlayWeights(g *builder.Graph, store *Store) {
	start := time.Now()
	
	pendingEdges := store.SwapPending()
	if len(pendingEdges) == 0 {
		return
	}

	index := getCustomizationIndex(g)
	index.weightSnapshotMu.Lock()
	defer index.weightSnapshotMu.Unlock()

	index.weightMultipliers = store.SnapshotWeightMultipliers(index.weightMultipliers, len(g.Edges))
	weights := index.weightMultipliers
	updates := make([]overlayWeightUpdate, 0, len(index.crossCellOverlayByBaseEdge))
	affectedCellIDs := make([]builder.CellID, 0, len(g.Cells))
	coverage := 0.0
	if len(g.Edges) > 0 {
		coverage = float64(len(pendingEdges)) / float64(len(g.Edges))
	}
	
	isDirty := make([]bool, len(g.Edges))
	for _, id := range pendingEdges {
		if int(id) < len(isDirty) {
			isDirty[id] = true
		}
	}

	if coverage >= fullAffectedCellsDirtyCoverage {
		affectedCellIDs = affectedCellIDs[:0]
		for _, cell := range g.Cells {
			affectedCellIDs = append(affectedCellIDs, cell.ID)
		}
	} else {
		cellSeen := make([]bool, len(g.Cells))
		for _, edgeID := range pendingEdges {
			if int(edgeID) >= len(index.edgeSourceCellIDs) || !index.edgeIsIntraCell[edgeID] {
				continue
			}
			cID := index.edgeSourceCellIDs[edgeID]
			if !cellSeen[cID] {
				cellSeen[cID] = true
				affectedCellIDs = append(affectedCellIDs, cID)
			}
		}
	}

	for _, edgeID := range index.crossCellBaseEdgeIDs {
		if int(edgeID) >= len(isDirty) || !isDirty[edgeID] {
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
	cellJobs := make(chan builder.Cell, len(affectedCellIDs))
	cellResults := make(chan cellUpdates, len(affectedCellIDs))

	workerCount := max(runtime.GOMAXPROCS(0), 1)
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scratch := newLiveDijkstraScratch(len(g.Nodes))
			// The work done in every cell is small enough that splitting it into boundary node jobs costs more than it saves
			for cell := range cellJobs {
				updates := computeCellCustomizationUpdates(g, cell, index, weights, scratch)
				cellResults <- cellUpdates{updates: updates}
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
	weights []float32,
	scratch *liveDijkstraScratch,
) []overlayWeightUpdate {
	if int(cell.ID) >= len(index.cellBoundaryIdxs) {
		return nil
	}
	cellBIdxs := index.cellBoundaryIdxs[cell.ID]
	if len(cellBIdxs) < 2 {
		return nil
	}

	updates := make([]overlayWeightUpdate, 0, len(cellBIdxs))
	for _, srcBIdx := range cellBIdxs {
		targets := index.shortcutTargetsByBoundary[srcBIdx]
		if len(targets) == 0 {
			continue
		}

		srcIdx := g.BoundaryBaseIdxs[srcBIdx]
		liveCellDijkstra(g, index, srcIdx, targets, weights, scratch)
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
	g *builder.Graph,
	index *customizationIndex,
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

		for _, eid := range index.intraCellAdj.Neighbours(cur.idx) {
			e := &g.Edges[eid]
			newCost := best + liveWeightFromSnapshot(weights, eid, e.Weight)
			if existing, has := scratch.cost(e.ToNodeIdx); !has || newCost < existing {
				scratch.set(e.ToNodeIdx, newCost)
				scratch.heap.Push(livePQItem{idx: e.ToNodeIdx, cost: newCost})
			}
		}
	}
}

func liveWeightFromSnapshot(weights []float32, id builder.EdgeID, baseSec float32) float32 {
	if int(id) >= len(weights) {
		return baseSec
	}
	return baseSec * weights[id]
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
		crossCellBaseEdgeIDs:       make([]builder.EdgeID, 0),
		shortcutTargetsByBoundary:  make([][]shortcutTarget, len(g.BoundaryBaseIdxs)),
		edgeSourceCellIDs:          make([]builder.CellID, len(g.Edges)),
		edgeIsIntraCell:            make([]bool, len(g.Edges)),
		intraCellAdj: builder.AdjacencyList{
			Offsets: make([]uint32, len(g.Nodes)+1),
			EdgeIDs: make([]builder.EdgeID, 0, len(g.BaseAdj.EdgeIDs)),
		},
		cellBoundaryIdxs:  make([][]uint32, len(g.Cells)),
	}
	for i := range index.crossCellOverlayByBaseEdge {
		index.crossCellOverlayByBaseEdge[i] = noOverlayEdgeIdx
	}
	for nodeIdx := uint32(0); int(nodeIdx) < len(g.Nodes); nodeIdx++ {
		sourceCellID := g.Nodes[nodeIdx].CellID
		for _, eid := range g.BaseAdj.Neighbours(nodeIdx) {
			index.edgeSourceCellIDs[eid] = sourceCellID
			if g.Nodes[g.Edges[eid].ToNodeIdx].CellID == sourceCellID {
				index.edgeIsIntraCell[eid] = true
				index.intraCellAdj.EdgeIDs = append(index.intraCellAdj.EdgeIDs, eid)
			}
		}
		index.intraCellAdj.Offsets[nodeIdx+1] = uint32(len(index.intraCellAdj.EdgeIDs))
	}

	for bIdx, nodeIdx := range g.BoundaryBaseIdxs {
		cellID := g.Nodes[nodeIdx].CellID
		if int(cellID) < len(index.cellBoundaryIdxs) {
			index.cellBoundaryIdxs[cellID] = append(index.cellBoundaryIdxs[cellID], uint32(bIdx))
		}
	}

	g.OverlayAdj.Mu.RLock()
	defer g.OverlayAdj.Mu.RUnlock()

	for overlayEdgeIdx, overlayEdge := range g.OverlayAdj.OverlayEdges {
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
		index.crossCellBaseEdgeIDs = append(index.crossCellBaseEdgeIDs, baseEdgeID)
	}

	return index
}

func baseEdgeIDBetweenNodeIdxs(g *builder.Graph, fromIdx, toIdx uint32) (builder.EdgeID, bool) {
	for _, eid := range g.BaseAdj.Neighbours(fromIdx) {
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
