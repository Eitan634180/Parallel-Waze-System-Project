package store

import (
	"context"
	"math"
	"time"

	"nav-system/src/traffic"
)

// Worker runs a background goroutine that periodically decays all dirty edge
// weights back toward their base value.
//
// It exits cleanly when ctx is cancelled.
func Worker(ctx context.Context, store *Store) {
	ticker := time.NewTicker(traffic.DecayInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			store.ApplyDecay(traffic.DecayFactor, traffic.DecayTolerance)
		}
	}
}

// ApplyDecay exponentially decays dirty-edge multipliers back toward 1.0.
func (s *Store) ApplyDecay(factor, tolerance float32) {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()

	newDirty := s.dirtyEdges[:0]
	data := s.base.Load()
	for _, id := range s.dirtyEdges {
		cur := math.Float32frombits(data.weight[id].Load())
		updated := 1.0 + (cur-1.0)*factor
		diff := updated - 1.0
		if diff < 0 {
			diff = -diff
		}

		if diff < tolerance {
			storeWeight(data, id, 1.0)
			s.isDirty[id] = false
			if data.density[id].Load() == 0 {
				s.isActive[id] = false
				s.prevWeight[id] = 1.0
				s.prevDensity[id] = 0
			}
		} else {
			storeWeight(data, id, updated)
			newDirty = append(newDirty, id)
		}

		if !s.isPending[id] {
			s.isPending[id] = true
			s.pendingEdges = append(s.pendingEdges, id)
		}
	}
	s.dirtyEdges = newDirty
}
