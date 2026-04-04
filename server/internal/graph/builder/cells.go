package builder

import (
	"container/heap"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sort"
	"sync"
)

// =============================================================================
// PART 1 — INERTIAL FLOW RECURSIVE BISECTION
// =============================================================================
// Algorithm (per partition step):
//  1. Project all node (X,Y) coords onto a random unit vector d.
//  2. Sort by projection score; label bottom-25% as Source set S, top-25% as Sink set T.
//  3. Run Dinic's max-flow on edges from S→T (capacity=1 per directed edge).
//  4. The min-cut identifies which nodes belong to the S-side vs T-side partition.
//  5. Recurse on each half until |partition| ≤ maxCellSize.

// PartitionCells runs Inertial Flow recursive bisection on the graph and
// populates g.Cells, g.CellIdx.
func PartitionCells(g *Graph, maxCellSize int) {
	fmt.Printf("[CELLS] Partitioning %d nodes (maxCellSize=%d) …\n", len(g.Nodes), maxCellSize)

	allNodeIdxs := make([]uint32, len(g.Nodes))
	for i := range allNodeIdxs {
		allNodeIdxs[i] = uint32(i)
	}

	cellAssign := make([]CellID, len(g.Nodes))
	var mu sync.Mutex
	var wg sync.WaitGroup

	var bisect func(nodeIdxs []uint32, rng *rand.Rand)
	bisect = func(nodeIdxs []uint32, rng *rand.Rand) {
		defer wg.Done()

		if len(nodeIdxs) <= maxCellSize {
			mu.Lock()
			cid := CellID(len(g.Cells))
			cell := Cell{ID: cid}
			cell.InternalNodeIDs = make([]NodeID, len(nodeIdxs))
			for i, idx := range nodeIdxs {
				cell.InternalNodeIDs[i] = g.Nodes[idx].ID
				cellAssign[idx] = cid
			}
			g.Cells = append(g.Cells, cell)
			mu.Unlock()
			return
		}

		left, right := inertialFlowBisect(g, nodeIdxs, rng)

		wg.Add(2)
		// Run left half in a new goroutine only if it's large enough to matter.
		// For small halves, run inline to avoid goroutine overhead.
		rng2 := rand.New(rand.NewSource(rng.Int63()))
		go bisect(left, rng2)
		go bisect(right, rng)
	}

	rng := rand.New(rand.NewSource(42))
	wg.Add(1)
	bisect(allNodeIdxs, rng)
	wg.Wait()

	// Populate g.CellIdx from cellAssign
	g.CellIdx = make(map[NodeID]CellID, len(g.Nodes))
	for i, node := range g.Nodes {
		cid := cellAssign[i]
		g.Nodes[i].CellID = cid
		g.CellIdx[node.ID] = cid
	}

	fmt.Printf("[CELLS] Partitioned into %d cells\n", len(g.Cells))
}

