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

type customizationIndex struct {
	crossCellOverlayByBaseEdge []uint32
	crossCellBaseEdgeIDs       []builder.EdgeID
	shortcutTargetsByBoundary  [][]shortcutTarget
	weightSnapshotMu           sync.Mutex
	weightMultipliers          []float32
}

type liveDijkstraScratch struct {
	dist      []float32
	seenEpoch []uint32
	epoch     uint32
}

type CustomizationProfile struct {
	Workers              int
	DirtyEdges           int
	CrossCellUpdates     int
	AffectedCells        int
	CellsProcessed       int
	DijkstraRuns         int
	SnapshotTime         time.Duration
	CrossCellTime        time.Duration
	AffectedCellsTime    time.Duration
	WorkerPhaseTime      time.Duration
	ApplyTime            time.Duration
	TotalTime            time.Duration
	WorkerComputeTotal   time.Duration
	MaxCellComputeTime   time.Duration
	MaxWorkerComputeTime time.Duration
	MinWorkerComputeTime time.Duration
	MaxWorkerCells       int
	MinWorkerCells       int
}

type workerCustomizationProfile struct {
	cells        int
	dijkstraRuns int
	computeTime  time.Duration
	maxCellTime  time.Duration
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
				customizeOverlayWeights(g, store, dirtyEdges, nil)
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
	customizeOverlayWeights(g, store, dirtyEdges, nil)
}

func ProfileCustomizeOverlayWeights(g *builder.Graph, store *Store) CustomizationProfile {
	dirtyEdges := snapshotDirtyEdges(store)
	profile := CustomizationProfile{DirtyEdges: len(dirtyEdges)}
	if len(dirtyEdges) == 0 {
		return profile
	}
	customizeOverlayWeights(g, store, dirtyEdges, &profile)
	return profile
}

