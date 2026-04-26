package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"nav-system/src/graph/builder"
	"nav-system/src/graph/model"
	navigationsessions "nav-system/src/navigation/sessions"
	routingengine "nav-system/src/routing/engine"
	"nav-system/src/simulation"
	trafficstore "nav-system/src/traffic/store"
	"nav-system/src/transport"
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
	Graph   *model.Graph
	Snap    *routingengine.SnapIndex
	Router  *routingengine.Router
	NodeIdx map[builder.NodeRawID]uint32
}

type ServerFixture struct {
	Graph      *model.Graph
	Snap       *routingengine.SnapIndex
	Router     *routingengine.Router
	Store      *trafficstore.Store
	Manager    *navigationsessions.Manager
	Simulation *simulation.Manager
	Server     *transport.Server
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
	nodes := make(map[builder.NodeRawID]*builder.RawNode, len(fixture.Nodes))
	for _, node := range fixture.Nodes {
		nodes[builder.NodeRawID(node.ID)] = &builder.RawNode{
			ID:  builder.NodeRawID(node.ID),
			Lat: node.Lat,
			Lon: node.Lon,
		}
	}

	ways := make([]*builder.RawWay, 0, len(fixture.Ways))
	for _, way := range fixture.Ways {
		refs := make([]builder.NodeRawID, len(way.NodeRefs))
		for i, ref := range way.NodeRefs {
			refs[i] = builder.NodeRawID(ref)
		}
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
	g, nodeIdx, err := builder.BuildBaseGraph(parseResult)
	if err != nil {
		tb.Fatalf("BuildGraph(%s): %v", graphName, err)
	}

	builder.PartitionCells(g, maxCellSize)
	builder.ReorderGraphByCell(g, nodeIdx)
	builder.DetectGateNodes(g)
	builder.BuildOverlayGraph(g, 1)

	snap := routingengine.BuildSnapIndex(g)
	return &BuiltGraphFixture{
		Graph:   g,
		Snap:    snap,
		Router:  routingengine.NewRouter(g, snap),
		NodeIdx: nodeIdx,
	}
}

func BuildServerFixture(tb testing.TB, graphName string, maxCellSize int) *ServerFixture {
	tb.Helper()

	tb.Setenv("NAV_SEARCH_UPSTREAM_URL", "https://example.test/search")
	tb.Setenv("NAV_SEARCH_USER_AGENT", "nav-system-tests/1.0")

	built := BuildGraphFixture(tb, graphName, maxCellSize)
	store := trafficstore.NewStoreWithCapacity(len(built.Graph.Edges))
	store.InitOverlayWeights(built.Graph)
	built.Router.SetOverlayWeightFunc(func(edgeIdx uint32, overlayEdge *model.OverlayEdge) float32 {
		return store.OverlayWeight(edgeIdx, overlayEdge)
	})
	manager := navigationsessions.NewManager()
	simManager := simulation.NewManager(built.Graph, store, built.Router, func() routingengine.WeightFunc {
		return func(e *model.Edge) float32 {
			return store.LiveWeight(e.ID, e.BaseWeight)
		}
	})
	srv := transport.NewServer(built.Graph, store, manager, built.Router, simManager)

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

func FindEdgeID(tb testing.TB, fixture *BuiltGraphFixture, fromID, toID builder.NodeRawID) model.EdgeID {
	tb.Helper()

	fromIdx, ok := fixture.NodeIdx[fromID]
	if !ok {
		tb.Fatalf("from node %d not found", fromID)
	}
	toIdx, ok := fixture.NodeIdx[toID]
	if !ok {
		tb.Fatalf("to node %d not found", toID)
	}

	start, end := fixture.Graph.Base.EdgeRange(fromIdx)
	for eid := start; eid < end; eid++ {
		if fixture.Graph.Edges[eid].ToNode == toIdx {
			return eid
		}
	}

	tb.Fatalf("edge %d -> %d not found", fromID, toID)
	return 0
}