// inertialFlowBisect partitions nodeIdxs into two halves using Inertial Flow.
func inertialFlowBisect(g *Graph, nodeIdxs []uint32, rng *rand.Rand) (left, right []uint32) {
	n := len(nodeIdxs)

	// 1. Random unit vector d
	angle := rng.Float64() * 2 * math.Pi
	dx, dy := math.Cos(angle), math.Sin(angle)

	// 2. Project nodes onto d and sort
	type scored struct {
		idx   uint32
		score float64
	}
	scores := make([]scored, n)
	for i, idx := range nodeIdxs {
		nd := &g.Nodes[idx]
		scores[i] = scored{idx: idx, score: float64(nd.X)*dx + float64(nd.Y)*dy}
	}
	sort.Slice(scores, func(a, b int) bool { return scores[a].score < scores[b].score })

	// 3. Label source (bottom 25%) and sink (top 25%)
	q := n / 4
	if q < 1 {
		q = 1
	}

	sourceSet := make(map[uint32]bool, q)
	sinkSet := make(map[uint32]bool, q)
	for i := 0; i < q; i++ {
		sourceSet[scores[i].idx] = true
	}
	for i := n - q; i < n; i++ {
		sinkSet[scores[i].idx] = true
	}

	// 4. Dinic's max-flow → min-cut
	// Build a local flow network over the current node subset.
	localIdx := make(map[uint32]int, n) // graph node idx → local flow node idx
	for i, s := range scores {
		localIdx[s.idx] = i
	}

	// Virtual super-source (index n) and super-sink (index n+1)
	S, T := n, n+1
	totalNodes := n + 2

	// Build the flow network (capacity = 1 per base-graph edge within the subset)
	fn := newFlowNet(totalNodes)

	// Super-source → all source nodes, super-sink ← all sink nodes
	for idx := range sourceSet {
		fn.addEdge(S, localIdx[idx], n) // large capacity from super-source
	}
	for idx := range sinkSet {
		fn.addEdge(localIdx[idx], T, n) // large capacity to super-sink
	}

	// Internal edges (within the current subset).
	// The cut objective should be symmetric even when the routing graph is not:
	// a one-way road crossing a partition is still a cross-cell boundary later.
	addUndirectedSubsetEdges(fn, g, nodeIdxs, localIdx)

	// Run Dinic's
	fn.maxflow(S, T)

	// 5. BFS reachability from S in residual graph → assigns side
	reachable := fn.reachableFrom(S)

	// Split into left (S-side) and right (T-side)
	left = make([]uint32, 0, n/2)
	right = make([]uint32, 0, n/2)
	for _, s := range scores {
		if reachable[localIdx[s.idx]] {
			left = append(left, s.idx)
		} else {
			right = append(right, s.idx)
		}
	}

	// Guard: if partitioning produced an empty side (fully connected subgraph),
	// fall back to a simple geometric split to avoid infinite recursion.
	if len(left) == 0 || len(right) == 0 {
		mid := n / 2
		left = make([]uint32, mid)
		right = make([]uint32, n-mid)
		for i, s := range scores[:mid] {
			left[i] = s.idx
		}
		for i, s := range scores[mid:] {
			right[i] = s.idx
		}
	}

	return left, right
}

// addUndirectedSubsetEdges adds one unit-capacity undirected edge for each
// pair of subset nodes connected by at least one base-graph edge.
func addUndirectedSubsetEdges(fn *flowNet, g *Graph, nodeIdxs []uint32, localIdx map[uint32]int) {
	seen := make(map[uint64]struct{}, len(nodeIdxs))
	for _, idx := range nodeIdxs {
		u := localIdx[idx]
		for _, eid := range g.BaseAdj.Neighbours(idx) {
			toID := g.Edges[eid].ToNodeID
			toIdx, ok := g.NodeIdx[toID]
			if !ok {
				continue
			}
			v, inSubset := localIdx[toIdx]
			if !inSubset || u == v {
				continue
			}

			a, b := idx, toIdx
			if a > b {
				a, b = b, a
			}
			key := uint64(a)<<32 | uint64(b)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}

			fn.addEdge(u, v, 1)
			fn.addEdge(v, u, 1)
		}
	}
}

// =============================================================================
// PART 2 — DINIC'S MAX-FLOW (simple implementation for unit-capacity graphs)
// =============================================================================

type flowEdge struct {
	to, rev int
	cap     int
}

type flowNet struct {
	graph [][]flowEdge
	level []int
	iter  []int
}

func newFlowNet(n int) *flowNet {
	return &flowNet{
		graph: make([][]flowEdge, n),
		level: make([]int, n),
		iter:  make([]int, n),
	}
}

func (fn *flowNet) addEdge(u, v, cap int) {
	fn.graph[u] = append(fn.graph[u], flowEdge{v, len(fn.graph[v]), cap})
	fn.graph[v] = append(fn.graph[v], flowEdge{u, len(fn.graph[u]) - 1, 0})
}

func (fn *flowNet) bfs(s int) {
	for i := range fn.level {
		fn.level[i] = -1
	}
	q := []int{s}
	fn.level[s] = 0
	for len(q) > 0 {
		v := q[0]
		q = q[1:]
		for _, e := range fn.graph[v] {
			if e.cap > 0 && fn.level[e.to] < 0 {
				fn.level[e.to] = fn.level[v] + 1
				q = append(q, e.to)
			}
		}
	}
}

