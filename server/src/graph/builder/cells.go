package builder

import (
	"log"
	"math"
	"math/rand"
	"runtime"
	"slices"
	"sync"

	"nav-system/src/utilities"
)

const cellBuilderLogPrefix = "cell-builder:"

const (
	minInertialFlowQuartile  = 1
	partitionSeed            = 42
	inertialFlowQuartileDiv  = 4
	flowTerminalNodeCount    = 2
	flowSinkNodeOffset       = 1
	unitFlowCapacity         = 1
	boundaryNodeCapacityHint = 32768
)

// PartitionCells recursively bisects the graph with Inertial Flow and fills
// g.Cells.
func PartitionCells(g *Graph, maxCellSize int) {
	log.Printf("%s partitioning %d nodes (max size %d)", cellBuilderLogPrefix, len(g.Nodes), maxCellSize)

	allNodeIdxs := make([]uint32, len(g.Nodes))
	for i := range allNodeIdxs {
		allNodeIdxs[i] = uint32(i)
	}

	cellAssign := make([]CellID, len(g.Nodes))
	var mu sync.Mutex
	var wg sync.WaitGroup

	sem := make(chan struct{}, max(runtime.GOMAXPROCS(0), 1))

	localIdxPool := &sync.Pool{
		New: func() interface{} {
			s := make([]int, len(g.Nodes))
			for i := range s {
				s[i] = -1
			}
			return &s
		},
	}

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

		left, right := inertialFlowBisect(g, nodeIdxs, rng, localIdxPool)

		wg.Add(2)
		rng2 := rand.New(rand.NewSource(rng.Int63()))

		spawn := func(child []uint32, childRNG *rand.Rand) {
			select {
			case sem <- struct{}{}:
				go func() {
					defer func() { <-sem }()
					bisect(child, childRNG)
				}()
			default:
				bisect(child, childRNG)
			}
		}

		spawn(left, rng2)
		spawn(right, rng)
	}

	rng := rand.New(rand.NewSource(partitionSeed))
	wg.Add(1)
	bisect(allNodeIdxs, rng)
	wg.Wait()

	for i := range g.Nodes {
		cid := cellAssign[i]
		g.Nodes[i].CellID = cid
	}

	log.Printf("%s partitioned graph into %d cells", cellBuilderLogPrefix, len(g.Cells))
}

// inertialFlowBisect partitions a node subset into two halves with one
// inertial-flow step.
func inertialFlowBisect(g *Graph, nodeIdxs []uint32, rng *rand.Rand, pool *sync.Pool) (left, right []uint32) {
	n := len(nodeIdxs)

	angle := rng.Float64() * 2 * math.Pi
	dx, dy := math.Cos(angle), math.Sin(angle)

	type scored struct {
		idx   uint32
		score float64
	}
	scores := make([]scored, n)
	for i, idx := range nodeIdxs {
		nd := &g.Nodes[idx]
		scores[i] = scored{idx: idx, score: float64(nd.X)*dx + float64(nd.Y)*dy}
	}
	slices.SortFunc(scores, func(a, b scored) int {
		if a.score < b.score {
			return -1
		}
		if a.score > b.score {
			return 1
		}
		return 0
	})

	q := n / inertialFlowQuartileDiv
	if q < minInertialFlowQuartile {
		q = minInertialFlowQuartile
	}

	localIdxPtr := pool.Get().(*[]int)
	localIdx := *localIdxPtr
	for i, s := range scores {
		localIdx[s.idx] = i
	}

	s, t := n, n+flowSinkNodeOffset
	fn := newFlowNet(n + flowTerminalNodeCount)

	for i := 0; i < q; i++ {
		fn.addEdge(s, localIdx[scores[i].idx], n)
	}
	for i := n - q; i < n; i++ {
		fn.addEdge(localIdx[scores[i].idx], t, n)
	}

	// Keep the cut symmetric even if the routing graph contains one-way roads.
	addUndirectedSubsetEdges(fn, g, nodeIdxs, localIdx)
	fn.maxflow(s, t)

	reachable := fn.reachableFrom(s)
	left = make([]uint32, 0, n/2)
	right = make([]uint32, 0, n/2)
	for _, node := range scores {
		if reachable[localIdx[node.idx]] {
			left = append(left, node.idx)
		} else {
			right = append(right, node.idx)
		}
	}

	// Fall back to a geometric split if the cut collapses to one side.
	if len(left) == 0 || len(right) == 0 {
		mid := n / 2
		left = make([]uint32, mid)
		right = make([]uint32, n-mid)
		for i, node := range scores[:mid] {
			left[i] = node.idx
		}
		for i, node := range scores[mid:] {
			right[i] = node.idx
		}
	}

	for _, node := range scores {
		localIdx[node.idx] = -1
	}
	pool.Put(localIdxPtr)

	return left, right
}

