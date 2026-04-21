package model

import "sync"

// Graph contains the base graph, partition metadata, and overlay graph.
type Graph struct {
	Nodes   []Node
	Edges   []Edge
	BaseAdj AdjacencyList
	Cells   []Cell

	BoundaryNodeIdx  []int32
	BoundaryBaseIdxs []uint32

	OverlayAdj OverlayAdjList

	BBox             BoundingBox
	ProjectionRefLat float64
}

// AdjacencyList stores outgoing base-graph edges in CSR form.
type AdjacencyList struct {
	Offsets []uint32
	EdgeIDs []EdgeID
}

// OverlayAdjList stores outgoing overlay edges in CSR form over boundary nodes.
type OverlayAdjList struct {
	Mu           *sync.RWMutex
	Offsets      []uint32
	OverlayEdges []OverlayEdge
}

// Neighbours returns all outgoing edges for the internal node index.
func (a *AdjacencyList) Neighbours(idx uint32) []EdgeID {
	return a.EdgeIDs[a.Offsets[idx]:a.Offsets[idx+1]]
}

// Neighbours returns all outgoing overlay edges for the boundary-node index.
func (o *OverlayAdjList) Neighbours(idx uint32) []OverlayEdge {
	return o.OverlayEdges[o.Offsets[idx]:o.Offsets[idx+1]]
}

// BuildNodeIdxMap returns a map from OSM NodeID to internal node index.
func (g *Graph) BuildNodeIdxMap() map[NodeRawID]uint32 {
	m := make(map[NodeRawID]uint32, len(g.Nodes))
	for i, n := range g.Nodes {
		m[n.ID] = uint32(i)
	}
	return m
}
