package benchmark_test

import (
	"testing"

	routingengine "nav-system/src/routing/engine"
	routingentities "nav-system/src/routing/entities"
)

func TestBenchmarkCorpusMatchesBaseAStarStatic(t *testing.T) {
	fixture := mustLoadFixture(t)
	hierarchical := routingengine.NewRouterWithMode(fixture.Graph, fixture.Snap, routingentities.RoutingModeHierarchical)
	baseAStar := routingengine.NewRouterWithMode(fixture.Graph, fixture.Snap, routingentities.RoutingModeBaseAStar)

	for _, query := range fixture.Corpus {
		hierarchicalRoutes := hierarchical.ComputeFromIndices(query.SrcIdx, query.DstIdx, 1, routingengine.BaseWeight)
		baseRoutes := baseAStar.ComputeFromIndices(query.SrcIdx, query.DstIdx, 1, routingengine.BaseWeight)
		if len(hierarchicalRoutes) != 1 || len(baseRoutes) != 1 {
			t.Fatalf("%s: expected one route from both modes, got hierarchical=%d base=%d", query.Name, len(hierarchicalRoutes), len(baseRoutes))
		}

		assertApproxRouteCost(t, hierarchicalRoutes[0].TotalTimeSec, baseRoutes[0].TotalTimeSec, query.Name+" total_time_sec")
		assertApproxRouteCost(t, hierarchicalRoutes[0].TotalDistM, baseRoutes[0].TotalDistM, query.Name+" total_dist_m")
	}
}
