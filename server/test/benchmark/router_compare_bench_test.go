package benchmark_test

import (
	"testing"

	"nav-system/src/routing"
	"nav-system/test/benchutil"
)

func BenchmarkRouterCompareStatic(b *testing.B) {
	fixture := mustLoadFixture(b, "")
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

func BenchmarkRouterCompareLive(b *testing.B) {
	fixture := mustLoadFixture(b, benchmarkTrafficName())
	liveWeight := weightFuncForFixture(fixture.Store)
	modes := []routing.RoutingMode{
		routing.RoutingModeHierarchical,
		routing.RoutingModeBaseAStar,
		routing.RoutingModeBaseDijkstra,
	}

	for _, mode := range modes {
		b.Run(string(mode), func(b *testing.B) {
			router := routing.NewRouterWithMode(fixture.Graph, fixture.Snap, mode)
			benchmarkCorpusQueries(b, fixture, router, liveWeight)
		})
	}
}

func benchmarkCorpusQueries(b *testing.B, fixture *benchutil.Fixture, router *routing.Router, wf routing.WeightFunc) {
	cases := fixture.Corpus
	var total routing.SearchStats

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := cases[i%len(cases)]
		routes, stats := router.ComputeFromIndicesWithStats(query.SrcIdx, query.DstIdx, 1, wf)
		if len(routes) != 1 {
			b.Fatalf("%s: expected one route, got %d", query.Name, len(routes))
		}

		total.SettledBaseNodes += stats.SettledBaseNodes
		total.SettledOverlayNodes += stats.SettledOverlayNodes
		total.RelaxedBaseEdges += stats.RelaxedBaseEdges
		total.RelaxedOverlayEdges += stats.RelaxedOverlayEdges
	}
	b.StopTimer()

	b.ReportMetric(float64(total.SettledBaseNodes)/float64(b.N), "settled_base/op")
	b.ReportMetric(float64(total.SettledOverlayNodes)/float64(b.N), "settled_overlay/op")
	b.ReportMetric(float64(total.RelaxedBaseEdges)/float64(b.N), "relaxed_base/op")
	b.ReportMetric(float64(total.RelaxedOverlayEdges)/float64(b.N), "relaxed_overlay/op")
}
