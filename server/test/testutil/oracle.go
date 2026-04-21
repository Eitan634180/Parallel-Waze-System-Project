package testutil

import (
	"container/heap"
	"math"

	"nav-system/src/graph/model"
	"nav-system/src/routing"
	"nav-system/src/utilities"
)

type OraclePath struct {
	NodeIdxs []uint32
	EdgeIDs  []model.EdgeID
	Cost     float32
	Distance float32
}

type oracleItem struct {
	nodeIdx uint32
	cost    float32
	index   int
}

type oracleHeap []*oracleItem

func (h oracleHeap) Len() int           { return len(h) }
func (h oracleHeap) Less(i, j int) bool { return h[i].cost < h[j].cost }
func (h oracleHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *oracleHeap) Push(x any) {
	item := x.(*oracleItem)
	item.index = len(*h)
	*h = append(*h, item)
}
func (h *oracleHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return item
}

func BruteForceSnap(g *model.Graph, lat, lon float64) uint32 {
	if len(g.Nodes) == 0 {
		return 0
	}

	qx, qy := utilities.ProjectAtReferenceLat(lat, lon, g.ProjectionRefLat)
	bestIdx := uint32(0)
	bestDist := float32(math.MaxFloat32)

	for idx := range g.Nodes {
		node := &g.Nodes[idx]
		dist := utilities.DistanceSquared(qx, qy, node.X, node.Y)
		if dist < bestDist {
			bestDist = dist
			bestIdx = uint32(idx)
		}
	}

	return bestIdx
}

func ShortestPath(g *model.Graph, srcIdx, dstIdx uint32, wf routing.WeightFunc) (OraclePath, bool) {
	if wf == nil {
		wf = routing.BaseWeight
	}
	if srcIdx == dstIdx {
		return OraclePath{NodeIdxs: []uint32{srcIdx}}, true
	}

	dist := make([]float32, len(g.Nodes))
	prevNode := make([]int, len(g.Nodes))
	prevEdge := make([]int, len(g.Nodes))
	for i := range dist {
		dist[i] = float32(math.MaxFloat32)
		prevNode[i] = -1
		prevEdge[i] = -1
	}

	pq := oracleHeap{}
	heap.Init(&pq)
	dist[srcIdx] = 0
	heap.Push(&pq, &oracleItem{nodeIdx: srcIdx, cost: 0})

	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*oracleItem)
		if item.cost > dist[item.nodeIdx] {
			continue
		}
		if item.nodeIdx == dstIdx {
			break
		}

		for _, edgeID := range g.BaseAdj.Neighbours(item.nodeIdx) {
			edge := &g.Edges[edgeID]
			nextIdx := edge.ToNodeIdx
			nextCost := item.cost + wf(edge)
			if nextCost < dist[nextIdx] {
				dist[nextIdx] = nextCost
				prevNode[nextIdx] = int(item.nodeIdx)
				prevEdge[nextIdx] = int(edgeID)
				heap.Push(&pq, &oracleItem{nodeIdx: nextIdx, cost: nextCost})
			}
		}
	}

	if dist[dstIdx] == float32(math.MaxFloat32) {
		return OraclePath{}, false
	}

	nodeIdxs := []uint32{dstIdx}
	edgeIDs := make([]model.EdgeID, 0, len(g.Nodes))
	var totalDistance float32

	for current := int(dstIdx); current != int(srcIdx); current = prevNode[current] {
		if current < 0 || prevNode[current] < 0 || prevEdge[current] < 0 {
			return OraclePath{}, false
		}
		edgeID := model.EdgeID(prevEdge[current])
		edgeIDs = append(edgeIDs, edgeID)
		totalDistance += g.Edges[edgeID].DistanceM
		nodeIdxs = append(nodeIdxs, uint32(prevNode[current]))
	}

	reverseUint32s(nodeIdxs)
	reverseEdgeIDs(edgeIDs)

	return OraclePath{
		NodeIdxs: nodeIdxs,
		EdgeIDs:  edgeIDs,
		Cost:     dist[dstIdx],
		Distance: totalDistance,
	}, true
}

func reverseUint32s(values []uint32) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func reverseEdgeIDs(values []model.EdgeID) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}
