package correctness_test

import (
	"testing"

	routingengine "nav-system/src/routing/engine"
	"nav-system/test/testutil"
)

func TestTwoLevelRouterMatchesBaseGraphOracle(t *testing.T) {
	testCases := []struct {
		graph string
		cases string
		size  int
	}{
		{graph: "diamond_graph.json", cases: "diamond_cases.json", size: 2},
		{graph: "grid_city.json", cases: "grid_cases.json", size: 3},
		{graph: "one_way_detour_graph.json", cases: "one_way_detour_cases.json", size: 2},
		{graph: "disconnected_graph.json", cases: "disconnected_cases.json", size: 2},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.graph, func(t *testing.T) {
			fixture := testutil.BuildGraphFixture(t, tc.graph, tc.size)
			for _, routeCase := range testutil.LoadRouteCases(t, tc.cases) {
				srcIdx := testutil.BruteForceSnap(fixture.Graph, routeCase.Src.Lat, routeCase.Src.Lon)
				dstIdx := testutil.BruteForceSnap(fixture.Graph, routeCase.Dst.Lat, routeCase.Dst.Lon)
				oracle, ok := testutil.ShortestPath(fixture.Graph, srcIdx, dstIdx, routingengine.BaseWeight)
				if !ok {
					t.Fatalf("%s: oracle did not find a path", routeCase.Name)
				}

				routes := fixture.Router.Compute(routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1, routingengine.BaseWeight)
				if len(routes) != 1 {
					t.Fatalf("%s: expected 1 route, got %d", routeCase.Name, len(routes))
				}

				testutil.AssertRouteMatchesOracle(t, fixture.Graph, routes[0], oracle, routingengine.BaseWeight)
			}
		})
	}
}