// addUndirectedSubsetEdges adds one unit-capacity undirected edge for each
// pair of subset nodes connected by at least one base-graph edge.
func addUndirectedSubsetEdges(fn *flowNet, g *Graph, nodeIdxs []uint32, localIdx []int) {
	seen := make([]uint64, 0, len(nodeIdxs)*2)
	for _, idx := range nodeIdxs {
		u := localIdx[idx]
		for _, eid := range g.BaseAdj.Neighbours(idx) {
			toIdx := g.Edges[eid].ToNodeIdx
			v := localIdx[toIdx]
			if v == -1 || u == v {
				continue
			}

			a, b := u, v
			if a > b {
				a, b = b, a
			}
			seen = append(seen, uint64(uint32(a))<<32|uint64(uint32(b)))
		}
	}

	if len(seen) == 0 {
		return
	}

	slices.Sort(seen)

	prev := seen[0]
	fn.addEdge(int(prev>>32), int(uint32(prev)), unitFlowCapacity)
	fn.addEdge(int(uint32(prev)), int(prev>>32), unitFlowCapacity)

	for i := 1; i < len(seen); i++ {
		curr := seen[i]
		if curr != prev {
			fn.addEdge(int(curr>>32), int(uint32(curr)), unitFlowCapacity)
			fn.addEdge(int(uint32(curr)), int(curr>>32), unitFlowCapacity)
			prev = curr
		}
	}
}

type flowEdge struct {
	to, rev int
	cap     int
}

type flowNet struct {
	graph [][]flowEdge
	level []int
	iter  []int
	q     []int
}

func newFlowNet(n int) *flowNet {
	fn := &flowNet{
		graph: make([][]flowEdge, n),
		level: make([]int, n),
		iter:  make([]int, n),
		q:     make([]int, 0, n),
	}
	for i := range fn.level {
		fn.level[i] = -1
	}
	return fn
}

func (fn *flowNet) addEdge(u, v, cap int) {
	fn.graph[u] = append(fn.graph[u], flowEdge{v, len(fn.graph[v]), cap})
	fn.graph[v] = append(fn.graph[v], flowEdge{u, len(fn.graph[u]) - 1, 0})
}

