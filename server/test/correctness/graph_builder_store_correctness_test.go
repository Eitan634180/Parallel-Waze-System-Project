package correctness_test

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"nav-system/src/graph/builder"
	"nav-system/src/graph/model"
	graphstore "nav-system/src/graph/store"
	"nav-system/src/utilities"
)

func TestBuildBaseGraphPreservesRawNodeAndWayEdgeMultiset(t *testing.T) {
	pr := baseGraphCorrectnessParseResult()
	g, nodeIdx, err := builder.BuildBaseGraph(pr)
	if err != nil {
		t.Fatalf("BuildBaseGraph: %v", err)
	}

	if len(g.Nodes) != len(pr.Nodes) {
		t.Fatalf("node count changed: got %d want %d", len(g.Nodes), len(pr.Nodes))
	}
	if len(nodeIdx) != len(pr.Nodes) {
		t.Fatalf("raw node index count changed: got %d want %d", len(nodeIdx), len(pr.Nodes))
	}

	rawIDByIdx := make([]builder.NodeRawID, len(g.Nodes))
	for rawID, rawNode := range pr.Nodes {
		idx, ok := nodeIdx[rawID]
		if !ok {
			t.Fatalf("raw node %d missing from node index", rawID)
		}
		if int(idx) >= len(g.Nodes) {
			t.Fatalf("raw node %d maps to out-of-range node index %d", rawID, idx)
		}
		rawIDByIdx[idx] = rawID

		got := g.Nodes[idx]
		assertApprox64(t, got.Lat, rawNode.Lat, "node latitude changed")
		assertApprox64(t, got.Lon, rawNode.Lon, "node longitude changed")
	}

	assertBaseCSRMatchesEdges(t, g)

	wantEdges := expectedDirectedEdgeMultiset(t, pr, nodeIdx)
	gotEdges := make(map[directedEdgeKey]int, len(g.Edges))
	for i := range g.Edges {
		edge := &g.Edges[i]
		if edge.ID != model.EdgeID(i) {
			t.Fatalf("edge at index %d has ID %d", i, edge.ID)
		}
		if int(edge.SrcNode) >= len(rawIDByIdx) || int(edge.DstNode) >= len(rawIDByIdx) {
			t.Fatalf("edge %d uses out-of-range nodes %d -> %d", i, edge.SrcNode, edge.DstNode)
		}

		srcRawID := rawIDByIdx[edge.SrcNode]
		dstRawID := rawIDByIdx[edge.DstNode]
		gotEdges[directedEdgeKey{
			srcRawID: srcRawID,
			dstRawID: dstRawID,
			speedMM:  quantize(edge.SpeedLimit),
			flags:    edge.Flags,
		}]++

		src := pr.Nodes[srcRawID]
		dst := pr.Nodes[dstRawID]
		wantLength := float32(utilities.HaversineM(src.Lat, src.Lon, dst.Lat, dst.Lon))
		wantWeight := wantLength / (edge.SpeedLimit / utilities.KilometersPerHourToMps)
		assertApprox32(t, edge.Length, wantLength, "edge length does not match raw node geometry")
		assertApprox32(t, edge.BaseWeight, wantWeight, "edge weight does not match speed and geometry")
	}

	if len(gotEdges) != len(wantEdges) {
		t.Fatalf("directed edge key count changed: got %d want %d\ngot:  %v\nwant: %v", len(gotEdges), len(wantEdges), gotEdges, wantEdges)
	}
	for key, wantCount := range wantEdges {
		if gotCount := gotEdges[key]; gotCount != wantCount {
			t.Fatalf("directed edge %s count changed: got %d want %d", key, gotCount, wantCount)
		}
	}
}

