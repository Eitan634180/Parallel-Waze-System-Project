package component_test

import (
	"testing"

	"nav-system/src/routing"
	"nav-system/test/testutil"
)

func TestRoutingEngineBuildsOverlayAndRoutesAcrossCorpus(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "grid_city.json", 3)
	cases := testutil.LoadRouteCases(t, "grid_cases.json")

	if len(fixture.Graph.Cells) < 2 {
		t.Fatalf("expected at least 2 cells, got %d", len(fixture.Graph.Cells))
	}
	if len(fixture.Graph.BoundaryNodes) == 0 {
		t.Fatal("expected boundary nodes to be detected")
	}
	if len(fixture.Graph.OverlayAdj.OverlayEdges) == 0 {
		t.Fatal("expected overlay graph to contain edges")
	}

	for _, routeCase := range cases {
		routes := fixture.Router.Compute(routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 2, routing.BaseWeight)
		if len(routes) == 0 {
			t.Fatalf("%s: expected at least one route", routeCase.Name)
		}

		for _, route := range routes {
			testutil.AssertRouteValid(t, fixture.Graph, route, routing.BaseWeight)
		}
	}
}
