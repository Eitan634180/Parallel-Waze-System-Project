package routing

import "nav-system/internal/graph/builder"

// ijItem is used for plain Dijkstra (no heuristic).
type ijItem struct {
	id   builder.NodeID
	cost float32
}

type ijPQ []ijItem

func (pq ijPQ) Len() int            { return len(pq) }
func (pq ijPQ) Less(i, j int) bool  { return pq[i].cost < pq[j].cost }
func (pq ijPQ) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *ijPQ) Push(x interface{}) { *pq = append(*pq, x.(ijItem)) }
func (pq *ijPQ) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}

// astarItem carries both the f-score (g+h) and g-score.
type astarItem struct {
	id builder.NodeID
	f  float32
	g  float32
}

type astarPQ []astarItem

func (pq astarPQ) Len() int            { return len(pq) }
func (pq astarPQ) Less(i, j int) bool  { return pq[i].f < pq[j].f }
func (pq astarPQ) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *astarPQ) Push(x interface{}) { *pq = append(*pq, x.(astarItem)) }
func (pq *astarPQ) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}

type localAstarItem struct {
	id   builder.NodeID
	f    float32
	g    float32
	hops int
}

type localAstarPQ []localAstarItem

func (pq localAstarPQ) Len() int            { return len(pq) }
func (pq localAstarPQ) Less(i, j int) bool  { return pq[i].f < pq[j].f }
func (pq localAstarPQ) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *localAstarPQ) Push(x interface{}) { *pq = append(*pq, x.(localAstarItem)) }
func (pq *localAstarPQ) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}
