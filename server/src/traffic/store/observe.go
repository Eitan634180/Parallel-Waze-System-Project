package store

import (
	"math"

	"nav-system/src/graph/model"
	"nav-system/src/utilities"
)

// RecordObservation updates the edge multiplier using an EWMA of observed/base time.
func (s *Store) RecordObservation(id model.EdgeID, observedSec, baseSec float32) {
	if baseSec <= 0 {
		return
	}
	s.recordObservedRatio(id, observedSec/baseSec, ewmaAlpha)
}

// RecordSpeedSample updates the observed multiplier from an in-progress speed sample.
func (s *Store) RecordSpeedSample(id model.EdgeID, speedKmh, baseSec, distanceM float32) {
	if speedKmh <= 0 || baseSec <= 0 || distanceM <= 0 {
		return
	}
	observedSec := distanceM / (speedKmh / utilities.KilometersPerHourToMps)
	if observedSec <= 0 {
		return
	}
	s.recordObservedRatio(id, observedSec/baseSec, partialSampleAlpha)
}

func (s *Store) recordObservedRatio(id model.EdgeID, ratio, alpha float32) {
	if ratio <= 0 {
		return
	}
	if ratio < 1.0 {
		ratio = 1.0
	}
	s.metaMu.Lock()
	data := s.ensureLocked(id)
	cur := math.Float32frombits(data.weight[id].Load())
	updated := alpha*ratio + (1-alpha)*cur
	if updated < 1.0 {
		updated = 1.0
	}
	if updated == cur {
		s.metaMu.Unlock()
		return
	}
	storeWeight(data, id, updated)

	if !s.isDirty[id] {
		s.isDirty[id] = true
		s.dirtyEdges = append(s.dirtyEdges, id)
	}
	if !s.isPending[id] {
		s.isPending[id] = true
		s.pendingEdges = append(s.pendingEdges, id)
	}
	s.metaMu.Unlock()
}

// LiveWeight returns the routing/ETA cost for an edge based on observed traffic only.
func (s *Store) LiveWeight(id model.EdgeID, baseSec float32) float32 {
	data := s.data.Load()
	if data == nil || int(id) >= len(data.weight) {
		return baseSec
	}
	return baseSec * math.Float32frombits(data.weight[id].Load())
}

// Multiplier returns the raw multiplier for an edge (1.0 if not observed).
func (s *Store) Multiplier(id model.EdgeID) float32 {
	return loadWeight(s.data.Load(), id)
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

	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	data := s.data.Load()
	for _, edgeID := range s.dirtyEdges {
		if int(edgeID) < len(dst) {
			dst[edgeID] = loadWeight(data, edgeID)
		}
	}
	return dst
}
