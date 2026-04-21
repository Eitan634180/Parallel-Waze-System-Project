package builder

import (
	"log"
	"math"
	"runtime"
	"sync"
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/utilities"
)

// BuildOverlayGraph constructs the overlay adjacency list for all boundary
// nodes. Each cell contributes cross-cell edges and intra-cell shortcuts.
func BuildOverlayGraph(g *model.Graph, numWorkers int) time.Duration {
	if numWorkers <= 0 {
		numWorkers = max(runtime.GOMAXPROCS(0), 1)
	}
	log.Printf("%s building overlay graph with %d workers", cellBuilderLogPrefix, numWorkers)

	type cellResult struct {
		edges []model.OverlayEdge
	}

	jobs := make(chan int, len(g.Cells))
	results := make(chan cellResult, len(g.Cells))
	start := time.Now()
	elapsed := time.Since(start)

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ci := range jobs {
				results <- cellResult{edges: computeCellOverlayEdges(g, &g.Cells[ci])}
			}
		}()
	}

	for i := range g.Cells {
		jobs <- i
	}
	close(jobs)

	go func() {
		wg.Wait()
		elapsed = time.Since(start)
		close(results)
	}()

	adjFrom := make([][]model.OverlayEdge, len(g.BoundaryBaseIdxs))
	totalEdges := 0
	for result := range results {
		for _, edge := range result.edges {
			fromIdx := g.BoundaryNodeIdx[edge.FromNodeIdx]
			if fromIdx == -1 {
				continue
			}
			adjFrom[fromIdx] = append(adjFrom[fromIdx], edge)
			totalEdges++
		}
	}

	offsets := make([]uint32, len(g.BoundaryBaseIdxs)+1)
	edges := make([]model.OverlayEdge, 0, totalEdges)
	for i, neighbours := range adjFrom {
		offsets[i] = uint32(len(edges))
		edges = append(edges, neighbours...)
	}
	offsets[len(g.BoundaryBaseIdxs)] = uint32(len(edges))

	g.OverlayAdj = model.OverlayAdjList{Mu: &sync.RWMutex{}, Offsets: offsets, OverlayEdges: edges}
	log.Printf("%s overlay graph ready (%d edges, %d cross-cell, %d shortcuts)",
		cellBuilderLogPrefix,
		totalEdges,
		countCrossCell(edges),
		totalEdges-countCrossCell(edges),
	)

	return elapsed
}

// DetectBoundaryNodes marks nodes that touch edges crossing a cell boundary.
func DetectBoundaryNodes(g *model.Graph) {
	log.Printf("%s detecting boundary nodes", cellBuilderLogPrefix)

	isBoundary := make([]bool, len(g.Nodes))
	for i := range g.Nodes {
		fromCellID := g.Nodes[i].CellID
		for _, eid := range g.BaseAdj.Neighbours(uint32(i)) {
			e := &g.Edges[eid]
			toIdx := e.ToNodeIdx
			if g.Nodes[toIdx].CellID != fromCellID {
				isBoundary[i] = true
				isBoundary[toIdx] = true
			}
		}
	}

	g.BoundaryBaseIdxs = make([]uint32, 0, boundaryNodeCapacityHint)
	g.BoundaryNodeIdx = make([]int32, len(g.Nodes))
	for i := range g.BoundaryNodeIdx {
		g.BoundaryNodeIdx[i] = -1
	}
	cellBoundary := make(map[model.CellID][]uint32)

	for i, node := range g.Nodes {
		if !isBoundary[i] {
			continue
		}
		bIdx := uint32(len(g.BoundaryBaseIdxs))
		g.BoundaryBaseIdxs = append(g.BoundaryBaseIdxs, uint32(i))
		g.BoundaryNodeIdx[i] = int32(bIdx)
		cellBoundary[node.CellID] = append(cellBoundary[node.CellID], uint32(i))
	}

	for i := range g.Cells {
		g.Cells[i].BoundaryNodeIdxs = cellBoundary[g.Cells[i].ID]
	}

	log.Printf("%s detected %d boundary nodes", cellBuilderLogPrefix, len(g.BoundaryBaseIdxs))
}