func (fn *flowNet) dfs(v, t, f int) int {
	if v == t {
		return f
	}
	for ; fn.iter[v] < len(fn.graph[v]); fn.iter[v]++ {
		e := &fn.graph[v][fn.iter[v]]
		if e.cap > 0 && fn.level[v] < fn.level[e.to] {
			d := fn.dfs(e.to, t, min(f, e.cap))
			if d > 0 {
				e.cap -= d
				fn.graph[e.to][e.rev].cap += d
				return d
			}
		}
	}
	return 0
}

func (fn *flowNet) maxflow(s, t int) int {
	flow := 0
	for {
		fn.bfs(s)
		if fn.level[t] < 0 {
			return flow
		}
		for i := range fn.iter {
			fn.iter[i] = 0
		}
		for {
			f := fn.dfs(s, t, math.MaxInt32)
			if f == 0 {
				break
			}
			flow += f
		}
	}
}

// reachableFrom returns a boolean slice: reachable[i] = true if node i is
// reachable from s in the residual graph (cap > 0 edges only).
func (fn *flowNet) reachableFrom(s int) []bool {
	n := len(fn.graph)
	vis := make([]bool, n)
	q := []int{s}
	vis[s] = true
	for len(q) > 0 {
		v := q[0]
		q = q[1:]
		for _, e := range fn.graph[v] {
			if e.cap > 0 && !vis[e.to] {
				vis[e.to] = true
				q = append(q, e.to)
			}
		}
	}
	return vis
}

// =============================================================================
// PART 3 — BOUNDARY NODE DETECTION
// =============================================================================

// DetectBoundaryNodes scans all edges and marks nodes as boundary nodes if
// they have at least one edge crossing to a different cell.
// Populates Cell.BoundaryNodeIDs, g.BoundaryNodes, g.BoundaryNodeIdx.
func DetectBoundaryNodes(g *Graph) {
	fmt.Println("[CELLS] Detecting boundary nodes …")

	isBoundary := make([]bool, len(g.Nodes))

	for i := range g.Nodes {
		fromCellID := g.Nodes[i].CellID
		for _, eid := range g.BaseAdj.Neighbours(uint32(i)) {
			e := &g.Edges[eid]
			toIdx, ok := g.NodeIdx[e.ToNodeID]
			if !ok {
				continue
			}
			if g.Nodes[toIdx].CellID != fromCellID {
				isBoundary[i] = true     // Mark the leaving node as a boundary
				isBoundary[toIdx] = true // Mark the arriving node as a boundary
			}
		}
	}

	// Build per-cell boundary node lists and global BoundaryNodes slice
	g.BoundaryNodes = make([]NodeID, 0, 32768)
	g.BoundaryNodeIdx = make(map[NodeID]uint32)

	// Temporary map: cellID → indices of boundary nodes added so far
	cellBoundary := make(map[CellID][]NodeID)

	for i, node := range g.Nodes {
		if isBoundary[i] {
			bIdx := uint32(len(g.BoundaryNodes))
			g.BoundaryNodes = append(g.BoundaryNodes, node.ID)
			g.BoundaryNodeIdx[node.ID] = bIdx
			cellBoundary[node.CellID] = append(cellBoundary[node.CellID], node.ID)
		}
	}

	// Assign to cells
	for i := range g.Cells {
		g.Cells[i].BoundaryNodeIDs = cellBoundary[g.Cells[i].ID]
	}

	fmt.Printf("[CELLS] %d boundary nodes detected\n", len(g.BoundaryNodes))
}

// =============================================================================
// PART 4 — OVERLAY GRAPH CONSTRUCTION (PARALLELISED)
// =============================================================================
// For each cell:
//   - Add cross-cell edges (base-graph cut edges, IsCrossCell=true)
//   - Run intra-cell Dijkstra from each boundary node → all other boundary
//     nodes in the same cell → emit shortcut edges (IsCrossCell=false)
//
// All cells are processed concurrently via a worker pool.

