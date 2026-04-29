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

type livePQHeap struct {
	items []livePQItem
}

func (h *livePQHeap) Len() int {
	return len(h.items)
}

func (h *livePQHeap) Reset() {
	h.items = h.items[:0]
}

func (h *livePQHeap) Push(item livePQItem) {
	h.items = append(h.items, item)
	child := len(h.items) - 1
	for child > 0 {
		parent := (child - 1) / 2
		if h.items[parent].cost <= h.items[child].cost {
			return
		}
		h.items[parent], h.items[child] = h.items[child], h.items[parent]
		child = parent
	}
}

func (h *livePQHeap) Pop() livePQItem {
	last := len(h.items) - 1
	h.items[0], h.items[last] = h.items[last], h.items[0]
	item := h.items[last]
	h.items = h.items[:last]

	parent := 0
	for {
		left := 2*parent + 1
		if left >= last {
			return item
		}

		best := left
		right := left + 1
		if right < last && h.items[right].cost < h.items[left].cost {
			best = right
		}

		if h.items[parent].cost <= h.items[best].cost {
			return item
		}

		h.items[parent], h.items[best] = h.items[best], h.items[parent]
		parent = best
	}
}

type shortcutTarget struct {
	toIdx   uint32
	edgeIdx uint32
}

type customizationIndex struct {
	offsets          []uint32
	targets          []shortcutTarget
	cellNodeOffsets  []uint32
	maxCellNodeCount int
	localScratch     bool
}

func (idx *customizationIndex) Targets(gateIdx uint32) []shortcutTarget {
	return idx.targets[idx.offsets[gateIdx]:idx.offsets[gateIdx+1]]
}

type Customizer struct {
	g                 *model.Graph
	index             *customizationIndex
	mu                sync.Mutex
	weightMultipliers []float32
}

type liveDijkstraScratch struct {
	dist        []float32
	seenEpoch   []uint32
	targetEpoch []uint32
	epoch       uint32
	targetMark  uint32
	cellStart   uint32
	local       bool
	heap        *livePQHeap
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
	estimatedAffectedShortcutUpdates := min(len(index.targets), len(pendingEdges))
	affectedCellIDs := make([]model.CellID, 0, len(g.Cells))

	coverage := 0.0
	if len(g.Edges) > 0 {
		coverage = float64(len(pendingEdges)) / float64(len(g.Edges))
	}

	if coverage >= traffic.FullAffectedCellsDirtyCoverage {
		for i := range g.Cells {
			affectedCellIDs = append(affectedCellIDs, model.CellID(i))
		}
	} else {
		cellSeen := make([]bool, len(g.Cells))
		for _, edgeID := range pendingEdges {
			if int(edgeID) >= len(g.Edges) {
				continue
			}

			edge := &g.Edges[edgeID]
			fromCellID := g.Nodes[edge.SrcNode].CellID
			toCellID := g.Nodes[edge.DstNode].CellID
			if fromCellID == toCellID && int(fromCellID) < len(cellSeen) && !cellSeen[fromCellID] {
				cellSeen[fromCellID] = true
				affectedCellIDs = append(affectedCellIDs, fromCellID)
			}
		}
	}

	type workerResult struct {
		workerID int
		updates  []overlayWeightUpdate
	}

	workerCount := max(runtime.GOMAXPROCS(0), 1)
	cellJobs := make(chan model.CellID, len(affectedCellIDs))
	workerResults := make(chan workerResult, workerCount)
	scratchNodeCount := len(g.Nodes)
	if index.localScratch {
		scratchNodeCount = index.maxCellNodeCount
	}
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		workerID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			scratch := newLiveDijkstraScratch(scratchNodeCount, index.localScratch)
			localUpdates := make([]overlayWeightUpdate, 0, (estimatedAffectedShortcutUpdates+workerCount-1)/workerCount)
			// The work done in every cell is small enough that splitting it into gate node jobs costs more than it saves.
			for cellID := range cellJobs {
				localUpdates = appendCellCustomizationUpdates(g, cellID, index, weights, scratch, localUpdates)
			}
			workerResults <- workerResult{
				workerID: workerID,
				updates:  localUpdates,
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
		close(workerResults)
	}()

	var totalUpdates int
	workerResultsByID := make([]workerResult, workerCount)
	for result := range workerResults {
		workerResultsByID[result.workerID] = result
		totalUpdates += len(result.updates)
	}

	for _, result := range workerResultsByID {
		store.applyOverlayUpdates(result.updates)
	}

	elapsed := time.Since(start)
	if elapsed >= traffic.SlowCustomizationLogThreshold {
		log.Printf("traffic: overlay customization slow (updates=%d duration=%s)", totalUpdates, elapsed.Round(time.Millisecond))
	}
}

