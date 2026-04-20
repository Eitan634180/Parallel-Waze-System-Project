package traffic

import (
	"math"
	"sync"

	"nav-system/src/graph/builder"
)

const (
	// ewmaAlpha controls how quickly observed travel-time changes affect weight.
	ewmaAlpha = float32(0.15)

	// partialSampleAlpha is used for weaker in-progress speed samples from pings.
	partialSampleAlpha = float32(0.08)

	// CongestionThreshold marks edges that should be treated as congested.
	CongestionThreshold = float32(1.5)

	// SignificantShift is the minimum multiplier change that triggers a speed update.
	SignificantShift = float32(0.10)

	// Hint-speed parameters are only used for speed-update guidance.
	hintJamDensityVehPerKm = float32(120)
	hintMinSpeedRatio      = float32(0.02)
	hintMinEdgeLengthKm    = float32(0.02)
	hintAlpha              = 1.0
)

// Store holds the live traffic state for every edge that has been observed.
// The zero value is valid via lazy ensure allocations; use NewStore() for clarity.
type Store struct {
	mu          sync.RWMutex
	weight      []float32 // observed EWMA multiplier (1.0 = free-flow)
	density     []int     // number of active sessions currently on edge
	prev        []float32 // multiplier at last propagation snapshot
	prevDensity []int     // density at last propagation snapshot

	dirtyEdges    []builder.EdgeID // persistent: edges whose multiplier != 1.0 (for decay)
	isDirty       []bool
	pendingEdges  []builder.EdgeID // consumable: edges whose weights changed recently (for customization)
	isPending     []bool
	activityEdges []builder.EdgeID // edges whose density changed since last snapshot
	isActive      []bool

	snapshotDedup []bool
}

// NewStore creates an empty Store.
func NewStore() *Store {
	return &Store{}
}

func (s *Store) ensure(id builder.EdgeID) {
	if int(id) < len(s.weight) {
		return
	}

	newLen := int(id)*2 + 1024

	newWeight := make([]float32, newLen)
	for i := range newWeight {
		newWeight[i] = 1.0
	}
	copy(newWeight, s.weight)
	s.weight = newWeight

	newPrev := make([]float32, newLen)
	for i := range newPrev {
		newPrev[i] = 1.0
	}
	copy(newPrev, s.prev)
	s.prev = newPrev

	newDensity := make([]int, newLen)
	copy(newDensity, s.density)
	s.density = newDensity

	newPrevDensity := make([]int, newLen)
	copy(newPrevDensity, s.prevDensity)
	s.prevDensity = newPrevDensity

	newIsDirty := make([]bool, newLen)
	copy(newIsDirty, s.isDirty)
	s.isDirty = newIsDirty

	newIsPending := make([]bool, newLen)
	copy(newIsPending, s.isPending)
	s.isPending = newIsPending

	newIsActive := make([]bool, newLen)
	copy(newIsActive, s.isActive)
	s.isActive = newIsActive

	newSnapshotDedup := make([]bool, newLen)
	copy(newSnapshotDedup, s.snapshotDedup)
	s.snapshotDedup = newSnapshotDedup
}

// EnterEdge increments the active-session count for an edge.
func (s *Store) EnterEdge(id builder.EdgeID) {
	s.mu.Lock()
	s.ensure(id)
	s.density[id]++
	if !s.isActive[id] {
		s.isActive[id] = true
		s.activityEdges = append(s.activityEdges, id)
	}
	s.mu.Unlock()
}

// LeaveEdge decrements the active-session count for an edge.
func (s *Store) LeaveEdge(id builder.EdgeID) {
	s.mu.Lock()
	s.ensure(id)
	d := s.density[id] - 1
	if d <= 0 {
		s.density[id] = 0
	} else {
		s.density[id] = d
	}
	if !s.isActive[id] {
		s.isActive[id] = true
		s.activityEdges = append(s.activityEdges, id)
	}
	s.mu.Unlock()
}

