package benchmark_test

import (
	"testing"

	routingengine "nav-system/src/routing/engine"
	routingentities "nav-system/src/routing/entities"
)

func BenchmarkRouterCompareStatic(b *testing.B) {
	fixture := mustLoadFixture(b)
	modes := []routingentities.RoutingMode{
		routingentities.RoutingModeHierarchical,
		routingentities.RoutingModeBaseAStar,
		routingentities.RoutingModeBaseDijkstra,
	}

	for _, mode := range modes {
		b.Run(string(mode), func(b *testing.B) {
			router := routingengine.NewRouterWithMode(fixture.Graph, fixture.Snap, mode)
			benchmarkCorpusQueries(b, fixture, router, routingengine.BaseWeight)
		})
	}
}

func benchmarkCorpusQueries(b *testing.B, fixture *Fixture, router *routingengine.Router, wf routingengine.WeightFunc) {
	cases := fixture.Corpus
	var total routingentities.SearchStats

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := cases[i%len(cases)]
		routes, stats := router.ComputeFromIndicesWithStats(query.SrcIdx, query.DstIdx, 1, wf)
		if len(routes) != 1 {
			b.Fatalf("%s: expected one route, got %d", query.Name, len(routes))
		}

		total.VisitedNodes += stats.VisitedNodes
	}
	b.StopTimer()

	b.ReportMetric(float64(total.VisitedNodes)/float64(b.N), "visited_nodes/op")
}
