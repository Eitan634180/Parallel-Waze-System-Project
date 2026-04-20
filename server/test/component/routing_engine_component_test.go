package component_test

import (
	"testing"

	"nav-system/src/routing"
	"nav-system/test/testutil"
)

func TestRoutingEngineBuildsOverlayAndRoutesAcrossCorpus(t *testing.T) {
	corpora := []struct {
		graph string
		cases string
		size  int
	}{
		{graph: "grid_city.json", cases: "grid_cases.json", size: 3},
		{graph: "one_way_detour_graph.json", cases: "one_way_detour_cases.json", size: 2},
	}

	for _, corpus := range corpora {
		corpus := corpus
		t.Run(corpus.graph, func(t *testing.T) {
			fixture := testutil.BuildGraphFixture(t, corpus.graph, corpus.size)
			cases := testutil.LoadRouteCases(t, corpus.cases)

			if len(fixture.Graph.Cells) < 2 {
				t.Fatalf("expected at least 2 cells, got %d", len(fixture.Graph.Cells))
			}
			if len(fixture.Graph.BoundaryBaseIdxs) == 0 {
				t.Fatal("expected boundary nodes to be detected")
			}
			if len(fixture.Graph.OverlayAdj.OverlayEdges) == 0 {
				t.Fatal("expected overlay graph to contain edges")
			}

			for _, routeCase := range cases {
				routes := fixture.Router.Compute(routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1, routing.BaseWeight)
				if len(routes) == 0 {
					t.Fatalf("%s: expected at least one route", routeCase.Name)
				}

				for _, route := range routes {
					testutil.AssertRouteValid(t, fixture.Graph, route, routing.BaseWeight)
				}
			}
		})
	}
}