// Density returns the current number of active sessions on an edge.
func (s *Store) Density(id builder.EdgeID) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if int(id) >= len(s.density) {
		return 0
	}
	return s.density[id]
}

// RecordObservation updates the edge multiplier using an EWMA of observed/base time.
func (s *Store) RecordObservation(id builder.EdgeID, observedSec, baseSec float32) {
	if baseSec <= 0 {
		return
	}
	s.recordObservedRatio(id, observedSec/baseSec, ewmaAlpha)
}

// RecordSpeedSample updates the observed multiplier from an in-progress speed sample.
func (s *Store) RecordSpeedSample(id builder.EdgeID, speedKmh, baseSec, distanceM float32) {
	if speedKmh <= 0 || baseSec <= 0 || distanceM <= 0 {
		return
	}
	observedSec := distanceM / (speedKmh / 3.6)
	if observedSec <= 0 {
		return
	}
	s.recordObservedRatio(id, observedSec/baseSec, partialSampleAlpha)
}

func (s *Store) recordObservedRatio(id builder.EdgeID, ratio, alpha float32) {
	if ratio <= 0 {
		return
	}
	if ratio < 1.0 {
		ratio = 1.0
	}
	s.mu.Lock()
	s.ensure(id)
	cur := s.weight[id]
	updated := alpha*ratio + (1-alpha)*cur
	if updated < 1.0 {
		updated = 1.0
	}
	if updated == cur {
		s.mu.Unlock()
		return
	}
	s.weight[id] = updated

	if !s.isDirty[id] {
		s.isDirty[id] = true
		s.dirtyEdges = append(s.dirtyEdges, id)
	}
	if !s.isPending[id] {
		s.isPending[id] = true
		s.pendingEdges = append(s.pendingEdges, id)
	}
	s.mu.Unlock()
}

// LiveWeight returns the routing/ETA cost for an edge based on observed traffic only.
func (s *Store) LiveWeight(id builder.EdgeID, baseSec float32) float32 {
	s.mu.RLock()
	if int(id) >= len(s.weight) {
		s.mu.RUnlock()
		return baseSec
	}
	m := s.weight[id]
	s.mu.RUnlock()
	return baseSec * m
}

// Multiplier returns the raw multiplier for an edge (1.0 if not observed).
func (s *Store) Multiplier(id builder.EdgeID) float32 {
	s.mu.RLock()
	if int(id) >= len(s.weight) {
		s.mu.RUnlock()
		return 1.0
	}
	m := s.weight[id]
	s.mu.RUnlock()
	return m
}

