package correctness_test

import (
	"testing"

	"nav-system/src/graph/model"
	routingengine "nav-system/src/routing/engine"
	routingentities "nav-system/src/routing/entities"
	trafficstore "nav-system/src/traffic/store"
	"nav-system/test/testutil"
)

// TestRouterReturnsSameSourceDestinationIsNil verifies the router returns no
// route when source and destination snap to the same node.
func TestRouterReturnsSameSourceDestinationIsNil(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "diamond_graph.json", 2)
	routes := fixture.Router.Compute(32.0000, 34.0000, 32.0000, 34.0000, 1, routingengine.BaseWeight)
	if len(routes) != 0 {
		t.Fatalf("same-node route should return nil, got %d routes", len(routes))
	}
}

// TestAlternativeRoutesAreDistinctPaths verifies that when k=2 and the diamond
// graph provides two structural paths, both are returned and follow different edges.
func TestAlternativeRoutesAreDistinctPaths(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "diamond_graph.json", 2)
	routeCase := testutil.LoadRouteCases(t, "diamond_cases.json")[0]

	routes := fixture.Router.Compute(
		routeCase.Src.Lat, routeCase.Src.Lon,
		routeCase.Dst.Lat, routeCase.Dst.Lon,
		2, routingengine.BaseWeight,
	)
	if len(routes) < 2 {
		t.Fatalf("expected at least 2 alternative routes, got %d", len(routes))
	}

	// Collect edge sets for each route.
	edgeSet := func(route routingentities.Route) map[uint32]struct{} {
		m := make(map[uint32]struct{})
		for _, step := range route.Steps {
			if step.EdgeID != nil {
				m[*step.EdgeID] = struct{}{}
			}
		}
		return m
	}

	set0 := edgeSet(routes[0])
	set1 := edgeSet(routes[1])

	// The two paths must not use exactly the same edges.
	identical := len(set0) == len(set1)
	if identical {
		for eid := range set0 {
			if _, ok := set1[eid]; !ok {
				identical = false
				break
			}
		}
	}
	if identical {
		t.Fatal("alternative routes should not traverse the exact same edges")
	}
}

// TestAlternativeRoutesBothValid verifies that all returned alternative routes
// are internally valid (monotonic cumulative metrics, correct edge connectivity).
func TestAlternativeRoutesBothValid(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "diamond_graph.json", 2)
	routeCase := testutil.LoadRouteCases(t, "diamond_cases.json")[0]

	routes := fixture.Router.Compute(
		routeCase.Src.Lat, routeCase.Src.Lon,
		routeCase.Dst.Lat, routeCase.Dst.Lon,
		2, routingengine.BaseWeight,
	)
	for i, route := range routes {
		_ = testutil.AssertRouteValid(t, fixture.Graph, route, routingengine.BaseWeight)
		if t.Failed() {
			t.Fatalf("route %d failed validation", i)
		}
	}
}

// TestTrafficCongestedEdgeRaisesLiveWeight verifies that after congestion is
// recorded the live weight on that edge is higher than the base weight.
func TestTrafficCongestedEdgeRaisesLiveWeight(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "diamond_graph.json", 2)
	store := trafficstore.NewStore()

	edgeID := testutil.FindEdgeID(t, fixture.Graph, 2, 4)
	baseWeight := fixture.Graph.Edges[edgeID].Weight

	// Record 50 observations at 5× the base time to saturate the EWMA.
	for i := 0; i < 50; i++ {
		store.RecordObservation(edgeID, baseWeight*5.0, baseWeight)
	}

	live := store.LiveWeight(edgeID, baseWeight)
	if live <= baseWeight {
		t.Fatalf("live weight %.4f should exceed base weight %.4f after congestion observations", live, baseWeight)
	}
}

// TestGridRoutesAllMatchOracle runs the full grid corpus through the router and
// verifies every route cost matches the Dijkstra oracle within floating-point tolerance.
func TestGridRoutesAllMatchOracle(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "grid_city.json", 3)
	cases := testutil.LoadRouteCases(t, "grid_cases.json")

	for _, routeCase := range cases {
		t.Run(routeCase.Name, func(t *testing.T) {
			srcIdx := testutil.BruteForceSnap(fixture.Graph, routeCase.Src.Lat, routeCase.Src.Lon)
			dstIdx := testutil.BruteForceSnap(fixture.Graph, routeCase.Dst.Lat, routeCase.Dst.Lon)

			oracle, ok := testutil.ShortestPath(fixture.Graph, srcIdx, dstIdx, routingengine.BaseWeight)
			if !ok {
				t.Fatalf("oracle could not find path for %s", routeCase.Name)
			}

			routes := fixture.Router.Compute(
				routeCase.Src.Lat, routeCase.Src.Lon,
				routeCase.Dst.Lat, routeCase.Dst.Lon,
				1, routingengine.BaseWeight,
			)
			if len(routes) != 1 {
				t.Fatalf("expected 1 route, got %d", len(routes))
			}
			testutil.AssertRouteMatchesOracle(t, fixture.Graph, routes[0], oracle, routingengine.BaseWeight)
		})
	}
}

