package traffic

import (
	"math"
	"sync"

	"nav-system/internal/graph/builder"
)

const (
	// ewmaAlpha controls how quickly observed travel-time changes affect weight.
	ewmaAlpha = float32(0.15)

	// partialSampleAlpha is used for weaker in-progress speed samples from pings.
	partialSampleAlpha = float32(0.05)

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
// The zero value is NOT valid; use NewStore().
type Store struct {
	mu          sync.RWMutex
	weight      map[builder.EdgeID]float32  // observed EWMA multiplier (1.0 = free-flow)
	density     map[builder.EdgeID]int      // number of active sessions currently on edge
	dirty       map[builder.EdgeID]struct{} // edges whose multiplier != 1.0
	activity    map[builder.EdgeID]struct{} // edges whose density changed since last snapshot
	prev        map[builder.EdgeID]float32  // multiplier at last propagation snapshot
	prevDensity map[builder.EdgeID]int      // density at last propagation snapshot
}

// NewStore creates an empty Store.
func NewStore() *Store {
	return &Store{
		weight:      make(map[builder.EdgeID]float32),
		density:     make(map[builder.EdgeID]int),
		dirty:       make(map[builder.EdgeID]struct{}),
		activity:    make(map[builder.EdgeID]struct{}),
		prev:        make(map[builder.EdgeID]float32),
		prevDensity: make(map[builder.EdgeID]int),
	}
}

// EnterEdge increments the active-session count for an edge.
func (s *Store) EnterEdge(id builder.EdgeID) {
	s.mu.Lock()
	s.density[id]++
	s.activity[id] = struct{}{}
	s.mu.Unlock()
}

// LeaveEdge decrements the active-session count for an edge.
func (s *Store) LeaveEdge(id builder.EdgeID) {
	s.mu.Lock()
	d := s.density[id] - 1
	if d <= 0 {
		delete(s.density, id)
	} else {
		s.density[id] = d
	}
	s.activity[id] = struct{}{}
	s.mu.Unlock()
}

// Density returns the current number of active sessions on an edge.
func (s *Store) Density(id builder.EdgeID) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
	s.mu.Lock()
	cur, ok := s.weight[id]
	if !ok {
		cur = 1.0
	}
	updated := alpha*ratio + (1-alpha)*cur
	s.weight[id] = updated
	s.dirty[id] = struct{}{}
	s.mu.Unlock()
}

// LiveWeight returns the routing/ETA cost for an edge based on observed traffic only.
func (s *Store) LiveWeight(id builder.EdgeID, baseSec float32) float32 {
	s.mu.RLock()
	m, ok := s.weight[id]
	s.mu.RUnlock()
	if !ok {
		return baseSec
	}
	return baseSec * m
}

// Multiplier returns the raw multiplier for an edge (1.0 if not observed).
func (s *Store) Multiplier(id builder.EdgeID) float32 {
	s.mu.RLock()
	m, ok := s.weight[id]
	s.mu.RUnlock()
	if !ok {
		return 1.0
	}
	return m
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

// DirtySnapshot returns edges whose multiplier or density changed since the previous call.
// Safe to call concurrently with EnterEdge/LeaveEdge/RecordObservation.
func (s *Store) DirtySnapshot() []ChangedEdge {
	s.mu.Lock()
	defer s.mu.Unlock()

	edgeIDs := make(map[builder.EdgeID]struct{}, len(s.dirty)+len(s.activity))
	for id := range s.dirty {
		edgeIDs[id] = struct{}{}
	}
	for id := range s.activity {
		edgeIDs[id] = struct{}{}
	}
	var changed []ChangedEdge
	for id := range edgeIDs {
		cur := s.weight[id]
		prev := s.prev[id]
		delta := cur - prev
		if delta < 0 {
			delta = -delta
		}
		density := s.density[id]
		densityChanged := density != s.prevDensity[id]
		if delta >= SignificantShift || densityChanged {
			changed = append(changed, ChangedEdge{EdgeID: id, OldMultiplier: prev, NewMultiplier: cur})
			s.prev[id] = cur
			s.prevDensity[id] = density
		}
	}

	clear(s.activity)
	return changed
}

// ApplyDecay exponentially decays dirty-edge multipliers back toward 1.0.
func (s *Store) ApplyDecay(factor, tolerance float32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id := range s.dirty {
		cur := s.weight[id]
		updated := 1.0 + (cur-1.0)*factor
		diff := updated - 1.0
		if diff < 0 {
			diff = -diff
		}
		if diff < tolerance {
			delete(s.weight, id)
			delete(s.dirty, id)
			if s.density[id] == 0 {
				delete(s.activity, id)
				delete(s.prev, id)
				delete(s.prevDensity, id)
			}
		} else {
			s.weight[id] = updated
		}
	}
}
