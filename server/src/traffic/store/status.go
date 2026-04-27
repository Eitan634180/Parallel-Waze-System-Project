package store

import (
	"math"
	"nav-system/src/graph/model"
	"nav-system/src/traffic"
	"nav-system/src/utilities"
)

// Density returns the current number of active sessions on an edge.
func (s *Store) Density(id model.EdgeID) int {
	return int(loadDensity(s.base.Load(), id))
}

// Multiplier returns the raw multiplier for an edge (1.0 if not observed).
func (s *Store) Multiplier(id model.EdgeID) float32 {
	return loadWeight(s.base.Load(), id)
}

// LiveWeight returns the routing/ETA cost for an edge based on observed traffic only.
func (s *Store) LiveWeight(id model.EdgeID, baseSec float32) float32 {
	data := s.base.Load()
	if data == nil || int(id) >= len(data.weight) {
		return baseSec
	}
	return baseSec * math.Float32frombits(data.weight[id].Load())
}

// RecommendedSpeedKmh returns a density-based speed hint for an edge in km/h.
func (s *Store) RecommendedSpeedKmh(id model.EdgeID, baseKmh, distanceM float32) float32 {
	if baseKmh <= 0 {
		return baseKmh
	}
	density := s.Density(id)
	if density <= 1 {
		return baseKmh
	}

	edgeKm := distanceM / utilities.KilometersPerMeter
	if edgeKm < traffic.HintMinEdgeLengthKm {
		edgeKm = traffic.HintMinEdgeLengthKm
	}

	k := float32(density) / edgeKm
	if k >= traffic.HintJamDensityVehPerKm {
		return baseKmh * traffic.HintMinSpeedRatio
	}

	ratio := 1.0 - (k / traffic.HintJamDensityVehPerKm)
	if ratio < 0 {
		ratio = 0
	}

	minSpeed := baseKmh * traffic.HintMinSpeedRatio
	speed := minSpeed + (baseKmh-minSpeed)*float32(math.Pow(float64(ratio), traffic.HintAlpha))
	if speed < minSpeed {
		return minSpeed
	}
	if speed > baseKmh {
		return baseKmh
	}
	return speed
}

// IsCongested returns true if the edge's multiplier exceeds CongestionThreshold.
func (s *Store) IsCongested(id model.EdgeID) bool {
	return s.Multiplier(id) > traffic.CongestionThreshold
}