func TestOverlayGraphMatchesBaseCrossCellEdgesAndIntraCellShortestPaths(t *testing.T) {
	g, _ := buildOverlayCorrectnessGraph(t)
	assertBaseCSRMatchesEdges(t, g)

	gateCount := countGates(g)
	if len(g.Overlay.Offsets) != gateCount+1 {
		t.Fatalf("overlay offset count changed: got %d want %d", len(g.Overlay.Offsets), gateCount+1)
	}
	if len(g.Overlay.Offsets) == 0 || g.Overlay.Offsets[len(g.Overlay.Offsets)-1] != uint32(len(g.Overlay.Edges)) {
		t.Fatalf("overlay final offset = %d, want %d", g.Overlay.Offsets[len(g.Overlay.Offsets)-1], len(g.Overlay.Edges))
	}

	crossByBaseEdge := make(map[model.EdgeID]int)
	shortcutStoreIdx := make(map[uint32]struct{})
	shortcutByPair := make(map[nodePair]float32)

	for gateIdx := 0; gateIdx < gateCount; gateIdx++ {
		start := g.Overlay.Offsets[gateIdx]
		end := g.Overlay.Offsets[gateIdx+1]
		if start > end || int(end) > len(g.Overlay.Edges) {
			t.Fatalf("bad overlay range for gate %d: [%d,%d)", gateIdx, start, end)
		}
		for edgeIdx := start; edgeIdx < end; edgeIdx++ {
			overlayEdge := g.Overlay.Edges[edgeIdx]
			if g.Gates[overlayEdge.SrcNode] != int32(gateIdx) {
				t.Fatalf("overlay edge %d is in gate %d range but has source gate %d", edgeIdx, gateIdx, g.Gates[overlayEdge.SrcNode])
			}
			if g.Gates[overlayEdge.DstNode] == -1 {
				t.Fatalf("overlay edge %d targets non-gate node %d", edgeIdx, overlayEdge.DstNode)
			}

			srcCell := g.Nodes[overlayEdge.SrcNode].CellID
			dstCell := g.Nodes[overlayEdge.DstNode].CellID
			if overlayEdge.IsCrossCell {
				if srcCell == dstCell {
					t.Fatalf("overlay edge %d is marked cross-cell within cell %d", edgeIdx, srcCell)
				}
				if int(overlayEdge.StoreIdx) >= len(g.Edges) {
					t.Fatalf("cross-cell overlay edge %d stores out-of-range base edge %d", edgeIdx, overlayEdge.StoreIdx)
				}
				baseEdge := g.Edges[overlayEdge.StoreIdx]
				if baseEdge.SrcNode != overlayEdge.SrcNode || baseEdge.DstNode != overlayEdge.DstNode {
					t.Fatalf("cross-cell overlay edge %d stores base edge %d as %d -> %d, want %d -> %d",
						edgeIdx, overlayEdge.StoreIdx, baseEdge.SrcNode, baseEdge.DstNode, overlayEdge.SrcNode, overlayEdge.DstNode)
				}
				assertApprox32(t, overlayEdge.BaseWeight, baseEdge.BaseWeight, "cross-cell overlay weight changed from base edge")
				crossByBaseEdge[overlayEdge.StoreIdx]++
				continue
			}

			if srcCell != dstCell {
				t.Fatalf("shortcut overlay edge %d crosses cells %d -> %d", edgeIdx, srcCell, dstCell)
			}
			if _, exists := shortcutStoreIdx[overlayEdge.StoreIdx]; exists {
				t.Fatalf("shortcut store index %d is reused", overlayEdge.StoreIdx)
			}
			shortcutStoreIdx[overlayEdge.StoreIdx] = struct{}{}

			weight, ok := shortestPathWithinCell(g, overlayEdge.SrcNode, overlayEdge.DstNode, srcCell)
			if !ok {
				t.Fatalf("shortcut overlay edge %d connects unreachable gates %d -> %d", edgeIdx, overlayEdge.SrcNode, overlayEdge.DstNode)
			}
			assertApprox32(t, overlayEdge.BaseWeight, weight, "shortcut overlay weight is not the in-cell shortest path")
			shortcutByPair[nodePair{from: overlayEdge.SrcNode, to: overlayEdge.DstNode}] = overlayEdge.BaseWeight
		}
	}

	for edgeID := range g.Edges {
		edge := &g.Edges[edgeID]
		if g.Nodes[edge.SrcNode].CellID == g.Nodes[edge.DstNode].CellID {
			continue
		}
		if got := crossByBaseEdge[model.EdgeID(edgeID)]; got != 1 {
			t.Fatalf("base cross-cell edge %d (%d -> %d) appears in overlay %d times, want 1", edgeID, edge.SrcNode, edge.DstNode, got)
		}
	}

	wantShortcuts := expectedShortcutPairs(t, g)
	if len(shortcutByPair) != len(wantShortcuts) {
		t.Fatalf("shortcut count changed: got %d want %d", len(shortcutByPair), len(wantShortcuts))
	}
	for pair, wantWeight := range wantShortcuts {
		gotWeight, ok := shortcutByPair[pair]
		if !ok {
			t.Fatalf("missing shortcut %d -> %d", pair.from, pair.to)
		}
		assertApprox32(t, gotWeight, wantWeight, "shortcut weight changed")
	}
}

