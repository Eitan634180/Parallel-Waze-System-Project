package component_test

import (
	"testing"

	"nav-system/src/routing"
	"nav-system/test/testutil"
)

func TestGraphPipelineProducesCorrectRoutesForFixtureCorpus(t *testing.T) {
	corpora := []struct {
		graph string
		cases string
		size  int
	}{
		{graph: "grid_city.json", cases: "grid_cases.json", size: 3},
		{graph: "one_way_detour_graph.json", cases: "one_way_detour_cases.json", size: 2},
		{graph: "disconnected_graph.json", cases: "disconnected_cases.json", size: 2},
	}

	for _, corpus := range corpora {
		corpus := corpus
		t.Run(corpus.graph, func(t *testing.T) {
			fixture := testutil.BuildGraphFixture(t, corpus.graph, corpus.size)
			cases := testutil.LoadRouteCases(t, corpus.cases)

			for _, routeCase := range cases {
				srcIdx := testutil.BruteForceSnap(fixture.Graph, routeCase.Src.Lat, routeCase.Src.Lon)
				dstIdx := testutil.BruteForceSnap(fixture.Graph, routeCase.Dst.Lat, routeCase.Dst.Lon)
				oracle, ok := testutil.ShortestPath(fixture.Graph, srcIdx, dstIdx, routing.BaseWeight)
				if !ok {
					t.Fatalf("%s: oracle did not find a path", routeCase.Name)
				}

				routes := fixture.Router.Compute(routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1, routing.BaseWeight)
				if len(routes) != 1 {
					t.Fatalf("%s: expected 1 route, got %d", routeCase.Name, len(routes))
				}
				testutil.AssertRouteMatchesOracle(t, fixture.Graph, routes[0], oracle, routing.BaseWeight)
			}
		})
	}
}