func appendCellCustomizationUpdates(
	g *model.Graph,
	cellID model.CellID,
	index *customizationIndex,
	weights []float32,
	scratch *liveDijkstraScratch,
	updates []overlayWeightUpdate,
) []overlayWeightUpdate {
	if int(cellID) >= len(g.Cells) {
		return updates
	}

	cell := &g.Cells[cellID]
	if len(cell.Gates) < 2 {
		return updates
	}

	for _, srcIdx := range cell.Gates {
		srcGateIdx := g.Gates[srcIdx]
		if srcGateIdx == -1 {
			continue
		}

		targets := index.Targets(uint32(srcGateIdx))
		if len(targets) == 0 {
			continue
		}

		cellStart := uint32(0)
		cellEnd := uint32(0)
		if index.localScratch {
			cellStart = index.cellNodeOffsets[cellID]
			cellEnd = index.cellNodeOffsets[cellID+1]
		}
		if index.localScratch {
			liveCellDijkstraLocal(g, cellStart, cellEnd, srcIdx, targets, weights, scratch)
			for _, target := range targets {
				weight, ok := scratch.costLocal(target.toIdx)
				if !ok {
					continue
				}
				updates = append(updates, overlayWeightUpdate{
					edgeIdx: target.edgeIdx,
					weight:  weight,
				})
			}
			continue
		}

		liveCellDijkstraGlobal(g, cellID, srcIdx, targets, weights, scratch)
		for _, target := range targets {
			weight, ok := scratch.cost(target.toIdx)
			if ok {
				updates = append(updates, overlayWeightUpdate{
					edgeIdx: target.edgeIdx,
					weight:  weight,
				})
			}
		}
	}
	return updates
}

func liveCellDijkstraLocal(
	g *model.Graph,
	cellStart uint32,
	cellEnd uint32,
	srcIdx uint32,
	targets []shortcutTarget,
	weights []float32,
	scratch *liveDijkstraScratch,
) {
	scratch.begin(cellStart)
	scratch.setLocal(srcIdx, 0)

	remaining := scratch.markTargetsLocal(targets)

	scratch.heap.Reset()
	scratch.heap.Push(livePQItem{idx: srcIdx, cost: 0})

	for scratch.heap.Len() > 0 {
		cur := scratch.heap.Pop()

		// Skip stale queue entries after a better path has already been recorded.
		best, hasBest := scratch.costLocal(cur.idx)
		if !hasBest || cur.cost > best {
			continue
		}

		if scratch.consumeTargetLocal(cur.idx) {
			remaining--
			if remaining == 0 {
				break
			}
		}

		start, end := g.Base.EdgeRange(cur.idx)
		for eid := start; eid < end; eid++ {
			e := &g.Edges[eid]
			if e.DstNode < cellStart || e.DstNode >= cellEnd {
				continue
			}

			newCost := best + e.BaseWeight*weights[eid]
			if existing, has := scratch.costLocal(e.DstNode); !has || newCost < existing {
				scratch.setLocal(e.DstNode, newCost)
				scratch.heap.Push(livePQItem{idx: e.DstNode, cost: newCost})
			}
		}
	}
}

