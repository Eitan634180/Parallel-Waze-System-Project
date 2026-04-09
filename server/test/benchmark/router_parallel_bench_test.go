package benchmark_test

import (
	"sync/atomic"
	"testing"

	"nav-system/src/routing"
	"nav-system/test/benchutil"
)

func BenchmarkRouterParallelStatic(b *testing.B) {
	fixture := mustLoadFixture(b, "")
	modes := []routing.RoutingMode{
		routing.RoutingModeHierarchical,
		routing.RoutingModeBaseAStar,
	}

	for _, mode := range modes {
		b.Run(string(mode), func(b *testing.B) {
			router := routing.NewRouterWithMode(fixture.Graph, fixture.Snap, mode)
			benchmarkParallelQueries(b, fixture, router, routing.BaseWeight)
		})
	}
}

func BenchmarkRouterParallelLive(b *testing.B) {
	fixture := mustLoadFixture(b, benchmarkTrafficName())
	liveWeight := weightFuncForFixture(fixture.Store)
	modes := []routing.RoutingMode{
		routing.RoutingModeHierarchical,
		routing.RoutingModeBaseAStar,
	}

	for _, mode := range modes {
		b.Run(string(mode), func(b *testing.B) {
			router := routing.NewRouterWithMode(fixture.Graph, fixture.Snap, mode)
			benchmarkParallelQueries(b, fixture, router, liveWeight)
		})
	}
}

func benchmarkParallelQueries(b *testing.B, fixture *benchutil.Fixture, router *routing.Router, wf routing.WeightFunc) {
	var next atomic.Uint64
	var settledBase atomic.Uint64
	var settledOverlay atomic.Uint64
	var relaxedBase atomic.Uint64
	var relaxedOverlay atomic.Uint64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := int(next.Add(1)-1) % len(fixture.Corpus)
			query := fixture.Corpus[idx]
			routes, stats := router.ComputeFromIndicesWithStats(query.SrcIdx, query.DstIdx, 1, wf)
			if len(routes) != 1 {
				b.Fatalf("%s: expected one route, got %d", query.Name, len(routes))
			}

			settledBase.Add(uint64(stats.SettledBaseNodes))
			settledOverlay.Add(uint64(stats.SettledOverlayNodes))
			relaxedBase.Add(uint64(stats.RelaxedBaseEdges))
			relaxedOverlay.Add(uint64(stats.RelaxedOverlayEdges))
		}
	})
	b.StopTimer()

	b.ReportMetric(float64(settledBase.Load())/float64(b.N), "settled_base/op")
	b.ReportMetric(float64(settledOverlay.Load())/float64(b.N), "settled_overlay/op")
	b.ReportMetric(float64(relaxedBase.Load())/float64(b.N), "relaxed_base/op")
	b.ReportMetric(float64(relaxedOverlay.Load())/float64(b.N), "relaxed_overlay/op")
}
