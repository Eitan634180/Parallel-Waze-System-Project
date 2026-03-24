package traffic

import (
	"container/heap"
	"context"
	"math"
	"runtime"
	"sync"
	"time"

	"nav-system/internal/graph/builder"
)

const customizationInterval = 10 * time.Second

type overlayWeightUpdate struct {
	edgeIdx uint32
	weight  float32
}

type livePQItem struct {
	id   builder.NodeID
	cost float32
}

type livePQ []livePQItem

func (pq livePQ) Len() int            { return len(pq) }
func (pq livePQ) Less(i, j int) bool  { return pq[i].cost < pq[j].cost }
func (pq livePQ) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *livePQ) Push(x interface{}) { *pq = append(*pq, x.(livePQItem)) }
func (pq *livePQ) Pop() interface{} {
	old := *pq
	n := len(old)
	x := old[n-1]
	*pq = old[:n-1]
	return x
}

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
	g.OverlayAdj.Mu.RLock()
	offsets := append([]uint32(nil), g.OverlayAdj.Offsets...)
	overlayEdges := append([]builder.OverlayEdge(nil), g.OverlayAdj.OverlayEdges...)
	g.OverlayAdj.Mu.RUnlock()

	updates := make([]overlayWeightUpdate, 0, len(overlayEdges))

	for idx, oe := range overlayEdges {
		if !oe.IsCrossCell {
			continue
		}
		eid, ok := baseEdgeIDBetweenNodeIDs(g, oe.FromNodeID, oe.ToNodeID)
		if !ok {
			continue
		}
		updates = append(updates, overlayWeightUpdate{
			edgeIdx: uint32(idx),
			weight:  store.LiveWeight(eid, g.Edges[eid].Weight),
		})
	}

	type cellUpdates struct {
		updates []overlayWeightUpdate
	}
	cellJobs := make(chan builder.Cell, len(g.Cells))
	cellResults := make(chan cellUpdates, len(g.Cells))

	workerCount := runtime.NumCPU()
	if workerCount < 1 {
		workerCount = 1
	}

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cell := range cellJobs {
				cellResults <- cellUpdates{updates: computeCellCustomizationUpdates(g, store, cell, offsets, overlayEdges)}
			}
		}()
	}

	for _, cell := range g.Cells {
		cellJobs <- cell
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
}

func computeCellCustomizationUpdates(
	g *builder.Graph,
	store *Store,
	cell builder.Cell,
	offsets []uint32,
	overlayEdges []builder.OverlayEdge,
) []overlayWeightUpdate {
	if len(cell.BoundaryNodeIDs) < 2 {
		return nil
	}

	inCell := make(map[builder.NodeID]struct{}, len(cell.InternalNodeIDs))
	for _, nid := range cell.InternalNodeIDs {
		inCell[nid] = struct{}{}
	}

	type shortcutTarget struct {
		toID    builder.NodeID
		edgeIdx uint32
	}

	updates := make([]overlayWeightUpdate, 0, len(cell.BoundaryNodeIDs))
	for _, srcID := range cell.BoundaryNodeIDs {
		srcBIdx, ok := g.BoundaryNodeIdx[srcID]
		if !ok {
			continue
		}

		var targets []shortcutTarget
		for edgeIdx := offsets[srcBIdx]; edgeIdx < offsets[srcBIdx+1]; edgeIdx++ {
			oe := overlayEdges[edgeIdx]
			if oe.IsCrossCell {
				continue
			}
			targets = append(targets, shortcutTarget{toID: oe.ToNodeID, edgeIdx: edgeIdx})
		}
		if len(targets) == 0 {
			continue
		}

		dists := liveCellDijkstra(g, store, srcID, cell.BoundaryNodeIDs, inCell)
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

func liveCellDijkstra(
	g *builder.Graph,
	store *Store,
	srcID builder.NodeID,
	targetNodeIDs []builder.NodeID,
	inCell map[builder.NodeID]struct{},
) map[builder.NodeID]float32 {
	const inf = float32(math.MaxFloat32)

	dist := make(map[builder.NodeID]float32)
	dist[srcID] = 0

	targetSet := make(map[builder.NodeID]struct{}, len(targetNodeIDs))
	for _, nid := range targetNodeIDs {
		targetSet[nid] = struct{}{}
	}
	remaining := len(targetSet)

	pq := &livePQ{}
	heap.Push(pq, livePQItem{id: srcID, cost: 0})

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(livePQItem)

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
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
			if _, ok := inCell[toID]; !ok {
				continue
			}

			newCost := best + store.LiveWeight(eid, e.Weight)
			if existing, has := dist[toID]; !has || newCost < existing {
				dist[toID] = newCost
				heap.Push(pq, livePQItem{id: toID, cost: newCost})
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
