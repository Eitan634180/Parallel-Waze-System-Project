package builder

import (
	"log"
	"math"
	"math/rand"
	"runtime"
	"slices"
	"sync"

	"nav-system/src/graph"
	"nav-system/src/graph/model"
	"nav-system/src/utilities"
)

const cellBuilderLogPrefix = "cell-builder:"

// PartitionCells recursively bisects the graph with Inertial Flow and fills g.Cells.
func PartitionCells(g *model.Graph, maxCellSize int) {
	log.Printf("%s partitioning %d nodes (max size %d)", cellBuilderLogPrefix, len(g.Nodes), maxCellSize)

	allNodeIdxs := make([]uint32, len(g.Nodes))
	for i := range allNodeIdxs {
		allNodeIdxs[i] = uint32(i)
	}

	cellAssign := make([]model.CellID, len(g.Nodes))
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
			cid := model.CellID(len(g.Cells))
			for _, idx := range nodeIdxs {
				cellAssign[idx] = cid
			}
			g.Cells = append(g.Cells, model.Cell{})
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

	rng := rand.New(rand.NewSource(graph.PartitionSeed))
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
func inertialFlowBisect(g *model.Graph, nodeIdxs []uint32, rng *rand.Rand, pool *sync.Pool) (left, right []uint32) {
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

	q := n / graph.InertialFlowQuartileDiv
	if q < graph.MinInertialFlowQuartile {
		q = graph.MinInertialFlowQuartile
	}

	localIdxPtr := pool.Get().(*[]int)
	localIdx := *localIdxPtr
	for i, s := range scores {
		localIdx[s.idx] = i
	}

	s, t := n, n+graph.FlowSinkNodeOffset
	fn := utilities.NewFlowNet(n + graph.FlowTerminalNodeCount)

	for i := 0; i < q; i++ {
		fn.AddEdge(s, localIdx[scores[i].idx], n)
	}
	for i := n - q; i < n; i++ {
		fn.AddEdge(localIdx[scores[i].idx], t, n)
	}

	// Keep the cut symmetric even if the routing graph contains one-way roads.
	addUndirectedSubsetEdges(fn, g, nodeIdxs, localIdx)
	fn.MaxFlow(s, t)

	reachable := fn.ReachableFrom(s)
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
func addUndirectedSubsetEdges(fn *utilities.FlowNet, g *model.Graph, nodeIdxs []uint32, localIdx []int) {
	seen := make([]uint64, 0, len(nodeIdxs)*2)
	for _, idx := range nodeIdxs {
		u := localIdx[idx]
		start, end := g.Base.EdgeRange(idx)
		for eid := start; eid < end; eid++ {
			toIdx := g.Edges[eid].ToNode
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
	fn.AddEdge(int(prev>>32), int(uint32(prev)), graph.UnitFlowCapacity)
	fn.AddEdge(int(uint32(prev)), int(prev>>32), graph.UnitFlowCapacity)

	for i := 1; i < len(seen); i++ {
		curr := seen[i]
		if curr != prev {
			fn.AddEdge(int(curr>>32), int(uint32(curr)), graph.UnitFlowCapacity)
			fn.AddEdge(int(uint32(curr)), int(curr>>32), graph.UnitFlowCapacity)
			prev = curr
		}
	}
}