// SnapshotWeightMultipliers fills a dense multiplier slice indexed by EdgeID.
// Entries default to 1.0 when an edge has no observed override.
func (s *Store) SnapshotWeightMultipliers(dst []float32, edgeCount int) []float32 {
	if cap(dst) < edgeCount {
		dst = make([]float32, edgeCount)
	} else {
		dst = dst[:edgeCount]
	}

	for i := range dst {
		dst[i] = 1.0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, edgeID := range s.dirtyEdges {
		if int(edgeID) < len(dst) {
			dst[edgeID] = s.weight[edgeID]
		}
	}
	return dst
}

// RecommendedSpeedKmh returns a density-based speed hint for an edge in km/h.
func (s *Store) RecommendedSpeedKmh(id builder.EdgeID, baseKmh, distanceM float32) float32 {
	if baseKmh <= 0 {
		return baseKmh
	}
	density := s.Density(id)
	if density <= 1 {
		return baseKmh
	}

	edgeKm := distanceM / 1000
	if edgeKm < hintMinEdgeLengthKm {
		edgeKm = hintMinEdgeLengthKm
	}

	k := float32(density) / edgeKm
	if k >= hintJamDensityVehPerKm {
		return baseKmh * hintMinSpeedRatio
	}

	ratio := 1.0 - (k / hintJamDensityVehPerKm)
	if ratio < 0 {
		ratio = 0
	}

	minSpeed := baseKmh * hintMinSpeedRatio
	speed := minSpeed + (baseKmh-minSpeed)*float32(math.Pow(float64(ratio), hintAlpha))
	if speed < minSpeed {
		return minSpeed
	}
	if speed > baseKmh {
		return baseKmh
	}
	return speed
}

// IsCongested returns true if the edge's multiplier exceeds CongestionThreshold.
func (s *Store) IsCongested(id builder.EdgeID) bool {
	return s.Multiplier(id) > CongestionThreshold
}

// ChangedEdge describes an edge whose live state changed since the last snapshot.
type ChangedEdge struct {
	EdgeID        builder.EdgeID
	OldMultiplier float32
	NewMultiplier float32
}

func (s *Store) checkDedup(id builder.EdgeID, changed *[]ChangedEdge) {
	if s.snapshotDedup[id] {
		return
	}
	s.snapshotDedup[id] = true

	cur := s.weight[id]
	prev := s.prev[id]
	delta := cur - prev
	if delta < 0 {
		delta = -delta
	}

	density := s.density[id]
	densityChanged := density != s.prevDensity[id]

	if delta >= SignificantShift || densityChanged {
		*changed = append(*changed, ChangedEdge{EdgeID: id, OldMultiplier: prev, NewMultiplier: cur})
		s.prev[id] = cur
		s.prevDensity[id] = density
	}
}

// DirtySnapshot returns edges whose multiplier or density changed since the previous call.
// Safe to call concurrently with EnterEdge/LeaveEdge/RecordObservation.
func (s *Store) DirtySnapshot() []ChangedEdge {
	s.mu.Lock()
	defer s.mu.Unlock()

	var changed []ChangedEdge
	for _, id := range s.dirtyEdges {
		s.checkDedup(id, &changed)
	}
	for _, id := range s.activityEdges {
		s.checkDedup(id, &changed)
	}

	// Cleanup dedup state for the next call
	for _, id := range s.dirtyEdges {
		s.snapshotDedup[id] = false
	}
	for _, id := range s.activityEdges {
		s.snapshotDedup[id] = false
		s.isActive[id] = false
	}

	s.activityEdges = s.activityEdges[:0]

	return changed
}

// SwapPending atomically takes ownership of the pending set and replaces it
// with a fresh empty slice. Used by the customization loop to find edges whose weights changed.
func (s *Store) SwapPending() []builder.EdgeID {
	s.mu.Lock()
	pending := s.pendingEdges
	s.pendingEdges = nil
	for _, id := range pending {
		s.isPending[id] = false
	}
	s.mu.Unlock()
	return pending
}

// RefillPendingForBenchmarks artificially repopulates the pending queue with all
// currently dirty edges. This is strictly for synthetic benchmarks where ApplyDecay
// does not naturally run between customization cycles.
func (s *Store) RefillPendingForBenchmarks() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingEdges = s.pendingEdges[:0]
	for _, id := range s.dirtyEdges {
		if !s.isPending[id] {
			s.isPending[id] = true
			s.pendingEdges = append(s.pendingEdges, id)
		}
	}
}

// ApplyDecay exponentially decays dirty-edge multipliers back toward 1.0.
func (s *Store) ApplyDecay(factor, tolerance float32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	newDirty := s.dirtyEdges[:0]
	for _, id := range s.dirtyEdges {
		cur := s.weight[id]
		updated := 1.0 + (cur-1.0)*factor
		diff := updated - 1.0
		if diff < 0 {
			diff = -diff
		}

		if diff < tolerance {
			s.weight[id] = 1.0
			s.isDirty[id] = false
			if s.density[id] == 0 {
				s.isActive[id] = false
				s.prev[id] = 1.0
				s.prevDensity[id] = 0
			}
		} else {
			s.weight[id] = updated
			newDirty = append(newDirty, id)
		}

		if !s.isPending[id] {
			s.isPending[id] = true
			s.pendingEdges = append(s.pendingEdges, id)
		}
	}
	s.dirtyEdges = newDirty
}
