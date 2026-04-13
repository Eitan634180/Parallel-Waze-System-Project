package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"nav-system/src/api"
	"nav-system/src/graph/builder"
	"nav-system/src/routing"
	"nav-system/src/session"
	"nav-system/src/simulation"
	"nav-system/src/traffic"
)

type graphFixture struct {
	Nodes []struct {
		ID  uint64  `json:"id"`
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"nodes"`
	Ways []struct {
		ID        uint64   `json:"id"`
		NodeRefs  []uint64 `json:"node_refs"`
		RoadClass uint8    `json:"road_class"`
		IsOneWay  bool     `json:"is_one_way"`
		MaxSpeed  float32  `json:"max_speed"`
	} `json:"ways"`
}

type Point struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type RouteCase struct {
	Name string `json:"name"`
	Src  Point  `json:"src"`
	Dst  Point  `json:"dst"`
}

type BuiltGraphFixture struct {
	Graph  *builder.Graph
	Snap   *routing.SnapIndex
	Router *routing.Router
}

type ServerFixture struct {
	Graph      *builder.Graph
	Snap       *routing.SnapIndex
	Router     *routing.Router
	Store      *traffic.Store
	Manager    *session.Manager
	Simulation *simulation.Manager
	Server     *api.Server
}

func testdataRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "testdata")
}

func loadJSONFixture[T any](tb testing.TB, path string) T {
	tb.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read fixture %s: %v", path, err)
	}

	var payload T
	if err := json.Unmarshal(data, &payload); err != nil {
		tb.Fatalf("decode fixture %s: %v", path, err)
	}
	return payload
}

func LoadParseResultFixture(tb testing.TB, name string) *builder.ParseResult {
	tb.Helper()

	fixture := loadJSONFixture[graphFixture](tb, filepath.Join(testdataRoot(), "synthetic", name))
	nodes := make(map[uint64]*builder.RawNode, len(fixture.Nodes))
	for _, node := range fixture.Nodes {
		nodes[node.ID] = &builder.RawNode{
			ID:  node.ID,
			Lat: node.Lat,
			Lon: node.Lon,
		}
	}

	ways := make([]*builder.RawWay, 0, len(fixture.Ways))
	for _, way := range fixture.Ways {
		refs := append([]uint64(nil), way.NodeRefs...)
		ways = append(ways, &builder.RawWay{
			ID:        way.ID,
			NodeRefs:  refs,
			RoadClass: way.RoadClass,
			IsOneWay:  way.IsOneWay,
			MaxSpeed:  way.MaxSpeed,
		})
	}

	return &builder.ParseResult{
		Nodes: nodes,
		Ways:  ways,
	}
}

func LoadRouteCases(tb testing.TB, name string) []RouteCase {
	tb.Helper()
	return loadJSONFixture[[]RouteCase](tb, filepath.Join(testdataRoot(), "route-cases", name))
}

func BuildGraphFixture(tb testing.TB, graphName string, maxCellSize int) *BuiltGraphFixture {
	tb.Helper()

	parseResult := LoadParseResultFixture(tb, graphName)
	g, err := builder.BuildGraph(parseResult)
	if err != nil {
		tb.Fatalf("BuildGraph(%s): %v", graphName, err)
	}

	builder.PartitionCells(g, maxCellSize)
	builder.DetectBoundaryNodes(g)
	builder.BuildOverlayGraph(g, 1)

	snap := routing.BuildSnapIndex(g)
	return &BuiltGraphFixture{
		Graph:  g,
		Snap:   snap,
		Router: routing.NewRouter(g, snap),
	}
}

func BuildServerFixture(tb testing.TB, graphName string, maxCellSize int) *ServerFixture {
	tb.Helper()

	built := BuildGraphFixture(tb, graphName, maxCellSize)
	store := traffic.NewStore()
	manager := session.NewManager()
	simManager := simulation.NewManager(built.Graph, store)
	srv := api.NewServer(built.Graph, store, manager, built.Router, simManager)

	return &ServerFixture{
		Graph:      built.Graph,
		Snap:       built.Snap,
		Router:     built.Router,
		Store:      store,
		Manager:    manager,
		Simulation: simManager,
		Server:     srv,
	}
}

func FindEdgeID(tb testing.TB, g *builder.Graph, fromID, toID builder.NodeID) builder.EdgeID {
	tb.Helper()

	fromIdx, ok := g.NodeIdx[fromID]
	if !ok {
		tb.Fatalf("from node %d not found", fromID)
	}

	for _, edgeID := range g.BaseAdj.Neighbours(fromIdx) {
		if g.Edges[edgeID].ToNodeID == toID {
			return edgeID
		}
	}

	tb.Fatalf("edge %d -> %d not found", fromID, toID)
	return 0
}
