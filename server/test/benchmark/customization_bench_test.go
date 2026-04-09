package benchmark_test

import (
	"time"
	"testing"

	"nav-system/src/routing"
	"nav-system/src/traffic"
)

func BenchmarkOverlayCustomizationLive(b *testing.B) {
	fixture := mustLoadFixture(b, benchmarkTrafficName())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		traffic.CustomizeOverlayWeights(fixture.Graph, fixture.Store)
	}
}

func BenchmarkCustomizationAmortizedLive(b *testing.B) {
	fixture := mustLoadFixture(b, benchmarkTrafficName())
	router := routing.NewRouterWithMode(fixture.Graph, fixture.Snap, routing.RoutingModeHierarchical)
	liveWeight := weightFuncForFixture(fixture.Store)

	customizationStart := time.Now()
	traffic.CustomizeOverlayWeights(fixture.Graph, fixture.Store)
	customizationMs := float64(time.Since(customizationStart).Microseconds()) / 1000.0

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := fixture.Corpus[i%len(fixture.Corpus)]
		routes := router.ComputeFromIndices(query.SrcIdx, query.DstIdx, 1, liveWeight)
		if len(routes) != 1 {
			b.Fatalf("%s: expected one route, got %d", query.Name, len(routes))
		}
	}
	b.StopTimer()

	avgQueryMs := float64(b.Elapsed().Microseconds()) / 1000.0 / float64(b.N)
	b.ReportMetric(customizationMs+avgQueryMs*1, "amortized_1_ms")
	b.ReportMetric(customizationMs+avgQueryMs*10, "amortized_10_ms")
	b.ReportMetric(customizationMs+avgQueryMs*100, "amortized_100_ms")
	b.ReportMetric(customizationMs+avgQueryMs*1000, "amortized_1000_ms")
}
