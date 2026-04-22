package unit_test

import (
	"math"
	"testing"

	"nav-system/src/graph/model"
	trafficstore "nav-system/src/traffic/store"
)

// ──────────────────────────────────────────────
// LiveWeight / RecordObservation
// ──────────────────────────────────────────────

func TestTrafficStoreLiveWeightReturnsBaseWhenUnobserved(t *testing.T) {
	store := trafficstore.NewStore()
	const base = float32(10.0)
	got := store.LiveWeight(model.EdgeID(1), base)
	if got != base {
		t.Fatalf("LiveWeight unobserved: got %.4f want %.4f", got, base)
	}
}

func TestTrafficStoreWithCapacitySupportsInRangeEdgesWithoutWarmup(t *testing.T) {
	store := trafficstore.NewStoreWithCapacity(4)
	const edgeID = model.EdgeID(3)
	const base = float32(10.0)

	if got := store.LiveWeight(edgeID, base); got != base {
		t.Fatalf("LiveWeight on pre-sized edge: got %.4f want %.4f", got, base)
	}

	store.RecordObservation(edgeID, base*2, base)
	if got := store.Multiplier(edgeID); got <= 1.0 {
		t.Fatalf("expected pre-sized edge multiplier to update, got %.4f", got)
	}
}

func TestTrafficStoreRecordObservationAppliesEWMA(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(42)
	const base = float32(10.0)
	const observed = float32(20.0) // 2× base → ratio 2.0

	store.RecordObservation(edgeID, observed, base)

	// After one update from 1.0: α·ratio + (1-α)·1.0 = 0.15·2.0 + 0.85·1.0 = 1.15
	const expectedMultiplier = float32(1.15)
	got := store.Multiplier(edgeID)
	if math.Abs(float64(got-expectedMultiplier)) > 0.001 {
		t.Fatalf("multiplier after observation: got %.6f want %.6f", got, expectedMultiplier)
	}

	// LiveWeight must equal base * actual multiplier (use what the store computed).
	wantLive := base * got
	gotLive := store.LiveWeight(edgeID, base)
	if math.Abs(float64(gotLive-wantLive)) > 0.001 {
		t.Fatalf("LiveWeight mismatch: got %.6f want %.6f", gotLive, wantLive)
	}
}

func TestTrafficStoreRecordObservationIgnoresNonPositiveBase(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(7)
	store.RecordObservation(edgeID, 10, 0)
	store.RecordObservation(edgeID, 10, -1)
	// Neither call should register an entry.
	if store.Multiplier(edgeID) != 1.0 {
		t.Fatalf("expected multiplier 1.0 for invalid observations, got %.4f", store.Multiplier(edgeID))
	}
}

func TestTrafficStoreRecordSpeedSampleDoesNotDropBelowFreeFlow(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(8)

	store.RecordSpeedSample(edgeID, 120, 10, 100)

	if got := store.Multiplier(edgeID); got != 1.0 {
		t.Fatalf("expected multiplier floor at 1.0, got %.4f", got)
	}
	for _, changed := range store.DirtySnapshot() {
		if changed.EdgeID == edgeID {
			t.Fatalf("free-flow sample should not produce a dirty snapshot entry: %+v", changed)
		}
	}
}

func TestTrafficStoreMultipleObservationsConverge(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(1)
	const base = float32(10.0)
	const congested = float32(30.0) // ratio 3.0

	// Apply many identical observations and expect the multiplier to settle near 3.0.
	for i := 0; i < 100; i++ {
		store.RecordObservation(edgeID, congested, base)
	}
	m := store.Multiplier(edgeID)
	if math.Abs(float64(m-3.0)) > 0.01 {
		t.Fatalf("multiplier should converge to 3.0, got %.6f", m)
	}
}

// ──────────────────────────────────────────────
// IsCongested
// ──────────────────────────────────────────────

func TestTrafficStoreIsCongestedReturnsFalseWhenUnobserved(t *testing.T) {
	store := trafficstore.NewStore()
	if store.IsCongested(model.EdgeID(99)) {
		t.Fatal("unobserved edge should not be congested")
	}
}

func TestTrafficStoreIsCongestedReturnsTrueAboveThreshold(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(5)
	const base = float32(10.0)

	// Force multiplier above CongestionThreshold (1.5) by saturating the EWMA.
	for i := 0; i < 200; i++ {
		store.RecordObservation(edgeID, base*10.0, base)
	}
	if !store.IsCongested(edgeID) {
		t.Fatalf("edge with multiplier %.2f should be congested", store.Multiplier(edgeID))
	}
}

// ──────────────────────────────────────────────
// ApplyDecay
// ──────────────────────────────────────────────

func TestTrafficStoreApplyDecayReducesMultiplierTowardOne(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(10)
	const base = float32(10.0)

	// Push multiplier to ~2.0.
	for i := 0; i < 60; i++ {
		store.RecordObservation(edgeID, base*2.0, base)
	}
	before := store.Multiplier(edgeID)
	if before <= 1.0 {
		t.Fatalf("expected multiplier > 1.0 after heavy observations, got %.4f", before)
	}

	// Apply one decay step (factor=0.5, tolerance=0.05).
	store.ApplyDecay(0.5, 0.05)

	after := store.Multiplier(edgeID)
	if after >= before {
		t.Fatalf("decay should reduce multiplier: before=%.4f after=%.4f", before, after)
	}
}

