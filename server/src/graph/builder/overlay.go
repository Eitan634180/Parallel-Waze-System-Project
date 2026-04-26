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

// BuildOverlayGraph constructs the overlay adjacency list for all gate
// nodes. Each cell contributes cross-cell edges and intra-cell shortcuts.
func BuildOverlayGraph(g *model.Graph, numWorkers int) time.Duration {
	if numWorkers <= 0 {
		numWorkers = max(runtime.GOMAXPROCS(0), 1)
	}
	log.Printf("%s building overlay graph with %d workers", cellBuilderLogPrefix, numWorkers)
	cellNodes := groupNodesByCell(g)

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
				results <- cellResult{edges: computeCellOverlayEdges(g, model.CellID(ci), &g.Cells[ci], cellNodes[ci])}
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

	gateCount := 0
	for _, bIdx := range g.NodeToGate {
		if bIdx != -1 {
			gateCount++
		}
	}

	adjFrom := make([][]model.OverlayEdge, gateCount)
	totalEdges := 0
	for result := range results {
		for _, edge := range result.edges {
			fromIdx := g.NodeToGate[edge.FromNode]
			if fromIdx == -1 {
				continue
			}
			adjFrom[fromIdx] = append(adjFrom[fromIdx], edge)
			totalEdges++
		}
	}

	offsets := make([]uint32, gateCount+1)
	edges := make([]model.OverlayEdge, 0, totalEdges)
	for i, neighbours := range adjFrom {
		offsets[i] = uint32(len(edges))
		edges = append(edges, neighbours...)
	}
	offsets[gateCount] = uint32(len(edges))

	shortcutIdx := uint32(0)
	for i := range edges {
		if !edges[i].IsCrossCell {
			edges[i].StoreIdx = shortcutIdx
			shortcutIdx++
		}
	}

	g.Overlay = model.OverlayGraph{Offsets: offsets, OverlayEdges: edges}
	log.Printf("%s overlay graph ready (%d edges, %d cross-cell, %d shortcuts)",
		cellBuilderLogPrefix,
		totalEdges,
		countCrossCell(g, edges),
		totalEdges-countCrossCell(g, edges),
	)

	return elapsed
}

func groupNodesByCell(g *model.Graph) [][]uint32 {
	nodesByCell := make([][]uint32, len(g.Cells))
	for idx := range g.Nodes {
		cellID := g.Nodes[idx].CellID
		if int(cellID) >= len(nodesByCell) {
			continue
		}
		nodesByCell[cellID] = append(nodesByCell[cellID], uint32(idx))
	}
	return nodesByCell
}

// DetectGateNodes marks nodes that touch edges crossing a cell gate.
func DetectGateNodes(g *model.Graph) {
	log.Printf("%s detecting gate nodes", cellBuilderLogPrefix)

	isGate := make([]bool, len(g.Nodes))
	for i := range g.Nodes {
		fromCellID := g.Nodes[i].CellID
		for _, eid := range g.Base.Neighbours(uint32(i)) {
			e := &g.Edges[eid]
			toIdx := e.ToNode
			if g.Nodes[toIdx].CellID != fromCellID {
				isGate[i] = true
				isGate[toIdx] = true
			}
		}
	}

	g.NodeToGate = make([]int32, len(g.Nodes))
	for i := range g.NodeToGate {
		g.NodeToGate[i] = -1
	}
	cellGates := make(map[model.CellID][]uint32)

	gateCount := 0
	for i, node := range g.Nodes {
		if !isGate[i] {
			continue
		}
		g.NodeToGate[i] = int32(gateCount)
		gateCount++
		cellGates[node.CellID] = append(cellGates[node.CellID], uint32(i))
	}

	for i := range g.Cells {
		g.Cells[i].GateNodes = cellGates[model.CellID(i)]
	}

	log.Printf("%s detected %d gate nodes", cellBuilderLogPrefix, gateCount)
}

func countCrossCell(g *model.Graph, edges []model.OverlayEdge) int {
	n := 0
	for _, e := range edges {
		if g.Nodes[e.FromNode].CellID != g.Nodes[e.ToNode].CellID {
			n++
		}
	}
	return n
}

// computeCellOverlayEdges emits cross-cell edges and intra-cell shortcuts for one cell.
func computeCellOverlayEdges(g *model.Graph, cellID model.CellID, cell *model.Cell, cellNodeIdxs []uint32) []model.OverlayEdge {
	var result []model.OverlayEdge

	for _, fromIdx := range cell.GateNodes {
		if g.NodeToGate[fromIdx] == -1 {
			continue
		}

		for _, eid := range g.Base.Neighbours(fromIdx) {
			e := &g.Edges[eid]
			toIdx := e.ToNode

			if g.Nodes[toIdx].CellID == cellID {
				continue
			}
			if g.NodeToGate[toIdx] == -1 {
				continue
			}
			result = append(result, model.OverlayEdge{
				FromNode:    fromIdx,
				ToNode:      toIdx,
				BaseWeight:  e.BaseWeight,
				StoreIdx:    eid,
				IsCrossCell: true,
			})
		}
	}

	if len(cell.GateNodes) < 2 {
		return result
	}

	inCell := make([]bool, len(g.Nodes))
	for _, idx := range cellNodeIdxs {
		inCell[idx] = true
	}

	for _, srcIdx := range cell.GateNodes {
		dists := cellDijkstra(g, srcIdx, cell.GateNodes, inCell)
		for _, dstIdx := range cell.GateNodes {
			if dstIdx == srcIdx {
				continue
			}
			d, reachable := dists[dstIdx]
			if !reachable || d.weight >= math.MaxFloat32 {
				continue
			}
			result = append(result, model.OverlayEdge{
				FromNode:   srcIdx,
				ToNode:     dstIdx,
				BaseWeight: d.weight,
			})
		}
	}

	return result
}

type distInfo struct {
	weight float32
}

// cellDijkstra runs Dijkstra inside one cell and returns settled gate
// distances from the source gate node.
func cellDijkstra(g *model.Graph, srcIdx uint32, GateNodes []uint32, inCell []bool) map[uint32]distInfo {
	dist := make(map[uint32]distInfo)
	dist[srcIdx] = distInfo{0}
	targetSet := make(map[uint32]struct{}, len(GateNodes))
	for _, idx := range GateNodes {
		targetSet[idx] = struct{}{}
	}

	pq := utilities.NewHeap(func(a, b dijkstraItem) bool { return a.weight < b.weight })
	pq.Push(dijkstraItem{idx: srcIdx, weight: 0})

	settled := 0
	totalGates := len(GateNodes)

	for pq.Len() > 0 {
		cur := pq.Pop()

		// Skip stale queue entries after a better path has already been recorded.
		best, hasBest := dist[cur.idx]
		if !hasBest || cur.weight > best.weight {
			continue
		}

		if _, isGate := targetSet[cur.idx]; isGate {
			settled++
			delete(targetSet, cur.idx)
			if settled == totalGates {
				break
			}
		}

		for _, eid := range g.Base.Neighbours(cur.idx) {
			e := &g.Edges[eid]
			toIdx := e.ToNode
			if !inCell[toIdx] {
				continue
			}
			newW := best.weight + e.BaseWeight
			if existing, hasDist := dist[toIdx]; !hasDist || newW < existing.weight {
				dist[toIdx] = distInfo{newW}
				pq.Push(dijkstraItem{idx: toIdx, weight: newW})
			}
		}
	}

	result := make(map[uint32]distInfo, len(GateNodes))
	for _, idx := range GateNodes {
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
