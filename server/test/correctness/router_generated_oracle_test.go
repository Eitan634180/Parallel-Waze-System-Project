package correctness_test

import (
	"slices"
	"testing"

	"nav-system/src/graph/model"
	routingengine "nav-system/src/routing/engine"
	routingentities "nav-system/src/routing/entities"
	trafficstore "nav-system/src/traffic/store"
	"nav-system/test/testutil"
)

func TestGeneratedStaticCorporaMatchOracleOnBaseModes(t *testing.T) {
	seeds := []int64{11, 29, 57}
	modes := []routingentities.RoutingMode{
		routingentities.RoutingModeBaseAStar,
		routingentities.RoutingModeBaseDijkstra,
	}

	for _, seed := range seeds {
		seed := seed
		t.Run(testName("static", seed), func(t *testing.T) {
			fixture := testutil.BuildGeneratedWeightedFixture(t, seed, 5, 5, 12, 5)
			assertGeneratedFixtureMatchesOracle(
				t,
				fixture,
				testutil.ExhaustiveGeneratedQueries(fixture.NodeIDs),
				modes,
				routingengine.BaseWeight,
			)
		})
	}
}

func TestGeneratedStaticCrossCellCorporaMatchOracleOnHierarchicalMode(t *testing.T) {
	seeds := []int64{11, 29, 57}
	modes := []routingentities.RoutingMode{routingentities.RoutingModeHierarchical}

	for _, seed := range seeds {
		seed := seed
		t.Run(testName("hier_static", seed), func(t *testing.T) {
			fixture := testutil.BuildGeneratedWeightedFixture(t, seed, 5, 5, 12, 5)
			assertGeneratedFixtureMatchesOracle(
				t,
				fixture,
				crossCellQueries(fixture.Graph, testutil.ExhaustiveGeneratedQueries(fixture.NodeIDs)),
				modes,
				routingengine.BaseWeight,
			)
		})
	}
}

func TestGeneratedDirectedCorporaMatchOracleOnBaseModes(t *testing.T) {
	seeds := []int64{101, 203, 307}
	modes := []routingentities.RoutingMode{
		routingentities.RoutingModeBaseAStar,
		routingentities.RoutingModeBaseDijkstra,
	}

	for _, seed := range seeds {
		seed := seed
		t.Run(testName("directed", seed), func(t *testing.T) {
			fixture := testutil.BuildGeneratedDirectedFixture(t, seed, 6, 4, 12, 4)
			assertGeneratedFixtureMatchesOracle(
				t,
				fixture,
				testutil.ExhaustiveGeneratedQueries(fixture.NodeIDs),
				modes,
				routingengine.BaseWeight,
			)
		})
	}
}

func TestGeneratedDirectedCrossCellCorporaMatchOracleOnHierarchicalMode(t *testing.T) {
	seeds := []int64{101, 203, 307}
	modes := []routingentities.RoutingMode{routingentities.RoutingModeHierarchical}

	for _, seed := range seeds {
		seed := seed
		t.Run(testName("hier_directed", seed), func(t *testing.T) {
			fixture := testutil.BuildGeneratedDirectedFixture(t, seed, 6, 4, 12, 4)
			assertGeneratedFixtureMatchesOracle(
				t,
				fixture,
				crossCellQueries(fixture.Graph, testutil.ExhaustiveGeneratedQueries(fixture.NodeIDs)),
				modes,
				routingengine.BaseWeight,
			)
		})
	}
}

func TestGeneratedCustomWeightCorporaMatchOracleOnBaseModes(t *testing.T) {
	seeds := []int64{13, 41, 73}
	modes := []routingentities.RoutingMode{
		routingentities.RoutingModeBaseAStar,
		routingentities.RoutingModeBaseDijkstra,
	}

	for _, seed := range seeds {
		seed := seed
		t.Run(testName("custom_weight", seed), func(t *testing.T) {
			fixture := testutil.BuildGeneratedWeightedFixture(t, seed, 5, 5, 12, 5)
			assertGeneratedFixtureMatchesOracle(
				t,
				fixture,
				testutil.ExhaustiveGeneratedQueries(fixture.NodeIDs),
				modes,
				fixture.WeightFunc,
			)
		})
	}
}

