package traffic

import (
	"testing"

	"nav-system/src/graph/builder"
)

func TestAffectedCellIDsForDirtyEdgesIncludesOnlyIntraCellCells(t *testing.T) {
	g := &builder.Graph{
		Nodes: []builder.Node{
			{ID: 1, CellID: 0},
			{ID: 2, CellID: 0},
			{ID: 3, CellID: 1},
			{ID: 4, CellID: 1},
		},
		NodeIdx: map[builder.NodeID]uint32{
			1: 0,
			2: 1,
			3: 2,
			4: 3,
		},
		Edges: []builder.Edge{
			{ID: 0, FromNodeID: 1, ToNodeID: 2, ToNodeIdx: 1},
			{ID: 1, FromNodeID: 2, ToNodeID: 3, ToNodeIdx: 2},
			{ID: 2, FromNodeID: 3, ToNodeID: 4, ToNodeIdx: 3},
		},
	}

	cellIDs := affectedCellIDsForDirtyEdges(g, map[builder.EdgeID]struct{}{
		0: {},
		1: {},
		2: {},
	})

	if len(cellIDs) != 2 {
		t.Fatalf("expected 2 affected cells, got %d", len(cellIDs))
	}

	found := map[builder.CellID]bool{}
	for _, cellID := range cellIDs {
		found[cellID] = true
	}

	if !found[0] || !found[1] {
		t.Fatalf("expected cells 0 and 1 to be affected, got %#v", cellIDs)
	}
}
