package benchmark_test

import (
	"math"
	"os"
	"testing"

	"nav-system/src/graph/builder"
	"nav-system/src/routing"
	"nav-system/src/traffic"
	"nav-system/test/benchutil"
)

const routeEpsilon = 0.001

func benchmarkCorpusName() string {
	if name := os.Getenv("BENCH_CORPUS"); name != "" {
		return name
	}
	return "israel-and-palestine-bench.json"
}

func benchmarkTrafficName() string {
	if name := os.Getenv("BENCH_TRAFFIC"); name != "" {
		return name
	}
	switch benchmarkCorpusName() {
	case "great-britain-bench.json":
		return "great-britain-live.json"
	default:
		return "israel-and-palestine-live.json"
	}
}

func mustLoadFixture(tb testing.TB, trafficName string) *benchutil.Fixture {
	tb.Helper()

	fixture, err := benchutil.LoadFixture(benchmarkCorpusName(), trafficName)
	if err != nil {
		tb.Fatalf("LoadFixture: %v", err)
	}
	return fixture
}

func weightFuncForFixture(store *traffic.Store) routing.WeightFunc {
	if store == nil {
		return routing.BaseWeight
	}
	return func(edge *builder.Edge) float32 {
		return store.LiveWeight(edge.ID, edge.Weight)
	}
}

func assertApproxRouteCost(t testing.TB, got, want float32, label string) {
	t.Helper()
	if math.Abs(float64(got-want)) > routeEpsilon {
		t.Fatalf("%s mismatch: got=%.6f want=%.6f", label, got, want)
	}
}
