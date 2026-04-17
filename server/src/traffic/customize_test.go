package traffic

import (
	"sync"
	"testing"

	"nav-system/src/graph/builder"
)

func TestBuildCustomizationIndexMapsOnlyCrossCellOverlayEdges(t *testing.T) {
	g := &builder.Graph{
		Nodes: []builder.Node{
			{ID: 1},
			{ID: 2},
			{ID: 3},
		},
		NodeIdx: map[builder.NodeID]uint32{
			1: 0,
			2: 1,
			3: 2,
		},
		Edges: []builder.Edge{
			{ID: 0, FromNodeID: 1, ToNodeID: 2, ToNodeIdx: 1},
			{ID: 1, FromNodeID: 2, ToNodeID: 3, ToNodeIdx: 2},
		},
		BaseAdj: builder.AdjacencyList{
			Offsets: []uint32{0, 1, 2, 2},
			EdgeIDs: []builder.EdgeID{0, 1},
		},
		OverlayAdj: builder.OverlayAdjList{
			Mu:      &sync.RWMutex{},
			Offsets: []uint32{0, 1, 2},
			OverlayEdges: []builder.OverlayEdge{
				{FromNodeID: 1, ToNodeID: 2, ToNodeIdx: 1, IsCrossCell: true},
				{FromNodeID: 2, ToNodeID: 3, ToNodeIdx: 2, IsCrossCell: false},
			},
		},
	}

	index := buildCustomizationIndex(g)

	if got := index.crossCellOverlayByBaseEdge[0]; got != 0 {
		t.Fatalf("expected base edge 0 to map to overlay edge 0, got %d", got)
	}
	if got := index.crossCellOverlayByBaseEdge[1]; got != noOverlayEdgeIdx {
		t.Fatalf("expected base edge 1 to have no cross-cell overlay mapping, got %d", got)
	}
}