func TestGraphStoreRoundTripPreservesBaseAndOverlayRepresentation(t *testing.T) {
	g, _ := buildOverlayCorrectnessGraph(t)

	dir := t.TempDir()
	if err := graphstore.SaveGraph(g, dir); err != nil {
		t.Fatalf("SaveGraph: %v", err)
	}

	reloaded, err := graphstore.LoadGraph(dir)
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}

	assertGraphRepresentationEqual(t, g, reloaded)
}

type directedEdgeKey struct {
	srcRawID builder.NodeRawID
	dstRawID builder.NodeRawID
	speedMM  int
	flags    uint8
}

func (k directedEdgeKey) String() string {
	return fmt.Sprintf("%d->%d speed=%d flags=%d", k.srcRawID, k.dstRawID, k.speedMM, k.flags)
}

type nodePair struct {
	from uint32
	to   uint32
}

func baseGraphCorrectnessParseResult() *builder.ParseResult {
	return &builder.ParseResult{
		Nodes: map[builder.NodeRawID]*builder.RawNode{
			10: {ID: 10, Lat: 32.0000, Lon: 34.0000},
			20: {ID: 20, Lat: 32.0000, Lon: 34.0010},
			30: {ID: 30, Lat: 32.0010, Lon: 34.0010},
			40: {ID: 40, Lat: 32.0010, Lon: 34.0020},
			50: {ID: 50, Lat: 32.0020, Lon: 34.0020},
		},
		Ways: []*builder.RawWay{
			{ID: 100, NodeRefs: []builder.NodeRawID{10, 20, 30}, RoadClass: model.RoadResidential, MaxSpeed: 36},
			{ID: 101, NodeRefs: []builder.NodeRawID{30, 40}, RoadClass: model.RoadPrimary, IsOneWay: true},
			{ID: 102, NodeRefs: []builder.NodeRawID{40, 50}, RoadClass: model.RoadService, IsOneWay: true, ReverseOneWay: true, MaxSpeed: 18},
			{ID: 103, NodeRefs: []builder.NodeRawID{10, 20}, RoadClass: model.RoadSecondary, MaxSpeed: 72},
		},
	}
}

func buildOverlayCorrectnessGraph(t *testing.T) (*model.Graph, map[builder.NodeRawID]uint32) {
	t.Helper()

	pr := &builder.ParseResult{
		Nodes: map[builder.NodeRawID]*builder.RawNode{
			1: {ID: 1, Lat: 32.0000, Lon: 34.0000},
			2: {ID: 2, Lat: 32.0000, Lon: 34.0010},
			3: {ID: 3, Lat: 32.0000, Lon: 34.0020},
			4: {ID: 4, Lat: 32.0010, Lon: 34.0020},
			5: {ID: 5, Lat: 32.0010, Lon: 34.0010},
		},
		Ways: []*builder.RawWay{
			{ID: 201, NodeRefs: []builder.NodeRawID{1, 2}, RoadClass: model.RoadResidential, MaxSpeed: 60},
			{ID: 202, NodeRefs: []builder.NodeRawID{2, 3}, RoadClass: model.RoadResidential, MaxSpeed: 30},
			{ID: 203, NodeRefs: []builder.NodeRawID{4, 5}, RoadClass: model.RoadResidential, MaxSpeed: 40},
			{ID: 204, NodeRefs: []builder.NodeRawID{3, 4}, RoadClass: model.RoadPrimary, IsOneWay: true, MaxSpeed: 30},
			{ID: 205, NodeRefs: []builder.NodeRawID{3, 4}, RoadClass: model.RoadMotorway, IsOneWay: true, MaxSpeed: 90},
			{ID: 206, NodeRefs: []builder.NodeRawID{2, 5}, RoadClass: model.RoadSecondary, MaxSpeed: 50},
		},
	}

	g, nodeIdx, err := builder.BuildBaseGraph(pr)
	if err != nil {
		t.Fatalf("BuildBaseGraph: %v", err)
	}

	for rawID, idx := range nodeIdx {
		switch rawID {
		case 1, 2, 3:
			g.Nodes[idx].CellID = 0
		case 4, 5:
			g.Nodes[idx].CellID = 1
		default:
			t.Fatalf("unexpected raw node %d", rawID)
		}
	}
	g.Cells = make([]model.Cell, 2)

	builder.DetectGateNodes(g)
	builder.BuildOverlayGraph(g, 2)
	return g, nodeIdx
}