// TestRouteDistanceIsPositive ensures that every step in every returned route
// increases cumulative distance (no zero-weight loops).
func TestRouteDistanceIsPositive(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "grid_city.json", 3)
	cases := testutil.LoadRouteCases(t, "grid_cases.json")

	for _, routeCase := range cases {
		routes := fixture.Router.Compute(
			routeCase.Src.Lat, routeCase.Src.Lon,
			routeCase.Dst.Lat, routeCase.Dst.Lon,
			1, routingengine.BaseWeight,
		)
		if len(routes) == 0 {
			t.Fatalf("%s: no route returned", routeCase.Name)
		}
		for _, step := range routes[0].Steps[1:] {
			if step.DistanceM <= 0 {
				t.Fatalf("%s: step cumulative distance must be positive, got %.4f", routeCase.Name, step.DistanceM)
			}
		}
		if routes[0].TotalDistM <= 0 {
			t.Fatalf("%s: total route distance must be positive, got %.4f", routeCase.Name, routes[0].TotalDistM)
		}
	}
}

func TestRouterRespectsOneWayDetours(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "one_way_detour_graph.json", 2)
	routeCases := testutil.LoadRouteCases(t, "one_way_detour_cases.json")
	nodeIdx := fixture.Graph.BuildNodeIdxMap()

	reverseCase := routeCases[1]
	routes := fixture.Router.Compute(
		reverseCase.Src.Lat, reverseCase.Src.Lon,
		reverseCase.Dst.Lat, reverseCase.Dst.Lon,
		1, routingengine.BaseWeight,
	)
	if len(routes) != 1 {
		t.Fatalf("expected 1 reverse detour route, got %d", len(routes))
	}
	validated := testutil.AssertRouteValid(t, fixture.Graph, routes[0], routingengine.BaseWeight)
	if validated.SourceIdx != nodeIdx[3] || validated.TargetIdx != nodeIdx[1] {
		t.Fatalf("unexpected endpoints for reverse detour route: %+v", validated)
	}

	expectedNodes := []uint32{nodeIdx[3], nodeIdx[6], nodeIdx[5], nodeIdx[4], nodeIdx[1]}
	if len(routes[0].Steps) != len(expectedNodes) {
		t.Fatalf("reverse detour route should have %d steps, got %d", len(expectedNodes), len(routes[0].Steps))
	}
	for i, step := range routes[0].Steps {
		if step.NodeIdx != expectedNodes[i] {
			t.Fatalf("reverse detour route step %d = %d, want %d", i, step.NodeIdx, expectedNodes[i])
		}
	}
}

func TestRouterReturnsNoRouteAcrossDisconnectedComponents(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "disconnected_graph.json", 2)
	routes := fixture.Router.Compute(32.0000, 34.0000, 32.0110, 34.0110, 1, routingengine.BaseWeight)
	if len(routes) != 0 {
		t.Fatalf("disconnected graph should not yield a route, got %d", len(routes))
	}
}

func TestOneWayGraphRoutesStillMatchOracle(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "one_way_detour_graph.json", 2)
	for _, routeCase := range testutil.LoadRouteCases(t, "one_way_detour_cases.json") {
		t.Run(routeCase.Name, func(t *testing.T) {
			srcIdx := testutil.BruteForceSnap(fixture.Graph, routeCase.Src.Lat, routeCase.Src.Lon)
			dstIdx := testutil.BruteForceSnap(fixture.Graph, routeCase.Dst.Lat, routeCase.Dst.Lon)
			oracle, ok := testutil.ShortestPath(fixture.Graph, srcIdx, dstIdx, routingengine.BaseWeight)
			if !ok {
				t.Fatalf("oracle could not find path for %s", routeCase.Name)
			}

			routes := fixture.Router.Compute(routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1, routingengine.BaseWeight)
			if len(routes) != 1 {
				t.Fatalf("expected 1 route, got %d", len(routes))
			}
			testutil.AssertRouteMatchesOracle(t, fixture.Graph, routes[0], oracle, routingengine.BaseWeight)
		})
	}
}

func TestTrafficStoreHandlesSparseHighEdgeIDs(t *testing.T) {
	store := trafficstore.NewStore()
	const base = float32(15.0)
	edgeID := model.EdgeID(4096)

	store.RecordObservation(edgeID, base*2, base)
	if multiplier := store.Multiplier(edgeID); multiplier <= 1.0 {
		t.Fatalf("expected sparse edge %d to record a multiplier, got %.3f", edgeID, multiplier)
	}
	if live := store.LiveWeight(edgeID, base); live <= base {
		t.Fatalf("expected sparse edge %d live weight to exceed base, got %.3f", edgeID, live)
	}
}
