package benchmark_test

import (
	"sync/atomic"
	"testing"

	routingengine "nav-system/src/routing/engine"
	routingentities "nav-system/src/routing/entities"
)

func BenchmarkRouterParallelStatic(b *testing.B) {
	fixture := mustLoadFixture(b)
	modes := []routingentities.RoutingMode{
		routingentities.RoutingModeHierarchical,
		routingentities.RoutingModeBaseAStar,
	}

	for _, mode := range modes {
		b.Run(string(mode), func(b *testing.B) {
			router := routingengine.NewRouterWithMode(fixture.Graph, fixture.Snap, mode)
			benchmarkParallelQueries(b, fixture, router, routingengine.BaseWeight)
		})
	}
}

func benchmarkParallelQueries(b *testing.B, fixture *Fixture, router *routingengine.Router, wf routingengine.WeightFunc) {
	var next atomic.Uint64
	var visited atomic.Uint64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := int(next.Add(1)-1) % len(fixture.Corpus)
			query := fixture.Corpus[idx]
			routes, stats := router.ComputeFromIndicesWithStats(query.SrcIdx, query.DstIdx, 1, wf)
			if len(routes) != 1 {
				b.Fatalf("%s: expected one route, got %d", query.Name, len(routes))
			}

			visited.Add(uint64(stats.VisitedNodes))
		}
	})
	b.StopTimer()

	b.ReportMetric(float64(visited.Load())/float64(b.N), "visited_nodes/op")
}