func countCrossCell(edges []model.OverlayEdge) int {
	n := 0
	for _, e := range edges {
		if e.IsCrossCell {
			n++
		}
	}
	return n
}

// computeCellOverlayEdges emits cross-cell edges and intra-cell shortcuts for one cell.
func computeCellOverlayEdges(g *model.Graph, cell *model.Cell) []model.OverlayEdge {
	var result []model.OverlayEdge

	for _, fromIdx := range cell.InternalNodeIdxs {
		if g.Nodes[fromIdx].CellID != cell.ID {
			continue
		}
		if g.BoundaryNodeIdx[fromIdx] == -1 {
			continue
		}

		for _, eid := range g.BaseAdj.Neighbours(fromIdx) {
			e := &g.Edges[eid]
			toIdx := e.ToNodeIdx

			if g.Nodes[toIdx].CellID == cell.ID {
				continue
			}
			if g.BoundaryNodeIdx[toIdx] == -1 {
				continue
			}
			result = append(result, model.OverlayEdge{
				FromNodeIdx: fromIdx,
				ToNodeIdx:   toIdx,
				Weight:      e.Weight,
				DistanceM:   e.DistanceM,
				IsCrossCell: true,
			})
		}
	}

	if len(cell.BoundaryNodeIdxs) < 2 {
		return result
	}

	inCell := make([]bool, len(g.Nodes))
	for _, idx := range cell.InternalNodeIdxs {
		inCell[idx] = true
	}

	for _, srcIdx := range cell.BoundaryNodeIdxs {
		dists := cellDijkstra(g, srcIdx, cell.BoundaryNodeIdxs, inCell)
		for _, dstIdx := range cell.BoundaryNodeIdxs {
			if dstIdx == srcIdx {
				continue
			}
			d, reachable := dists[dstIdx]
			if !reachable || d.weight >= math.MaxFloat32 {
				continue
			}
			result = append(result, model.OverlayEdge{
				FromNodeIdx: srcIdx,
				ToNodeIdx:   dstIdx,
				Weight:      d.weight,
				DistanceM:   d.distM,
				IsCrossCell: false,
			})
		}
	}

	return result
}

type distInfo struct {
	weight float32
	distM  float32
}

// cellDijkstra runs Dijkstra inside one cell and returns settled boundary
// distances from the source boundary node.
func cellDijkstra(g *model.Graph, srcIdx uint32, boundaryNodes []uint32, inCell []bool) map[uint32]distInfo {
	dist := make(map[uint32]distInfo)
	dist[srcIdx] = distInfo{0, 0}
	targetSet := make(map[uint32]struct{}, len(boundaryNodes))
	for _, idx := range boundaryNodes {
		targetSet[idx] = struct{}{}
	}

	pq := utilities.NewHeap(func(a, b dijkstraItem) bool { return a.weight < b.weight })
	pq.Push(dijkstraItem{idx: srcIdx, weight: 0})

	settled := 0
	totalBoundary := len(boundaryNodes)

	for pq.Len() > 0 {
		cur := pq.Pop()

		// Skip stale queue entries after a better path has already been recorded.
		best, hasBest := dist[cur.idx]
		if !hasBest || cur.weight > best.weight {
			continue
		}

		if _, isBoundary := targetSet[cur.idx]; isBoundary {
			settled++
			delete(targetSet, cur.idx)
			if settled == totalBoundary {
				break
			}
		}

		for _, eid := range g.BaseAdj.Neighbours(cur.idx) {
			e := &g.Edges[eid]
			toIdx := e.ToNodeIdx
			if !inCell[toIdx] {
				continue
			}
			newW := best.weight + e.Weight
			newD := best.distM + e.DistanceM
			if existing, hasDist := dist[toIdx]; !hasDist || newW < existing.weight {
				dist[toIdx] = distInfo{newW, newD}
				pq.Push(dijkstraItem{idx: toIdx, weight: newW})
			}
		}
	}

	result := make(map[uint32]distInfo, len(boundaryNodes))
	for _, idx := range boundaryNodes {
		if d, ok := dist[idx]; ok {
			result[idx] = d
		}
	}
	return result
}

type dijkstraItem struct {
	idx    uint32
	weight float32
}