func TestGeneratedTrafficCorporaMatchOracleOnBaseModes(t *testing.T) {
	seeds := []int64{17, 61}
	modes := []routingentities.RoutingMode{
		routingentities.RoutingModeBaseAStar,
		routingentities.RoutingModeBaseDijkstra,
	}

	for _, seed := range seeds {
		seed := seed
		t.Run(testName("traffic", seed), func(t *testing.T) {
			fixture := testutil.BuildGeneratedWeightedFixture(t, seed, 5, 5, 12, 5)
			liveWeight := applyGeneratedTraffic(t, fixture.Graph)
			assertGeneratedFixtureMatchesOracle(
				t,
				fixture,
				testutil.ExhaustiveGeneratedQueries(fixture.NodeIDs),
				modes,
				liveWeight,
			)
		})
	}
}

func TestGeneratedTrafficCrossCellCorporaMatchOracleOnHierarchicalMode(t *testing.T) {
	seeds := []int64{17, 61}
	modes := []routingentities.RoutingMode{routingentities.RoutingModeHierarchical}

	for _, seed := range seeds {
		seed := seed
		t.Run(testName("hier_traffic", seed), func(t *testing.T) {
			fixture := testutil.BuildGeneratedWeightedFixture(t, seed, 5, 5, 12, 5)
			liveWeight := applyGeneratedTraffic(t, fixture.Graph)
			assertGeneratedFixtureMatchesOracle(
				t,
				fixture,
				crossCellQueries(fixture.Graph, testutil.ExhaustiveGeneratedQueries(fixture.NodeIDs)),
				modes,
				liveWeight,
			)
		})
	}
}

func TestGeneratedLongCrossCellRoutesMatchOracle(t *testing.T) {
	testCases := []struct {
		name  string
		seed  int64
		build func(t testing.TB) (*testutil.GeneratedWeightedFixture, routingengine.WeightFunc)
	}{
		{
			name: "hierarchical_static_long",
			seed: 91,
			build: func(t testing.TB) (*testutil.GeneratedWeightedFixture, routingengine.WeightFunc) {
				fixture := testutil.BuildGeneratedWeightedFixture(t, 91, 8, 8, 12, 6)
				return fixture, routingengine.BaseWeight
			},
		},
		{
			name: "hierarchical_traffic_long",
			seed: 131,
			build: func(t testing.TB) (*testutil.GeneratedWeightedFixture, routingengine.WeightFunc) {
				fixture := testutil.BuildGeneratedWeightedFixture(t, 131, 8, 8, 12, 6)
				return fixture, applyGeneratedTraffic(t, fixture.Graph)
			},
		},
		{
			name: "base_custom_long",
			seed: 173,
			build: func(t testing.TB) (*testutil.GeneratedWeightedFixture, routingengine.WeightFunc) {
				fixture := testutil.BuildGeneratedWeightedFixture(t, 173, 8, 8, 12, 6)
				return fixture, fixture.WeightFunc
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fixture, wf := tc.build(t)
			modes := longPathModes(tc.name)
			queries := longCrossCellQueries(fixture.Graph, fixture.NodeIDs, wf, 12, 48)
			if len(queries) == 0 {
				t.Fatal("expected long cross-cell queries")
			}
			assertGeneratedFixtureMatchesOracle(t, fixture, queries, modes, wf)
		})
	}
}

func assertGeneratedFixtureMatchesOracle(
	t *testing.T,
	fixture *testutil.GeneratedWeightedFixture,
	queries []testutil.GeneratedQuery,
	modes []routingentities.RoutingMode,
	wf routingengine.WeightFunc,
) {
	t.Helper()

	nodeIdx := fixture.Graph.BuildNodeIdxMap()
	for _, query := range queries {
		srcIdx := nodeIdx[query.SrcNodeID]
		dstIdx := nodeIdx[query.DstNodeID]

		oracle, ok := testutil.ShortestPath(fixture.Graph, srcIdx, dstIdx, wf)
		for _, mode := range modes {
			router := routingengine.NewRouterWithMode(fixture.Graph, fixture.Snap, mode)
			t.Run(string(mode)+"/"+query.Name, func(t *testing.T) {
				routes := router.ComputeFromIndices(srcIdx, dstIdx, 1, wf)
				if !ok {
					if len(routes) != 0 {
						t.Fatalf("expected no route for unreachable query, got %d", len(routes))
					}
					return
				}
				if len(routes) != 1 {
					t.Fatalf("expected 1 route, got %d", len(routes))
				}

				testutil.AssertRouteMatchesOracle(t, fixture.Graph, routes[0], oracle, wf)
			})
		}
	}
}

