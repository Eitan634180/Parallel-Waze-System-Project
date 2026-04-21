package geometry

import "nav-system/src/graph/model"

func EdgeByID(g *model.Graph, edgeID model.EdgeID) (*model.Edge, bool) {
	if int(edgeID) < 0 || int(edgeID) >= len(g.Edges) {
		return nil, false
	}
	return &g.Edges[edgeID], true
}

func NodeByIndex(g *model.Graph, idx uint32) *model.Node {
	if idx >= uint32(len(g.Nodes)) {
		return nil
	}
	return &g.Nodes[idx]
}
