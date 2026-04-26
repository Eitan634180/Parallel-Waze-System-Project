package unit_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	graphstore "nav-system/src/graph/store"
	"nav-system/test/testutil"
)

func TestGraphStoreRoundTripPreservesRuntimeCellBoundaries(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "diamond_graph.json", 2)

	dir := t.TempDir()
	if err := graphstore.SaveGraph(fixture.Graph, dir); err != nil {
		t.Fatalf("SaveGraph: %v", err)
	}

	reloaded, err := graphstore.LoadGraph(dir)
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}

	if len(reloaded.Cells) != len(fixture.Graph.Cells) {
		t.Fatalf("cell count changed across save/load: got %d want %d", len(reloaded.Cells), len(fixture.Graph.Cells))
	}

	for i := range fixture.Graph.Cells {
		want := fixture.Graph.Cells[i].GateNodes
		got := reloaded.Cells[i].GateNodes
		if len(got) != len(want) {
			t.Fatalf("cell %d gate count changed across save/load: got %d want %d", i, len(got), len(want))
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("cell %d gate %d changed across save/load: got %d want %d", i, j, got[j], want[j])
			}
		}
	}
}

func TestGraphStoreRoundTripPreservesNodeToGateMapping(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "one_way_detour_graph.json", 2)

	dir := t.TempDir()
	if err := graphstore.SaveGraph(fixture.Graph, dir); err != nil {
		t.Fatalf("SaveGraph: %v", err)
	}

	reloaded, err := graphstore.LoadGraph(dir)
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}

	if len(reloaded.NodeToGate) != len(fixture.Graph.NodeToGate) {
		t.Fatalf("node-to-gate length changed across save/load: got %d want %d", len(reloaded.NodeToGate), len(fixture.Graph.NodeToGate))
	}

	for i := range fixture.Graph.NodeToGate {
		if reloaded.NodeToGate[i] != fixture.Graph.NodeToGate[i] {
			t.Fatalf("node %d gate mapping changed across save/load: got %d want %d", i, reloaded.NodeToGate[i], fixture.Graph.NodeToGate[i])
		}
	}
}

func TestGraphStoreRejectsUnsupportedSequenceFileVersion(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "diamond_graph.json", 2)

	dir := t.TempDir()
	if err := graphstore.SaveGraph(fixture.Graph, dir); err != nil {
		t.Fatalf("SaveGraph: %v", err)
	}

	baseAdjPath := filepath.Join(dir, "base_adj.bin")
	data, err := os.ReadFile(baseAdjPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", baseAdjPath, err)
	}
	if len(data) < 6 {
		t.Fatalf("%s too short to contain a version header", baseAdjPath)
	}
	binary.LittleEndian.PutUint16(data[4:6], 3)
	if err := os.WriteFile(baseAdjPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", baseAdjPath, err)
	}

	_, err = graphstore.LoadGraph(dir)
	if err == nil {
		t.Fatal("LoadGraph should reject unsupported graph format versions")
	}
	if !strings.Contains(err.Error(), "unsupported graph format version") {
		t.Fatalf("unexpected error for unsupported version: %v", err)
	}
}
