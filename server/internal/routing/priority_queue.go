package routing

import "nav-system/internal/graph/builder"

// ijItem is used for plain Dijkstra (no heuristic).
type ijItem struct {
	id   builder.NodeID
	cost float32
}

// astarItem carries both the f-score (g+h) and g-score.
type astarItem struct {
	id builder.NodeID
	f  float32
	g  float32
}

type localAstarItem struct {
	id   builder.NodeID
	f    float32
	g    float32
	hops int
}
