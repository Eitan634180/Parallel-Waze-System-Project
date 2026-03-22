package traffic

import (
	"math"
	"sync"

	"nav-system/map/builder"
)

const (
	// ewmaAlpha is the smoothing factor for the Exponentially Weighted Moving Average.
	// Higher = more reactive to new observations; lower = smoother.
	ewmaAlpha = float32(0.15)

	// CongestionThreshold: edges with multiplier above this are considered congested.
	CongestionThreshold = float32(1.5)

	// SignificantShift is the minimum multiplier change that triggers a speed_update
	// broadcast to subscribed sessions.
	SignificantShift = float32(0.10)

	// Hint-speed model parameters. These are used only for speed_update guidance,
	// not for routing or ETA.
	hintJamDensityVehPerKm = float32(120)
	hintMinSpeedRatio      = float32(0.02)
	hintMinEdgeLengthKm    = float32(0.02)
	hintAlpha              = 1.0
)

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

// Store holds the live traffic state for every edge that has been observed.
// The zero value is NOT valid; use NewStore().
type Store struct {
	mu          sync.RWMutex
	weight      map[builder.EdgeID]float32  // effective multiplier (1.0 = free-flow)
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

// ---------------------------------------------------------------------------
// Density tracking
// ---------------------------------------------------------------------------

// EnterEdge increments the active-session count for an edge.
// Called when a session advances onto an edge.
func (s *Store) EnterEdge(id builder.EdgeID) {
	s.mu.Lock()
	s.density[id]++
	s.activity[id] = struct{}{}
	s.mu.Unlock()
}

// LeaveEdge decrements the active-session count for an edge.
// Called when a session leaves an edge.
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

// ---------------------------------------------------------------------------
// EWMA weight update
// ---------------------------------------------------------------------------

// RecordObservation updates the edge multiplier using an EWMA of
// (observedSec / baseSec). A ratio > 1 means the edge is slower than base.
func (s *Store) RecordObservation(id builder.EdgeID, observedSec, baseSec float32) {
	if baseSec <= 0 {
		return
	}
	ratio := observedSec / baseSec

	s.mu.Lock()
	cur, ok := s.weight[id]
	if !ok {
		cur = 1.0
	}
	updated := ewmaAlpha*ratio + (1-ewmaAlpha)*cur
	s.weight[id] = updated
	s.dirty[id] = struct{}{}
	s.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// EffectiveWeight returns base multiplied by the current traffic multiplier.
func (s *Store) EffectiveWeight(id builder.EdgeID, base float32) float32 {
	s.mu.RLock()
	m, ok := s.weight[id]
	s.mu.RUnlock()
	if !ok {
		return base
	}
	return base * m
}

// LiveWeight returns the routing/ETA cost for an edge after combining
// observation-based slowdown with density-based slowdown.
func (s *Store) LiveWeight(id builder.EdgeID, baseSec, baseKmh, distanceM float32) float32 {
	observed := s.EffectiveWeight(id, baseSec)
	if baseSec <= 0 || baseKmh <= 0 {
		return observed
	}

	recommended := s.RecommendedSpeedKmh(id, baseKmh, distanceM)
	if recommended <= 0 {
		return observed
	}

	densityBased := distanceM / (recommended / 3.6)
	if densityBased > observed {
		return densityBased
	}
	return observed
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

// RecommendedSpeedKmh returns a density-based hint speed for an edge in km/h.
// It uses a modified Greenshields-style speed-density relationship:
// v = v0 + (vf-v0) * (1 - k/kjam)^alpha
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

// ---------------------------------------------------------------------------
// Snapshot for background propagation worker
// ---------------------------------------------------------------------------

// ChangedEdge describes an edge whose multiplier has shifted significantly
// since the last snapshot.
type ChangedEdge struct {
	EdgeID        builder.EdgeID
	OldMultiplier float32
	NewMultiplier float32
}

// DirtySnapshot returns all dirty edges and flags those whose multiplier has
// shifted by more than SignificantShift since the previous call.
// It also updates the internal prev map for next comparison.
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

// ---------------------------------------------------------------------------
// Decay (called by decay.Worker)
// ---------------------------------------------------------------------------

// ApplyDecay exponentially decays all dirty-edge multipliers back toward 1.0.
// factor is the decay coefficient, e.g. 0.85.
// Edges that return within tolerance of 1.0 are cleaned up.
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
