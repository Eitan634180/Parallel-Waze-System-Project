package benchmark_test

import (
	"math"
	"os"
	"testing"
)

const routeEpsilon = 0.001

func mustLoadFixture(tb testing.TB) *Fixture {
	tb.Helper()

	corpusName := os.Getenv("TEST_BENCH_CORPUS")
	if corpusName == "" {
		tb.Skip("TEST_BENCH_CORPUS is not set")
	}

	fixture, err := LoadFixture(corpusName)
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
