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
	idx  uint32
	cost float32
}

type shortcutTarget struct {
	toID    builder.NodeID
	toIdx   uint32
	edgeIdx uint32
}

type customizationJob struct {
	cellID  builder.CellID
	srcID   builder.NodeID
	srcIdx  uint32
	srcBIdx uint32
}

type customizationIndex struct {
	crossCellOverlayByBaseEdge []uint32
	shortcutTargetsByBoundary  [][]shortcutTarget
}

type liveDijkstraScratch struct {
	dist      []float32
	seenEpoch []uint32
	epoch     uint32
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
	jobs := makeCustomizationJobs(g, affectedCellIDs, index)
	cellJobs := make(chan customizationJob, len(jobs))
	cellResults := make(chan cellUpdates, len(jobs))

	workerCount := max(runtime.GOMAXPROCS(0), 1)

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scratch := newLiveDijkstraScratch(len(g.Nodes))
			for job := range cellJobs {
				cellResults <- cellUpdates{updates: computeBoundaryCustomizationUpdates(g, job, index, weights, scratch)}
			}
		}()
	}

	for _, job := range jobs {
		cellJobs <- job
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

func makeCustomizationJobs(
	g *builder.Graph,
	affectedCellIDs []builder.CellID,
	index *customizationIndex,
) []customizationJob {
	if len(affectedCellIDs) == 0 {
		return nil
	}

	jobs := make([]customizationJob, 0, len(affectedCellIDs))
	for _, cellID := range affectedCellIDs {
		if int(cellID) >= len(g.Cells) {
			continue
		}

		cell := g.Cells[cellID]
		for _, srcID := range cell.BoundaryNodeIDs {
			srcBIdx, ok := g.BoundaryNodeIdx[srcID]
			if !ok {
				continue
			}
			targets := index.shortcutTargetsByBoundary[srcBIdx]
			if len(targets) == 0 {
				continue
			}

			srcIdx, ok := g.NodeIdx[srcID]
			if !ok {
				continue
			}

			jobs = append(jobs, customizationJob{
				cellID:  cell.ID,
				srcID:   srcID,
				srcIdx:  srcIdx,
				srcBIdx: srcBIdx,
			})
		}
	}
	return jobs
}

func computeBoundaryCustomizationUpdates(
	g *builder.Graph,
	job customizationJob,
	index *customizationIndex,
	weights map[builder.EdgeID]float32,
	scratch *liveDijkstraScratch,
) []overlayWeightUpdate {
	targets := index.shortcutTargetsByBoundary[job.srcBIdx]
	if len(targets) == 0 {
		return nil
	}

	liveCellDijkstra(g, job.srcIdx, targets, job.cellID, weights, scratch)
	updates := make([]overlayWeightUpdate, 0, len(targets))
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
	srcIdx uint32,
	targets []shortcutTarget,
	cellID builder.CellID,
	weights map[builder.EdgeID]float32,
	scratch *liveDijkstraScratch,
) {
	scratch.begin()
	scratch.set(srcIdx, 0)

	targetSet := make(map[uint32]struct{}, len(targets))
	for _, target := range targets {
		targetSet[target.toIdx] = struct{}{}
	}
	remaining := len(targetSet)

	pq := utilities.NewHeap(func(a, b livePQItem) bool { return a.cost < b.cost })
	pq.Push(livePQItem{idx: srcIdx, cost: 0})

	for pq.Len() > 0 {
		cur := pq.Pop()

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

		for _, eid := range g.BaseAdj.Neighbours(cur.idx) {
			e := &g.Edges[eid]
			if g.Nodes[e.ToNodeIdx].CellID != cellID {
				continue
			}

			newCost := best + liveWeightFromSnapshot(weights, eid, e.Weight)
			if existing, has := scratch.cost(e.ToNodeIdx); !has || newCost < existing {
				scratch.set(e.ToNodeIdx, newCost)
				pq.Push(livePQItem{idx: e.ToNodeIdx, cost: newCost})
			}
		}
	}
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
					shortcutTarget{toID: overlayEdge.ToNodeID, toIdx: overlayEdge.ToNodeIdx, edgeIdx: uint32(overlayEdgeIdx)},
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

func newLiveDijkstraScratch(nodeCount int) *liveDijkstraScratch {
	return &liveDijkstraScratch{
		dist:      make([]float32, nodeCount),
		seenEpoch: make([]uint32, nodeCount),
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