func (fn *flowNet) bfs(s int) {
	for _, v := range fn.q {
		fn.level[v] = -1
		fn.iter[v] = 0
	}

	fn.q = fn.q[:0]
	fn.q = append(fn.q, s)
	fn.level[s] = 0

	for head := 0; head < len(fn.q); head++ {
		v := fn.q[head]
		for _, e := range fn.graph[v] {
			if e.cap > 0 && fn.level[e.to] < 0 {
				fn.level[e.to] = fn.level[v] + 1
				fn.q = append(fn.q, e.to)
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

		for {
			f := fn.dfs(s, t, math.MaxInt32)
			if f == 0 {
				break
			}
			flow += f
		}
	}
}

// reachableFrom reports which residual-network nodes remain reachable from s.
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

// DetectBoundaryNodes marks nodes that touch edges crossing a cell boundary.
func DetectBoundaryNodes(g *Graph) {
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

	g.BoundaryNodes = make([]NodeID, 0, boundaryNodeCapacityHint)
	g.BoundaryNodeIdx = make(map[NodeID]uint32)
	cellBoundary := make(map[CellID][]NodeID)

	for i, node := range g.Nodes {
		if !isBoundary[i] {
			continue
		}
		bIdx := uint32(len(g.BoundaryNodes))
		g.BoundaryNodes = append(g.BoundaryNodes, node.ID)
		g.BoundaryNodeIdx[node.ID] = bIdx
		cellBoundary[node.CellID] = append(cellBoundary[node.CellID], node.ID)
	}

	for i := range g.Cells {
		g.Cells[i].BoundaryNodeIDs = cellBoundary[g.Cells[i].ID]
	}

	log.Printf("%s detected %d boundary nodes", cellBuilderLogPrefix, len(g.BoundaryNodes))
}

// BuildOverlayGraph constructs the overlay adjacency list for all boundary
// nodes. Each cell contributes cross-cell edges and intra-cell shortcuts.
func BuildOverlayGraph(g *Graph, numWorkers int) {
	if numWorkers <= 0 {
		numWorkers = max(runtime.GOMAXPROCS(0), 1)
	}
	log.Printf("%s building overlay graph with %d workers", cellBuilderLogPrefix, numWorkers)

	type cellResult struct {
		edges []OverlayEdge
	}

	jobs := make(chan int, len(g.Cells))
	results := make(chan cellResult, len(g.Cells))

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
		close(results)
	}()

	adjFrom := make([][]OverlayEdge, len(g.BoundaryNodes))
	totalEdges := 0
	for result := range results {
		for _, edge := range result.edges {
			fromIdx, ok := g.BoundaryNodeIdx[edge.FromNodeID]
			if !ok {
				continue
			}
			adjFrom[fromIdx] = append(adjFrom[fromIdx], edge)
			totalEdges++
		}
	}

	offsets := make([]uint32, len(g.BoundaryNodes)+1)
	edges := make([]OverlayEdge, 0, totalEdges)
	for i, neighbours := range adjFrom {
		offsets[i] = uint32(len(edges))
		edges = append(edges, neighbours...)
	}
	offsets[len(g.BoundaryNodes)] = uint32(len(edges))

	g.OverlayAdj = OverlayAdjList{Mu: &sync.RWMutex{}, Offsets: offsets, OverlayEdges: edges}
	log.Printf("%s overlay graph ready (%d edges, %d cross-cell, %d shortcuts)",
		cellBuilderLogPrefix,
		totalEdges,
		countCrossCell(edges),
		totalEdges-countCrossCell(edges),
	)
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

// computeCellOverlayEdges emits cross-cell edges and intra-cell shortcuts for
// one cell.
func computeCellOverlayEdges(g *Graph, cell *Cell) []OverlayEdge {
	var result []OverlayEdge

	for _, nid := range cell.InternalNodeIDs {
		fromIdx, ok := g.NodeIdx[nid]
		if !ok {
			continue
		}
		if g.Nodes[fromIdx].CellID != cell.ID {
			continue
		}
		if _, fromIsBoundary := g.BoundaryNodeIdx[nid]; !fromIsBoundary {
			continue
		}

		for _, eid := range g.BaseAdj.Neighbours(fromIdx) {
			e := &g.Edges[eid]
			toIdx := e.ToNodeIdx

			if g.Nodes[toIdx].CellID == cell.ID {
				continue
			}
			if _, toIsBoundary := g.BoundaryNodeIdx[e.ToNodeID]; !toIsBoundary {
				continue
			}
			result = append(result, OverlayEdge{
				FromNodeID:  nid,
				ToNodeID:    e.ToNodeID,
				ToNodeIdx:   toIdx,
				Weight:      e.Weight,
				DistanceM:   e.DistanceM,
				IsCrossCell: true,
			})
		}
	}

	if len(cell.BoundaryNodeIDs) < 2 {
		return result
	}

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
			dstIdx, ok := g.NodeIdx[dstID]
			if !ok {
				continue
			}
			result = append(result, OverlayEdge{
				FromNodeID:  srcID,
				ToNodeID:    dstID,
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
func cellDijkstra(g *Graph, srcID NodeID, boundaryNodes []NodeID, inCell map[NodeID]struct{}) map[NodeID]distInfo {
	const inf = float32(math.MaxFloat32)

	dist := make(map[NodeID]distInfo)
	dist[srcID] = distInfo{0, 0}
	targetSet := make(map[NodeID]struct{}, len(boundaryNodes))
	for _, nid := range boundaryNodes {
		targetSet[nid] = struct{}{}
	}

	pq := utilities.NewHeap(func(a, b dijkstraItem) bool { return a.weight < b.weight })
	pq.Push(dijkstraItem{id: srcID, weight: 0})

	settled := 0
	totalBoundary := len(boundaryNodes)

	for pq.Len() > 0 {
		cur := pq.Pop()
		curID := cur.id

		// Skip stale queue entries after a better path has already been recorded.
		best, hasBest := dist[curID]
		if !hasBest || cur.weight > best.weight {
			continue
		}

		if _, isBoundary := targetSet[curID]; isBoundary {
			settled++
			delete(targetSet, curID)
			if settled == totalBoundary {
				break
			}
		}

		curIdx, ok := g.NodeIdx[curID]
		if !ok {
			continue
		}

		for _, eid := range g.BaseAdj.Neighbours(curIdx) {
			e := &g.Edges[eid]
			toID := e.ToNodeID
			if _, ok := inCell[toID]; !ok {
				continue
			}

			newW := best.weight + e.Weight
			newD := best.distM + e.DistanceM
			if existing, hasDist := dist[toID]; !hasDist || newW < existing.weight {
				dist[toID] = distInfo{newW, newD}
				pq.Push(dijkstraItem{id: toID, weight: newW})
			}
		}
	}

	result := make(map[NodeID]distInfo, len(boundaryNodes))
	for _, bid := range boundaryNodes {
		if d, ok := dist[bid]; ok {
			result[bid] = d
		}
	}
	return result
}

type dijkstraItem struct {
	id     NodeID
	weight float32
}
