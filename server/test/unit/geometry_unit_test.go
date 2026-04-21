package unit_test

import (
	"math"
	"testing"

	"nav-system/src/core/utilities"
)

// ──────────────────────────────────────────────
// Project / DistanceSquared
// ──────────────────────────────────────────────

func TestProjectIsConsistentForSameCoord(t *testing.T) {
	x1, y1 := utilities.Project(32.0, 34.0)
	x2, y2 := utilities.Project(32.0, 34.0)
	if x1 != x2 || y1 != y2 {
		t.Fatal("Project must return identical results for the same input")
	}
}

func TestProjectLatIncreasesY(t *testing.T) {
	_, y1 := utilities.Project(32.0, 34.0)
	_, y2 := utilities.Project(33.0, 34.0)
	if y2 <= y1 {
		t.Fatalf("higher latitude should produce larger y: y(32)=%.2f y(33)=%.2f", y1, y2)
	}
}

func TestProjectLonIncreasesX(t *testing.T) {
	x1, _ := utilities.Project(32.0, 34.0)
	x2, _ := utilities.Project(32.0, 35.0)
	if x2 <= x1 {
		t.Fatalf("higher longitude should produce larger x: x(34)=%.2f x(35)=%.2f", x1, x2)
	}
}

// ──────────────────────────────────────────────
// HaversineM
// ──────────────────────────────────────────────

func TestHaversineMIdenticalPointsIsZero(t *testing.T) {
	d := utilities.HaversineM(32.0, 34.0, 32.0, 34.0)
	if d != 0 {
		t.Fatalf("haversine of identical coords should be 0, got %.6f", d)
	}
}

func TestHaversineMIsSymmetric(t *testing.T) {
	d1 := utilities.HaversineM(32.0, 34.0, 33.0, 35.0)
	d2 := utilities.HaversineM(33.0, 35.0, 32.0, 34.0)
	if math.Abs(d1-d2) > 0.001 {
		t.Fatalf("haversine should be symmetric: d1=%.6f d2=%.6f", d1, d2)
	}
}

func TestHaversineMOneDegreeLatitudeApprox111Km(t *testing.T) {
	// One degree of latitude ≈ 111 km.
	d := utilities.HaversineM(0.0, 0.0, 1.0, 0.0)
	const wantM = 111319.0
	const toleranceM = 1000.0
	if math.Abs(d-wantM) > toleranceM {
		t.Fatalf("one degree latitude should be ~%.0f m, got %.0f m", wantM, d)
	}
}

// ──────────────────────────────────────────────
// DistanceSquared / Distance
// ──────────────────────────────────────────────

func TestDistanceSquaredIdentical(t *testing.T) {
	if d := utilities.DistanceSquared(1, 2, 1, 2); d != 0 {
		t.Fatalf("DistanceSquared of identical points should be 0, got %.6f", d)
	}
}

func TestDistanceSquared3_4_5Triangle(t *testing.T) {
	// 3-4-5 right triangle.
	got := utilities.DistanceSquared(0, 0, 3, 4)
	if math.Abs(float64(got-25.0)) > 0.001 {
		t.Fatalf("DistanceSquared(0,0,3,4) = %.6f want 25", got)
	}
}

// ──────────────────────────────────────────────
// DistancePointToSegment
// ──────────────────────────────────────────────

func TestDistancePointToSegmentOnSegmentIsZero(t *testing.T) {
	// Midpoint of segment (0,0)–(0,10) is (0,5).
	d := utilities.DistancePointToSegment(0, 5, 0, 0, 0, 10)
	if math.Abs(float64(d)) > 0.001 {
		t.Fatalf("point on segment should have distance 0, got %.6f", d)
	}
}

func TestDistancePointToSegmentPerpendicularProjection(t *testing.T) {
	// Point (3,5) to segment (0,5)–(10,5): nearest point is (3,5) → distance 0.
	d := utilities.DistancePointToSegment(3, 5, 0, 5, 10, 5)
	if math.Abs(float64(d)) > 0.001 {
		t.Fatalf("perpendicular projection on segment: got %.6f want 0", d)
	}
}

func TestDistancePointToSegmentBeyondEndClamps(t *testing.T) {
	// Point (15,0) to segment (0,0)–(10,0): nearest is endpoint (10,0), distance = 5.
	d := utilities.DistancePointToSegment(15, 0, 0, 0, 10, 0)
	if math.Abs(float64(d-5.0)) > 0.001 {
		t.Fatalf("point beyond segment end: got %.6f want 5.0", d)
	}
}

func TestDistancePointToSegmentZeroLengthSegment(t *testing.T) {
	// Zero-length segment should degenerate to point distance.
	d := utilities.DistancePointToSegment(3, 4, 0, 0, 0, 0)
	if math.Abs(float64(d-5.0)) > 0.001 {
		t.Fatalf("zero-length segment: got %.6f want 5.0 (the 3-4-5 distance)", d)
	}
}

// ──────────────────────────────────────────────
// Heap – additional edge cases
// ──────────────────────────────────────────────

func TestHeapMaxOrderWithInvertedComparator(t *testing.T) {
	h := utilities.NewHeap(func(a, b int) bool { return a > b }) // max-heap
	for _, v := range []int{3, 1, 4, 1, 5} {
		h.Push(v)
	}
	prev := h.Pop()
	for h.Len() > 0 {
		cur := h.Pop()
		if cur > prev {
			t.Fatalf("max-heap ordering violated: %d > %d", cur, prev)
		}
		prev = cur
	}
}

func TestHeapSingleElementRoundtrip(t *testing.T) {
	h := utilities.NewHeap(func(a, b int) bool { return a < b })
	h.Push(42)
	if got := h.Pop(); got != 42 {
		t.Fatalf("single-element heap pop: got %d want 42", got)
	}
	if h.Len() != 0 {
		t.Fatal("heap should be empty after popping its only element")
	}
}
