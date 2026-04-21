package store

import (
	"math"

	"nav-system/src/graph/model"
	"nav-system/src/utilities"
)

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
func (s *Store) IsCongested(id model.EdgeID) bool {
	return s.Multiplier(id) > CongestionThreshold
}