func liveCellDijkstraGlobal(
	g *model.Graph,
	cellID model.CellID,
	srcIdx uint32,
	targets []shortcutTarget,
	weights []float32,
	scratch *liveDijkstraScratch,
) {
	scratch.begin(0)
	scratch.set(srcIdx, 0)

	remaining := scratch.markTargets(targets)

	scratch.heap.Reset()
	scratch.heap.Push(livePQItem{idx: srcIdx, cost: 0})

	for scratch.heap.Len() > 0 {
		cur := scratch.heap.Pop()

		best, hasBest := scratch.cost(cur.idx)
		if !hasBest || cur.cost > best {
			continue
		}

		if scratch.consumeTarget(cur.idx) {
			remaining--
			if remaining == 0 {
				break
			}
		}

		start, end := g.Base.EdgeRange(cur.idx)
		for eid := start; eid < end; eid++ {
			e := &g.Edges[eid]
			if g.Nodes[e.DstNode].CellID != cellID {
				continue
			}

			newCost := best + liveWeightFromSnapshot(weights, eid, e.BaseWeight)
			if existing, has := scratch.cost(e.DstNode); !has || newCost < existing {
				scratch.set(e.DstNode, newCost)
				scratch.heap.Push(livePQItem{idx: e.DstNode, cost: newCost})
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
	numGates := len(g.Overlay.Offsets) - 1
	cellNodeOffsets, maxCellNodeCount, localScratch := buildCellNodeOffsets(g)

	shortcutCounts := make([]int, numGates)
	totalShortcuts := 0
	for gateIdx := 0; gateIdx < numGates; gateIdx++ {
		start, end := g.Overlay.Offsets[gateIdx], g.Overlay.Offsets[gateIdx+1]
		for i := start; i < end; i++ {
			if !g.Overlay.Edges[i].IsCrossCell {
				shortcutCounts[gateIdx]++
				totalShortcuts++
			}
		}
	}

	index := &customizationIndex{
		offsets:          make([]uint32, numGates+1),
		targets:          make([]shortcutTarget, totalShortcuts),
		cellNodeOffsets:  cellNodeOffsets,
		maxCellNodeCount: maxCellNodeCount,
		localScratch:     localScratch,
	}

	currentPos := 0
	for gateIdx := 0; gateIdx < numGates; gateIdx++ {
		index.offsets[gateIdx] = uint32(currentPos)
		count := shortcutCounts[gateIdx]
		if count == 0 {
			continue
		}

		writeIdx := 0
		start, end := g.Overlay.Offsets[gateIdx], g.Overlay.Offsets[gateIdx+1]
		for i := start; i < end; i++ {
			edge := &g.Overlay.Edges[i]
			if !edge.IsCrossCell {
				index.targets[currentPos+writeIdx] = shortcutTarget{
					toIdx:   edge.DstNode,
					edgeIdx: edge.StoreIdx,
				}
				writeIdx++
			}
		}
		currentPos += count
	}
	index.offsets[numGates] = uint32(totalShortcuts)

	return index
}

func buildCellNodeOffsets(g *model.Graph) ([]uint32, int, bool) {
	offsets := make([]uint32, len(g.Cells)+1)
	nodeIdx := 0
	maxCellNodeCount := 0

	for cellID := range g.Cells {
		offsets[cellID] = uint32(nodeIdx)
		for nodeIdx < len(g.Nodes) && g.Nodes[nodeIdx].CellID == model.CellID(cellID) {
			nodeIdx++
		}
		cellNodeCount := int(uint32(nodeIdx) - offsets[cellID])
		if cellNodeCount > maxCellNodeCount {
			maxCellNodeCount = cellNodeCount
		}
	}
	offsets[len(g.Cells)] = uint32(nodeIdx)

	return offsets, maxCellNodeCount, nodeIdx == len(g.Nodes)
}

func newLiveDijkstraScratch(nodeCount int, local bool) *liveDijkstraScratch {
	return &liveDijkstraScratch{
		dist:        make([]float32, nodeCount),
		seenEpoch:   make([]uint32, nodeCount),
		targetEpoch: make([]uint32, nodeCount),
		local:       local,
		heap:        &livePQHeap{},
	}
}

func (s *liveDijkstraScratch) begin(cellStart uint32) {
	s.epoch++
	s.cellStart = cellStart
	if s.epoch != 0 {
		return
	}

	clear(s.seenEpoch)
	s.epoch = 1
}

func (s *liveDijkstraScratch) index(nodeIdx uint32) uint32 {
	if !s.local {
		return nodeIdx
	}
	return nodeIdx - s.cellStart
}

func (s *liveDijkstraScratch) localIndex(nodeIdx uint32) uint32 {
	return nodeIdx - s.cellStart
}

func (s *liveDijkstraScratch) markTargets(targets []shortcutTarget) int {
	s.targetMark++
	if s.targetMark == 0 {
		clear(s.targetEpoch)
		s.targetMark = 1
	}

	remaining := 0
	for _, target := range targets {
		scratchIdx := s.index(target.toIdx)
		if s.targetEpoch[scratchIdx] == s.targetMark {
			continue
		}
		s.targetEpoch[scratchIdx] = s.targetMark
		remaining++
	}
	return remaining
}

func (s *liveDijkstraScratch) markTargetsLocal(targets []shortcutTarget) int {
	s.targetMark++
	if s.targetMark == 0 {
		clear(s.targetEpoch)
		s.targetMark = 1
	}

	remaining := 0
	for _, target := range targets {
		scratchIdx := s.localIndex(target.toIdx)
		if s.targetEpoch[scratchIdx] == s.targetMark {
			continue
		}
		s.targetEpoch[scratchIdx] = s.targetMark
		remaining++
	}
	return remaining
}

func (s *liveDijkstraScratch) consumeTarget(idx uint32) bool {
	scratchIdx := s.index(idx)
	if s.targetEpoch[scratchIdx] != s.targetMark {
		return false
	}
	s.targetEpoch[scratchIdx] = 0
	return true
}

func (s *liveDijkstraScratch) consumeTargetLocal(idx uint32) bool {
	scratchIdx := s.localIndex(idx)
	if s.targetEpoch[scratchIdx] != s.targetMark {
		return false
	}
	s.targetEpoch[scratchIdx] = 0
	return true
}

func (s *liveDijkstraScratch) cost(idx uint32) (float32, bool) {
	scratchIdx := s.index(idx)
	if s.seenEpoch[scratchIdx] != s.epoch {
		return math.MaxFloat32, false
	}
	return s.dist[scratchIdx], true
}

func (s *liveDijkstraScratch) costLocal(idx uint32) (float32, bool) {
	scratchIdx := s.localIndex(idx)
	if s.seenEpoch[scratchIdx] != s.epoch {
		return math.MaxFloat32, false
	}
	return s.dist[scratchIdx], true
}

func (s *liveDijkstraScratch) set(idx uint32, cost float32) {
	scratchIdx := s.index(idx)
	s.seenEpoch[scratchIdx] = s.epoch
	s.dist[scratchIdx] = cost
}

func (s *liveDijkstraScratch) setLocal(idx uint32, cost float32) {
	scratchIdx := s.localIndex(idx)
	s.seenEpoch[scratchIdx] = s.epoch
	s.dist[scratchIdx] = cost
}