func TestTrafficStoreApplyDecayEventuallyRemovesDirtyEdge(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(3)
	const base = float32(5.0)

	store.RecordObservation(edgeID, base*1.5, base) // one small push

	// Decay with large factor and tight tolerance so it converges quickly.
	for i := 0; i < 50; i++ {
		store.ApplyDecay(0.5, 0.05)
	}

	// After full decay the multiplier should be back at the default (1.0).
	if m := store.Multiplier(edgeID); math.Abs(float64(m-1.0)) > 0.05 {
		t.Fatalf("multiplier should return near 1.0 after full decay, got %.4f", m)
	}
}

// ──────────────────────────────────────────────
// DirtySnapshot
// ──────────────────────────────────────────────

func TestTrafficStoreDirtySnapshotReturnsChangedEdges(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(20)
	const base = float32(10.0)

	// First snapshot – nothing has changed yet.
	first := store.DirtySnapshot()
	for _, ce := range first {
		if ce.EdgeID == edgeID {
			t.Fatal("edge should not appear in first snapshot before any observation")
		}
	}

	// Record a significant observation.
	for i := 0; i < 10; i++ {
		store.RecordObservation(edgeID, base*5.0, base)
	}
	second := store.DirtySnapshot()
	found := false
	for _, ce := range second {
		if ce.EdgeID == edgeID {
			found = true
			if ce.NewMultiplier <= ce.OldMultiplier && ce.NewMultiplier <= 1.0 {
				t.Fatalf("new multiplier should be elevated, got old=%.4f new=%.4f", ce.OldMultiplier, ce.NewMultiplier)
			}
		}
	}
	if !found {
		t.Fatal("dirty snapshot should contain the observed edge")
	}
}

func TestTrafficStoreDirtySnapshotClearsActivityAfterCall(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(30)

	store.EnterEdge(edgeID)
	first := store.DirtySnapshot()
	found := false
	for _, ce := range first {
		if ce.EdgeID == edgeID {
			found = true
		}
	}
	if !found {
		t.Fatal("EnterEdge should mark edge as dirty activity")
	}

	// Second snapshot without further changes should NOT return the same edge.
	second := store.DirtySnapshot()
	for _, ce := range second {
		if ce.EdgeID == edgeID {
			t.Fatal("activity should be cleared after first snapshot; edge should not re-appear")
		}
	}
}

// ──────────────────────────────────────────────
// EnterEdge / LeaveEdge / Density
// ──────────────────────────────────────────────

func TestTrafficStoreDensityTracksEnterLeave(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(50)

	if got := store.Density(edgeID); got != 0 {
		t.Fatalf("initial density should be 0, got %d", got)
	}

	store.EnterEdge(edgeID)
	store.EnterEdge(edgeID)
	if got := store.Density(edgeID); got != 2 {
		t.Fatalf("density after 2 enters should be 2, got %d", got)
	}

	store.LeaveEdge(edgeID)
	if got := store.Density(edgeID); got != 1 {
		t.Fatalf("density after 1 leave should be 1, got %d", got)
	}

	store.LeaveEdge(edgeID)
	if got := store.Density(edgeID); got != 0 {
		t.Fatalf("density should return to 0, got %d", got)
	}
}

func TestTrafficStoreLeaveEdgeBelowZeroClipsToZero(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(99)

	// LeaveEdge on an edge with no sessions should not go negative.
	store.LeaveEdge(edgeID)
	if got := store.Density(edgeID); got != 0 {
		t.Fatalf("density should be 0 after leave with no enters, got %d", got)
	}
}

// ──────────────────────────────────────────────
// RecommendedSpeedKmh
// ──────────────────────────────────────────────

func TestTrafficStoreRecommendedSpeedReturnBaseWithOneCar(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(60)
	const base = float32(90.0)
	const dist = float32(500.0)

	store.EnterEdge(edgeID)
	got := store.RecommendedSpeedKmh(edgeID, base, dist)
	if got != base {
		t.Fatalf("with only 1 car speed should equal base %.1f, got %.1f", base, got)
	}
}

func TestTrafficStoreRecommendedSpeedDropsWithHighDensity(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(61)
	const base = float32(90.0)
	const dist = float32(500.0)

	// Jam: 50 cars on a 0.5 km segment → 100 veh/km, close to jam density.
	for i := 0; i < 50; i++ {
		store.EnterEdge(edgeID)
	}

	got := store.RecommendedSpeedKmh(edgeID, base, dist)
	if got >= base {
		t.Fatalf("high-density speed should be below base %.1f, got %.1f", base, got)
	}
	if got < 0 {
		t.Fatalf("recommended speed should never be negative, got %.1f", got)
	}
}

func TestTrafficStoreRecommendedSpeedZeroBaseReturnsZero(t *testing.T) {
	store := trafficstore.NewStore()
	const edgeID = model.EdgeID(62)
	for i := 0; i < 10; i++ {
		store.EnterEdge(edgeID)
	}
	got := store.RecommendedSpeedKmh(edgeID, 0, 500)
	if got != 0 {
		t.Fatalf("zero base speed should yield zero, got %.1f", got)
	}
}