func expectedDirectedEdgeMultiset(t *testing.T, pr *builder.ParseResult, nodeIdx map[builder.NodeRawID]uint32) map[directedEdgeKey]int {
	t.Helper()

	want := make(map[directedEdgeKey]int)
	for _, way := range pr.Ways {
		speedKmh := way.MaxSpeed
		if speedKmh == 0 {
			speedKmh = model.DefaultSpeedKmh(way.RoadClass)
		}

		flags := uint8(0)
		if way.IsOneWay {
			flags |= model.FlagOneWay
		}

		for i := 0; i < len(way.NodeRefs)-1; i++ {
			from := way.NodeRefs[i]
			to := way.NodeRefs[i+1]
			if _, ok := nodeIdx[from]; !ok {
				t.Fatalf("raw way %d references missing node %d", way.ID, from)
			}
			if _, ok := nodeIdx[to]; !ok {
				t.Fatalf("raw way %d references missing node %d", way.ID, to)
			}

			if !way.IsOneWay || !way.ReverseOneWay {
				want[directedEdgeKey{srcRawID: from, dstRawID: to, speedMM: quantize(speedKmh), flags: flags}]++
			}
			if !way.IsOneWay || way.ReverseOneWay {
				want[directedEdgeKey{srcRawID: to, dstRawID: from, speedMM: quantize(speedKmh), flags: flags}]++
			}
		}
	}
	return want
}

func expectedShortcutPairs(t *testing.T, g *model.Graph) map[nodePair]float32 {
	t.Helper()

	want := make(map[nodePair]float32)
	for _, cell := range g.Cells {
		for _, from := range cell.Gates {
			for _, to := range cell.Gates {
				if from == to {
					continue
				}
				weight, ok := shortestPathWithinCell(g, from, to, g.Nodes[from].CellID)
				if ok {
					want[nodePair{from: from, to: to}] = weight
				}
			}
		}
	}
	return want
}

func shortestPathWithinCell(g *model.Graph, src, dst uint32, cellID model.CellID) (float32, bool) {
	dist := make([]float32, len(g.Nodes))
	visited := make([]bool, len(g.Nodes))
	for i := range dist {
		dist[i] = float32(math.MaxFloat32)
	}
	dist[src] = 0

	for {
		current := -1
		best := float32(math.MaxFloat32)
		for i := range dist {
			if visited[i] || g.Nodes[i].CellID != cellID || dist[i] >= best {
				continue
			}
			current = i
			best = dist[i]
		}
		if current == -1 {
			return 0, false
		}
		if uint32(current) == dst {
			return best, true
		}
		visited[current] = true

		start, end := g.Base.EdgeRange(uint32(current))
		for edgeID := start; edgeID < end; edgeID++ {
			edge := &g.Edges[edgeID]
			if g.Nodes[edge.DstNode].CellID != cellID {
				continue
			}
			next := edge.DstNode
			nextWeight := best + edge.BaseWeight
			if nextWeight < dist[next] {
				dist[next] = nextWeight
			}
		}
	}
}

func countGates(g *model.Graph) int {
	count := 0
	for _, gateIdx := range g.Gates {
		if gateIdx != -1 {
			count++
		}
	}
	return count
}

func assertBaseCSRMatchesEdges(t *testing.T, g *model.Graph) {
	t.Helper()

	if len(g.Base.Offsets) != len(g.Nodes)+1 {
		t.Fatalf("base offset count changed: got %d want %d", len(g.Base.Offsets), len(g.Nodes)+1)
	}
	if g.Base.Offsets[len(g.Base.Offsets)-1] != uint32(len(g.Edges)) {
		t.Fatalf("base final offset = %d, want %d", g.Base.Offsets[len(g.Base.Offsets)-1], len(g.Edges))
	}

	for nodeIdx := range g.Nodes {
		start := g.Base.Offsets[nodeIdx]
		end := g.Base.Offsets[nodeIdx+1]
		if start > end || int(end) > len(g.Edges) {
			t.Fatalf("bad base edge range for node %d: [%d,%d)", nodeIdx, start, end)
		}
		for edgeID := start; edgeID < end; edgeID++ {
			edge := g.Edges[edgeID]
			if edge.SrcNode != uint32(nodeIdx) {
				t.Fatalf("edge %d is in node %d CSR range but has source %d", edgeID, nodeIdx, edge.SrcNode)
			}
			if int(edge.DstNode) >= len(g.Nodes) {
				t.Fatalf("edge %d has out-of-range destination %d", edgeID, edge.DstNode)
			}
		}
	}
}

