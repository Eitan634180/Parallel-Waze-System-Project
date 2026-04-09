package benchmark_test

import (
	"testing"

	"nav-system/src/routing"
)

func TestBenchmarkCorpusMatchesBaseAStarStatic(t *testing.T) {
	fixture := mustLoadFixture(t, "")
	hierarchical := routing.NewRouterWithMode(fixture.Graph, fixture.Snap, routing.RoutingModeHierarchical)
	baseAStar := routing.NewRouterWithMode(fixture.Graph, fixture.Snap, routing.RoutingModeBaseAStar)

	for _, query := range fixture.Corpus {
		hierarchicalRoutes := hierarchical.ComputeFromIndices(query.SrcIdx, query.DstIdx, 1, routing.BaseWeight)
		baseRoutes := baseAStar.ComputeFromIndices(query.SrcIdx, query.DstIdx, 1, routing.BaseWeight)
		if len(hierarchicalRoutes) != 1 || len(baseRoutes) != 1 {
			t.Fatalf("%s: expected one route from both modes, got hierarchical=%d base=%d", query.Name, len(hierarchicalRoutes), len(baseRoutes))
		}

		assertApproxRouteCost(t, hierarchicalRoutes[0].TotalTimeSec, baseRoutes[0].TotalTimeSec, query.Name+" total_time_sec")
		assertApproxRouteCost(t, hierarchicalRoutes[0].TotalDistM, baseRoutes[0].TotalDistM, query.Name+" total_dist_m")
	}
}

func TestBenchmarkCorpusMatchesBaseAStarLiveTraffic(t *testing.T) {
	fixture := mustLoadFixture(t, benchmarkTrafficName())
	hierarchical := routing.NewRouterWithMode(fixture.Graph, fixture.Snap, routing.RoutingModeHierarchical)
	baseAStar := routing.NewRouterWithMode(fixture.Graph, fixture.Snap, routing.RoutingModeBaseAStar)
	liveWeight := weightFuncForFixture(fixture.Store)

	for _, query := range fixture.Corpus {
		hierarchicalRoutes := hierarchical.ComputeFromIndices(query.SrcIdx, query.DstIdx, 1, liveWeight)
		baseRoutes := baseAStar.ComputeFromIndices(query.SrcIdx, query.DstIdx, 1, liveWeight)
		if len(hierarchicalRoutes) != 1 || len(baseRoutes) != 1 {
			t.Fatalf("%s: expected one route from both live modes, got hierarchical=%d base=%d", query.Name, len(hierarchicalRoutes), len(baseRoutes))
		}

		assertApproxRouteCost(t, hierarchicalRoutes[0].TotalTimeSec, baseRoutes[0].TotalTimeSec, query.Name+" total_time_sec")
		assertApproxRouteCost(t, hierarchicalRoutes[0].TotalDistM, baseRoutes[0].TotalDistM, query.Name+" total_dist_m")
	}
}