// BuildOverlayGraph constructs the overlay adjacency list and stores it in g.OverlayAdj.
func BuildOverlayGraph(g *Graph, numWorkers int) {
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	fmt.Printf("[OVERLAY] Building overlay graph (%d workers) …\n", numWorkers)

	type cellResult struct {
		cellIdx int
		edges   []OverlayEdge
	}

	jobs := make(chan int, len(g.Cells))
	results := make(chan cellResult, len(g.Cells))

	// Launch worker pool
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ci := range jobs {
				edges := computeCellOverlayEdges(g, &g.Cells[ci])
				results <- cellResult{cellIdx: ci, edges: edges}
			}
		}()
	}

	// Enqueue all cells
	for i := range g.Cells {
		jobs <- i
	}
	close(jobs)

	// Close results after all workers done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all overlay edges
	allEdges := make([][]OverlayEdge, len(g.Nodes)) // indexed by boundary node overlay index
	_ = allEdges

	// We'll build a per-boundary-node adjacency, then convert to CSR.
	// adjFrom[overlayIdx] = []OverlayEdge
	adjFrom := make([][]OverlayEdge, len(g.BoundaryNodes))

	totalEdges := 0
	for r := range results {
		_ = r.cellIdx
		for _, e := range r.edges {
			fromIdx, ok := g.BoundaryNodeIdx[e.FromNodeID]
			if !ok {
				continue
			}
			adjFrom[fromIdx] = append(adjFrom[fromIdx], e)
			totalEdges++
		}
	}

	// Build overlay CSR
	offsets := make([]uint32, len(g.BoundaryNodes)+1)
	edges := make([]OverlayEdge, 0, totalEdges)
	for i, nbrs := range adjFrom {
		offsets[i] = uint32(len(edges))
		edges = append(edges, nbrs...)
	}
	offsets[len(g.BoundaryNodes)] = uint32(len(edges))

	g.OverlayAdj = OverlayAdjList{Offsets: offsets, OverlayEdges: edges}
	fmt.Printf("[OVERLAY] Done: %d overlay edges (%d cross-cell + %d shortcuts)\n",
		totalEdges, countCrossCell(edges), totalEdges-countCrossCell(edges))
}

func countCrossCell(edges []OverlayEdge) int {
	n := 0
	for _, e := range edges {
		if e.IsCrossCell {
			n++
		}
	}
	return n
}

// computeCellOverlayEdges computes all overlay edges for one cell:
//  1. Cross-cell edges (cut edges in base graph)
//  2. Intra-cell shortcuts via Dijkstra from each boundary node
func computeCellOverlayEdges(g *Graph, cell *Cell) []OverlayEdge {
	var result []OverlayEdge

	// 1. Cross-cell edges: base-graph edges where from is in this cell and
	//    to is in a DIFFERENT cell, and both endpoints are boundary nodes.
	for _, nid := range cell.InternalNodeIDs {
		fromIdx, ok := g.NodeIdx[nid]
		if !ok {
			continue
		}
		fromCellID := g.Nodes[fromIdx].CellID
		if fromCellID != cell.ID {
			continue
		}
		_, fromIsBoundary := g.BoundaryNodeIdx[nid]
		if !fromIsBoundary {
			continue
		}
		for _, eid := range g.BaseAdj.Neighbours(fromIdx) {
			e := &g.Edges[eid]
			toIdx, ok2 := g.NodeIdx[e.ToNodeID]
			if !ok2 {
				continue
			}
			if g.Nodes[toIdx].CellID == cell.ID {
				continue // same cell — not a cross-cell edge
			}
			_, toIsBoundary := g.BoundaryNodeIdx[e.ToNodeID]
			if !toIsBoundary {
				continue
			}
			result = append(result, OverlayEdge{
				FromNodeID:  nid,
				ToNodeID:    e.ToNodeID,
				Weight:      e.Weight,
				DistanceM:   e.DistanceM,
				IsCrossCell: true,
			})
		}
	}

	// 2. Intra-cell shortcuts: for each boundary node, Dijkstra within cell
	if len(cell.BoundaryNodeIDs) < 2 {
		return result
	}

	// Build a local node-index set for fast membership testing
	inCell := make(map[NodeID]struct{}, len(cell.InternalNodeIDs))
	for _, nid := range cell.InternalNodeIDs {
		inCell[nid] = struct{}{}
	}

	for _, srcID := range cell.BoundaryNodeIDs {
		dists := cellDijkstra(g, srcID, cell.BoundaryNodeIDs, inCell)
		for _, dstID := range cell.BoundaryNodeIDs {
			if dstID == srcID {
				continue
			}
			d, reachable := dists[dstID]
			if !reachable || d.weight >= math.MaxFloat32 {
				continue
			}
			result = append(result, OverlayEdge{
				FromNodeID:  srcID,
				ToNodeID:    dstID,
				Weight:      d.weight,
				DistanceM:   d.distM,
				IsCrossCell: false,
			})
		}
	}

	return result
}