func crossCellQueries(g *model.Graph, queries []testutil.GeneratedQuery) []testutil.GeneratedQuery {
	nodeIdx := g.BuildNodeIdxMap()
	filtered := make([]testutil.GeneratedQuery, 0, len(queries))
	for _, query := range queries {
		srcIdx := nodeIdx[query.SrcNodeID]
		dstIdx := nodeIdx[query.DstNodeID]
		if g.Nodes[srcIdx].CellID == g.Nodes[dstIdx].CellID {
			continue
		}
		filtered = append(filtered, query)
	}
	return filtered
}

func longCrossCellQueries(
	g *model.Graph,
	nodeIDs []model.NodeRawID,
	wf routingengine.WeightFunc,
	minEdges int,
	limit int,
) []testutil.GeneratedQuery {
	all := crossCellQueries(g, testutil.ExhaustiveGeneratedQueries(nodeIDs))
	type scoredQuery struct {
		query testutil.GeneratedQuery
		edges int
	}

	nodeIdx := g.BuildNodeIdxMap()
	scored := make([]scoredQuery, 0, len(all))
	for _, query := range all {
		srcIdx := nodeIdx[query.SrcNodeID]
		dstIdx := nodeIdx[query.DstNodeID]
		oracle, ok := testutil.ShortestPath(g, srcIdx, dstIdx, wf)
		if !ok || len(oracle.EdgeIDs) < minEdges {
			continue
		}
		scored = append(scored, scoredQuery{query: query, edges: len(oracle.EdgeIDs)})
	}

	slices.SortFunc(scored, func(a, b scoredQuery) int {
		if a.edges != b.edges {
			return b.edges - a.edges
		}
		if a.query.SrcNodeID != b.query.SrcNodeID {
			if a.query.SrcNodeID < b.query.SrcNodeID {
				return -1
			}
			return 1
		}
		if a.query.DstNodeID < b.query.DstNodeID {
			return -1
		}
		if a.query.DstNodeID > b.query.DstNodeID {
			return 1
		}
		return 0
	})

	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}

	filtered := make([]testutil.GeneratedQuery, 0, len(scored))
	for _, entry := range scored {
		filtered = append(filtered, entry.query)
	}
	return filtered
}

func longPathModes(name string) []routingentities.RoutingMode {
	if name == "base_custom_long" {
		return []routingentities.RoutingMode{
			routingentities.RoutingModeBaseAStar,
			routingentities.RoutingModeBaseDijkstra,
		}
	}
	return []routingentities.RoutingMode{routingentities.RoutingModeHierarchical}
}

func applyGeneratedTraffic(t testing.TB, g *model.Graph) routingengine.WeightFunc {
	t.Helper()

	store := trafficstore.NewStoreWithCapacity(len(g.Edges))
	customizer := trafficstore.NewCustomizer(g)
	for edgeID := range g.Edges {
		edge := &g.Edges[edgeID]
		switch edgeID % 4 {
		case 0:
			store.RecordObservation(edge.ID, edge.BaseWeight*1.35, edge.BaseWeight)
		case 1:
			store.RecordObservation(edge.ID, edge.BaseWeight*1.8, edge.BaseWeight)
		case 2:
			store.RecordObservation(edge.ID, edge.BaseWeight*2.25, edge.BaseWeight)
		}
	}

	customizer.Customize(store)
	return func(edge *model.Edge) float32 {
		return store.LiveWeight(edge.ID, edge.BaseWeight)
	}
}

func testName(prefix string, seed int64) string {
	return prefix + "_seed_" + itoa(seed)
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}

	var digits [20]byte
	i := len(digits)
	for v > 0 {
		i--
		digits[i] = byte('0' + (v % 10))
		v /= 10
	}
	return string(digits[i:])
}
