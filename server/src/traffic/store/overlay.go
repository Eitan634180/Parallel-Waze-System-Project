package store

import (
	"math"
	"sync/atomic"

	"nav-system/src/graph/model"
)

func (s *Store) InitOverlayWeights(g *model.Graph) {

	shortcutCount := 0
	for i := range g.Overlay.Edges {
		if !g.Overlay.Edges[i].IsCrossCell {
			shortcutCount++
		}
	}

	data := &shortcutStore{
		weight: make([]atomic.Uint32, shortcutCount),
	}
	for i := range g.Overlay.Edges {
		e := &g.Overlay.Edges[i]
		if !e.IsCrossCell {
			data.weight[e.StoreIdx].Store(math.Float32bits(e.BaseWeight))
		}
	}
	s.shortcut.Store(data)
}

// OverlayWeight returns the live weight for an overlay edge.
// Cross-cell edges derive their weight from the base traffic store.
// Shortcut edges read from the dedicated shortcut weight array.
func (s *Store) OverlayWeight(edgeIdx uint32, overlayEdge *model.OverlayEdge) float32 {
	if overlayEdge.IsCrossCell {
		data := s.base.Load()
		return overlayEdge.BaseWeight * loadWeight(data, overlayEdge.StoreIdx)
	}
	return loadOverlayWeight(s.shortcut.Load(), overlayEdge.StoreIdx, overlayEdge.BaseWeight)
}

func (s *Store) applyOverlayUpdates(updates []overlayWeightUpdate) {
	data := s.shortcut.Load()
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