func customizeOverlayWeights(g *builder.Graph, store *Store, dirtyEdges map[builder.EdgeID]struct{}, profile *CustomizationProfile) {
	start := time.Now()
	index := getCustomizationIndex(g)
	index.weightSnapshotMu.Lock()
	defer index.weightSnapshotMu.Unlock()

	snapshotStart := time.Now()
	index.weightMultipliers = store.SnapshotWeightMultipliers(index.weightMultipliers, len(g.Edges))
	weights := index.weightMultipliers
	if profile != nil {
		profile.DirtyEdges = len(dirtyEdges)
		profile.SnapshotTime = time.Since(snapshotStart)
	}
	updates := make([]overlayWeightUpdate, 0, len(index.crossCellOverlayByBaseEdge))
	affectedCells := make(map[builder.CellID]struct{}, len(g.Cells))

	affectedCellsStart := time.Now()
	for edgeID := range dirtyEdges {
		if int(edgeID) >= len(g.Edges) {
			continue
		}

		edge := &g.Edges[edgeID]
		fromIdx, ok := g.NodeIdx[edge.FromNodeID]
		if ok {
			fromCellID := g.Nodes[fromIdx].CellID
			if fromCellID == g.Nodes[edge.ToNodeIdx].CellID {
				affectedCells[fromCellID] = struct{}{}
			}
		}
	}
	if profile != nil {
		profile.AffectedCellsTime = time.Since(affectedCellsStart)
	}

	crossCellStart := time.Now()
	for _, edgeID := range index.crossCellBaseEdgeIDs {
		if _, dirty := dirtyEdges[edgeID]; !dirty {
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
	if profile != nil {
		profile.CrossCellTime = time.Since(crossCellStart)
		profile.CrossCellUpdates = len(updates)
	}

	type cellUpdates struct {
		updates []overlayWeightUpdate
	}
	affectedCellIDs := make([]builder.CellID, 0, len(affectedCells))
	for cellID := range affectedCells {
		affectedCellIDs = append(affectedCellIDs, cellID)
	}
	if profile != nil {
		profile.AffectedCells = len(affectedCellIDs)
	}
	cellJobs := make(chan builder.Cell, len(affectedCellIDs))
	cellResults := make(chan cellUpdates, len(affectedCellIDs))

	workerCount := max(runtime.GOMAXPROCS(0), 1)
	if profile != nil {
		profile.Workers = workerCount
	}
	var workerProfiles chan workerCustomizationProfile
	if profile != nil {
		workerProfiles = make(chan workerCustomizationProfile, workerCount)
	}

	var wg sync.WaitGroup
	workerPhaseStart := time.Now()
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scratch := newLiveDijkstraScratch(len(g.Nodes))
			// The work done in every cell is small enough that splitting it into boundary node jobs costs more than it saves
			localProfile := workerCustomizationProfile{}
			for cell := range cellJobs {
				cellStart := time.Now()
				updates, dijkstraRuns := computeCellCustomizationUpdates(g, cell, index, weights, scratch)
				cellResults <- cellUpdates{updates: updates}
				if profile != nil {
					cellElapsed := time.Since(cellStart)
					localProfile.cells++
					localProfile.dijkstraRuns += dijkstraRuns
					localProfile.computeTime += cellElapsed
					if cellElapsed > localProfile.maxCellTime {
						localProfile.maxCellTime = cellElapsed
					}
				}
			}
			if workerProfiles != nil {
				workerProfiles <- localProfile
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
		if workerProfiles != nil {
			close(workerProfiles)
		}
	}()

	for result := range cellResults {
		updates = append(updates, result.updates...)
	}
	if profile != nil {
		profile.WorkerPhaseTime = time.Since(workerPhaseStart)
		profile.MinWorkerComputeTime = time.Duration(1<<63 - 1)
		profile.MinWorkerCells = int(^uint(0) >> 1)
		for workerProfile := range workerProfiles {
			profile.CellsProcessed += workerProfile.cells
			profile.DijkstraRuns += workerProfile.dijkstraRuns
			profile.WorkerComputeTotal += workerProfile.computeTime
			if workerProfile.maxCellTime > profile.MaxCellComputeTime {
				profile.MaxCellComputeTime = workerProfile.maxCellTime
			}
			if workerProfile.computeTime > profile.MaxWorkerComputeTime {
				profile.MaxWorkerComputeTime = workerProfile.computeTime
			}
			if workerProfile.computeTime < profile.MinWorkerComputeTime {
				profile.MinWorkerComputeTime = workerProfile.computeTime
			}
			if workerProfile.cells > profile.MaxWorkerCells {
				profile.MaxWorkerCells = workerProfile.cells
			}
			if workerProfile.cells < profile.MinWorkerCells {
				profile.MinWorkerCells = workerProfile.cells
			}
		}
		if profile.MinWorkerComputeTime == time.Duration(1<<63-1) {
			profile.MinWorkerComputeTime = 0
		}
		if profile.MinWorkerCells == int(^uint(0)>>1) {
			profile.MinWorkerCells = 0
		}
	}

	applyStart := time.Now()
	g.OverlayAdj.Mu.Lock()
	for _, update := range updates {
		g.OverlayAdj.OverlayEdges[update.edgeIdx].Weight = update.weight
	}
	g.OverlayAdj.Mu.Unlock()
	if profile != nil {
		profile.ApplyTime = time.Since(applyStart)
		profile.TotalTime = time.Since(start)
	}

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
) ([]overlayWeightUpdate, int) {
	if len(cell.BoundaryNodeIDs) < 2 {
		return nil, 0
	}

	updates := make([]overlayWeightUpdate, 0, len(cell.BoundaryNodeIDs))
	dijkstraRuns := 0
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

		liveCellDijkstra(g, srcIdx, targets, cell.ID, weights, scratch)
		dijkstraRuns++
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
	return updates, dijkstraRuns
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

func liveCellDijkstra(
	g *builder.Graph,
	srcIdx uint32,
	targets []shortcutTarget,
	cellID builder.CellID,
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
		index.crossCellBaseEdgeIDs = append(index.crossCellBaseEdgeIDs, baseEdgeID)
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