func assertGraphRepresentationEqual(t *testing.T, before, after *model.Graph) {
	t.Helper()

	if len(after.Nodes) != len(before.Nodes) {
		t.Fatalf("node count changed across save/load: got %d want %d", len(after.Nodes), len(before.Nodes))
	}
	for i := range before.Nodes {
		assertApprox64(t, after.Nodes[i].Lat, before.Nodes[i].Lat, "node latitude changed across save/load")
		assertApprox64(t, after.Nodes[i].Lon, before.Nodes[i].Lon, "node longitude changed across save/load")
		assertApprox32(t, after.Nodes[i].X, before.Nodes[i].X, "node projected X changed across save/load")
		assertApprox32(t, after.Nodes[i].Y, before.Nodes[i].Y, "node projected Y changed across save/load")
		if after.Nodes[i].CellID != before.Nodes[i].CellID {
			t.Fatalf("node %d cell changed across save/load: got %d want %d", i, after.Nodes[i].CellID, before.Nodes[i].CellID)
		}
	}

	if !slices.Equal(after.Edges, before.Edges) {
		t.Fatalf("edges changed across save/load:\ngot:  %#v\nwant: %#v", after.Edges, before.Edges)
	}
	if !slices.Equal(after.Base.Offsets, before.Base.Offsets) {
		t.Fatalf("base offsets changed across save/load:\ngot:  %v\nwant: %v", after.Base.Offsets, before.Base.Offsets)
	}
	if len(after.Cells) != len(before.Cells) {
		t.Fatalf("cell count changed across save/load: got %d want %d", len(after.Cells), len(before.Cells))
	}
	for i := range before.Cells {
		if !slices.Equal(after.Cells[i].Gates, before.Cells[i].Gates) {
			t.Fatalf("cell %d gates changed across save/load: got %v want %v", i, after.Cells[i].Gates, before.Cells[i].Gates)
		}
	}
	if !slices.Equal(after.Gates, before.Gates) {
		t.Fatalf("node-to-gate mapping changed across save/load:\ngot:  %v\nwant: %v", after.Gates, before.Gates)
	}
	if !slices.Equal(after.Overlay.Offsets, before.Overlay.Offsets) {
		t.Fatalf("overlay offsets changed across save/load:\ngot:  %v\nwant: %v", after.Overlay.Offsets, before.Overlay.Offsets)
	}
	if !slices.Equal(after.Overlay.Edges, before.Overlay.Edges) {
		t.Fatalf("overlay edges changed across save/load:\ngot:  %#v\nwant: %#v", after.Overlay.Edges, before.Overlay.Edges)
	}

	assertApprox64(t, after.BBox.MinLat, before.BBox.MinLat, "bbox min latitude changed across save/load")
	assertApprox64(t, after.BBox.MaxLat, before.BBox.MaxLat, "bbox max latitude changed across save/load")
	assertApprox64(t, after.BBox.MinLon, before.BBox.MinLon, "bbox min longitude changed across save/load")
	assertApprox64(t, after.BBox.MaxLon, before.BBox.MaxLon, "bbox max longitude changed across save/load")
	assertApprox64(t, after.RefLat, before.RefLat, "reference latitude changed across save/load")
}

func quantize(v float32) int {
	return int(math.Round(float64(v) * 1000))
}

func assertApprox32(t *testing.T, got, want float32, label string) {
	t.Helper()
	if math.Abs(float64(got-want)) > 0.001 {
		t.Fatalf("%s: got %.9f want %.9f", label, got, want)
	}
}

func assertApprox64(t *testing.T, got, want float64, label string) {
	t.Helper()
	if math.Abs(got-want) > 0.000000001 {
		t.Fatalf("%s: got %.12f want %.12f", label, got, want)
	}
}
