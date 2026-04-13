package correctness_test

import (
	"testing"

	"nav-system/test/testutil"
)

func TestSnapIndexMatchesBruteForceNearestNode(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "grid_city.json", 3)
	points := []struct {
		name string
		lat  float64
		lon  float64
	}{
		{name: "node-1", lat: 32.0000, lon: 34.0000},
		{name: "node-5", lat: 32.0010, lon: 34.0010},
		{name: "node-9", lat: 32.0020, lon: 34.0020},
		{name: "mid-top", lat: 32.0000, lon: 34.0014},
		{name: "mid-left", lat: 32.0015, lon: 34.0000},
		{name: "center", lat: 32.0011, lon: 34.0011},
	}

	for _, point := range points {
		got := fixture.Snap.Snap(point.lat, point.lon)
		want := testutil.BruteForceSnap(fixture.Graph, point.lat, point.lon)
		if got != want {
			t.Fatalf("%s: snap got %d want %d", point.name, got, want)
		}
	}
}
