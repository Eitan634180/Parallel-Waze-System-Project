package store

import (
	"math"

	"nav-system/src/graph/model"
	"nav-system/src/traffic"
	"nav-system/src/utilities"
)

// Update weight using EWMA of observed/base drive time.
func (s *Store) RecordObservation(id model.EdgeID, observedSec, baseSec float32) {
	if baseSec <= 0 {
		return
	}
	s.recordObservedRatio(id, observedSec/baseSec, traffic.EwmaAlpha)
}

// Update weight using EWMA of speeds on the edge.
func (s *Store) RecordSpeedSample(id model.EdgeID, speedKmh, baseSec, distanceM float32) {
	if speedKmh <= 0 || baseSec <= 0 || distanceM <= 0 {
		return
	}
	observedSec := distanceM / (speedKmh / utilities.KilometersPerHourToMps)
	if observedSec <= 0 {
		return
	}
	s.recordObservedRatio(id, observedSec/baseSec, traffic.PartialSampleAlpha)
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
	data := s.base.Load()
	for _, edgeID := range s.dirtyEdges {
		if int(edgeID) < len(dst) {
			dst[edgeID] = loadWeight(data, edgeID)
		}
	}
	return dst
}

// EnterEdge increments the active-session count for an edge.
func (s *Store) EnterEdge(id model.EdgeID) {
	s.metaMu.Lock()
	data := s.ensureLocked(id)
	data.density[id].Add(1)
	if !s.isActive[id] {
		s.isActive[id] = true
		s.activityEdges = append(s.activityEdges, id)
	}
	s.metaMu.Unlock()
}

// LeaveEdge decrements the active-session count for an edge.
func (s *Store) LeaveEdge(id model.EdgeID) {
	s.metaMu.Lock()
	data := s.ensureLocked(id)
	d := data.density[id].Load() - 1
	if d <= 0 {
		data.density[id].Store(0)
	} else {
		data.density[id].Store(d)
	}
	if !s.isActive[id] {
		s.isActive[id] = true
		s.activityEdges = append(s.activityEdges, id)
	}
	s.metaMu.Unlock()
}
