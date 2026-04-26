package model

// Graph contains the base graph, partition metadata, and overlay graph.
type Graph struct {
	Nodes      []Node
	Edges      []Edge
	Cells      []Cell
	NodeToGate []int32

	Base    BaseGraph
	Overlay OverlayGraph

	BBox             BoundingBox
	ProjectionRefLat float64
}

// BaseGraph stores outgoing base-graph edges in CSR form.
type BaseGraph struct {
	Offsets []uint32
}

func (a *BaseGraph) EdgeRange(idx uint32) (uint32, uint32) {
	return a.Offsets[idx], a.Offsets[idx+1]
}

// OverlayGraph stores outgoing overlay edges in CSR form over gate nodes.
type OverlayGraph struct {
	Offsets      []uint32
	OverlayEdges []OverlayEdge
}

func (g *Graph) Edge(id EdgeID) (*Edge, bool) {
	if int(id) < 0 || int(id) >= len(g.Edges) {
		return nil, false
	}
	return &g.Edges[id], true
}

func (g *Graph) Node(idx uint32) *Node {
	if idx >= uint32(len(g.Nodes)) {
		return nil
	}
	return &g.Nodes[idx]
}