// =============================================================================
// PART 5 — INTRA-CELL DIJKSTRA
// =============================================================================

type distInfo struct {
	weight float32
	distM  float32
}

// cellDijkstra runs Dijkstra from srcID within the nodes of the given cell,
// stopping when all boundary nodes have been settled.  Returns a map of
// boundary node ID → best (weight, distM).
func cellDijkstra(g *Graph, srcID NodeID, boundaryNodes []NodeID, inCell map[NodeID]struct{}) map[NodeID]distInfo {
	const inf = float32(math.MaxFloat32)

	dist := make(map[NodeID]distInfo)
	dist[srcID] = distInfo{0, 0}
	targetSet := make(map[NodeID]struct{}, len(boundaryNodes))
	for _, nid := range boundaryNodes {
		targetSet[nid] = struct{}{}
	}

	pq := &dijkstraPQ{}
	heap.Push(pq, dijkstraItem{id: srcID, weight: 0})

	settled := 0
	totalBoundary := len(boundaryNodes)

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(dijkstraItem)
		curID := cur.id

		// The queue may contain multiple entries for the same node with different costs.
		// If the popped cost is worse than our recorded best, it's an old entry and we can skip it.
		best, hasBest := dist[curID]
		if !hasBest || cur.weight > best.weight {
			continue
		}

		// Check if this is a boundary node (count settled boundary nodes)
		if _, isBoundary := targetSet[curID]; isBoundary {
			settled++
			delete(targetSet, curID)
			if settled == totalBoundary {
				break // all boundary nodes settled — done
			}
		}

		curIdx, ok := g.NodeIdx[curID]
		if !ok {
			continue
		}

		for _, eid := range g.BaseAdj.Neighbours(curIdx) {
			e := &g.Edges[eid]
			toID := e.ToNodeID

			// Stay within the cell
			if _, inC := inCell[toID]; !inC {
				continue
			}

			newW := best.weight + e.Weight
			newD := best.distM + e.DistanceM

			if existing, hasDist := dist[toID]; !hasDist || newW < existing.weight {
				dist[toID] = distInfo{newW, newD}
				heap.Push(pq, dijkstraItem{id: toID, weight: newW})
			}
		}
	}

	// Return only boundary node distances
	result := make(map[NodeID]distInfo, len(boundaryNodes))
	for _, bid := range boundaryNodes {
		if d, ok := dist[bid]; ok {
			result[bid] = d
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Min-heap for Dijkstra
// ---------------------------------------------------------------------------

type dijkstraItem struct {
	id     NodeID
	weight float32
}

type dijkstraPQ []dijkstraItem

func (pq dijkstraPQ) Len() int            { return len(pq) }
func (pq dijkstraPQ) Less(i, j int) bool  { return pq[i].weight < pq[j].weight }
func (pq dijkstraPQ) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *dijkstraPQ) Push(x interface{}) { *pq = append(*pq, x.(dijkstraItem)) }
func (pq *dijkstraPQ) Pop() interface{} {
	old := *pq
	n := len(old)
	x := old[n-1]
	*pq = old[:n-1]
	return x
}
