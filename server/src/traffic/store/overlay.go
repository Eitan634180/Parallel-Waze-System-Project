package store

import (
	"math"
	"sync/atomic"

	"nav-system/src/graph/model"
)

func (s *Store) InitOverlayWeights(edges []model.OverlayEdge) {
	data := &overlayData{
		weight: make([]atomic.Uint32, len(edges)),
	}
	for i, edge := range edges {
		data.weight[i].Store(math.Float32bits(edge.BaseWeight))
	}
	s.overlay.Store(data)
}

func (s *Store) OverlayWeight(edgeIdx uint32, fallback float32) float32 {
	return loadOverlayWeight(s.overlay.Load(), edgeIdx, fallback)
}

func (s *Store) applyOverlayUpdates(updates []overlayWeightUpdate) {
	data := s.overlay.Load()
	if data == nil {
		return
	}
	for _, update := range updates {
		if int(update.edgeIdx) >= len(data.weight) {
			continue
		}
		data.weight[update.edgeIdx].Store(math.Float32bits(update.weight))
	}
}
