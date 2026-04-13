package correctness_test

import (
	"testing"

	"nav-system/src/graph/builder"
	"nav-system/src/traffic"
	"nav-system/test/testutil"
)

func TestTwoLevelRouterMatchesTrafficWeightedOracle(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "diamond_graph.json", 2)
	store := traffic.NewStore()

	congestedEdge := testutil.FindEdgeID(t, fixture.Graph, 2, 4)
	baseWeight := fixture.Graph.Edges[congestedEdge].Weight
	store.RecordObservation(congestedEdge, baseWeight*40, baseWeight)
	traffic.CustomizeOverlayWeights(fixture.Graph, store)

	liveWeight := func(edge *builder.Edge) float32 {
		return store.LiveWeight(edge.ID, edge.Weight)
	}

	routeCase := testutil.LoadRouteCases(t, "diamond_cases.json")[0]
	srcIdx := testutil.BruteForceSnap(fixture.Graph, routeCase.Src.Lat, routeCase.Src.Lon)
	dstIdx := testutil.BruteForceSnap(fixture.Graph, routeCase.Dst.Lat, routeCase.Dst.Lon)
	oracle, ok := testutil.ShortestPath(fixture.Graph, srcIdx, dstIdx, liveWeight)
	if !ok {
		t.Fatal("oracle did not find a path")
	}

	routes := fixture.Router.Compute(routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1, liveWeight)
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}

	testutil.AssertRouteMatchesOracle(t, fixture.Graph, routes[0], oracle, liveWeight)
	if len(routes[0].Steps) < 3 || routes[0].Steps[1].NodeID != 3 {
		t.Fatalf("expected congestion to divert route through node 3, got %+v", routes[0].Steps)
	}
}
