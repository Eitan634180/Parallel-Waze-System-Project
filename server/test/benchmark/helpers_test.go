package benchmark_test

import (
	"math"
	"os"
	"testing"
)

const routeEpsilon = 0.001

func benchmarkCorpusName() string {
	if name := os.Getenv("BENCH_CORPUS"); name != "" {
		return name
	}
	return "israel-and-palestine-bench.json"
}

func mustLoadFixture(tb testing.TB) *Fixture {
	tb.Helper()

	fixture, err := LoadFixture(benchmarkCorpusName())
	if err != nil {
		tb.Fatalf("LoadFixture: %v", err)
	}
	return fixture
}

func assertApproxRouteCost(t testing.TB, got, want float32, label string) {
	t.Helper()
	if math.Abs(float64(got-want)) > routeEpsilon {
		t.Fatalf("%s mismatch: got=%.6f want=%.6f", label, got, want)
	}
}
