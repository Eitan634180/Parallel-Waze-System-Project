package benchmark_test

import (
	"testing"

	"nav-system/src/routing"
)

func BenchmarkRouterCompareStatic(b *testing.B) {
	fixture := mustLoadFixture(b)
	modes := []routing.RoutingMode{
		routing.RoutingModeHierarchical,
		routing.RoutingModeBaseAStar,
		routing.RoutingModeBaseDijkstra,
	}

	for _, mode := range modes {
		b.Run(string(mode), func(b *testing.B) {
			router := routing.NewRouterWithMode(fixture.Graph, fixture.Snap, mode)
			benchmarkCorpusQueries(b, fixture, router, routing.BaseWeight)
		})
	}
}

func benchmarkCorpusQueries(b *testing.B, fixture *Fixture, router *routing.Router, wf routing.WeightFunc) {
	cases := fixture.Corpus
	var total routing.SearchStats

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
