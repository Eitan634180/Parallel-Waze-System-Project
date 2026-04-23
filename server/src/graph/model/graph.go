package model

// Graph contains the base graph, partition metadata, and overlay graph.
type Graph struct {
	Nodes []Node
	Edges []Edge
	Cells []Cell

	Base    BaseGraph
	Overlay OverlayGraph

	BoundaryNodeIdx []int32

	BBox             BoundingBox
	ProjectionRefLat float64
}

// BaseGraph stores outgoing base-graph edges in CSR form.
type BaseGraph struct {
	Offsets []uint32
	EdgeIDs []EdgeID
}

// OverlayGraph stores outgoing overlay edges in CSR form over boundary nodes.
type OverlayGraph struct {
	Offsets      []uint32
	OverlayEdges []OverlayEdge
}

// Neighbours returns all outgoing edges for the internal node index.
func (a *BaseGraph) Neighbours(idx uint32) []EdgeID {
	return a.EdgeIDs[a.Offsets[idx]:a.Offsets[idx+1]]
}

// Neighbours returns all outgoing overlay edges for the boundary-node index.
func (o *OverlayGraph) Neighbours(idx uint32) []OverlayEdge {
	return o.OverlayEdges[o.Offsets[idx]:o.Offsets[idx+1]]
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
