package propagation

import (
	"nav-system/src/graph/model"
	trafficstore "nav-system/src/traffic/store"
)

// SpeedUpdate represents a recommended-speed recalculation for one edge.
type SpeedUpdate struct {
	EdgeID              model.EdgeID
	RecommendedSpeedKmh float32
}

// ImprovedEdges filters snapshot changes down to the edges whose live multiplier
// improved enough to justify a better-route check.
func ImprovedEdges(changed []trafficstore.ChangedEdge) []trafficstore.ChangedEdge {
	improved := make([]trafficstore.ChangedEdge, 0, len(changed))
	for _, edge := range changed {
		if edge.OldMultiplier-edge.NewMultiplier >= trafficstore.SignificantShift {
			improved = append(improved, edge)
		}
	}
	return improved
}

// RecommendedSpeedUpdates converts changed edges into speed-update payloads for
// edges that have enough metadata to emit guidance.
func RecommendedSpeedUpdates(g *model.Graph, store *trafficstore.Store, changed []trafficstore.ChangedEdge) []SpeedUpdate {
	updates := make([]SpeedUpdate, 0, len(changed))
	for _, edgeChange := range changed {
		if int(edgeChange.EdgeID) >= len(g.Edges) {
			continue
		}

		edge := &g.Edges[edgeChange.EdgeID]
		if edge.SpeedKmh <= 0 {
			continue
		}

		updates = append(updates, SpeedUpdate{
			EdgeID:              edgeChange.EdgeID,
			RecommendedSpeedKmh: store.RecommendedSpeedKmh(edgeChange.EdgeID, edge.SpeedKmh, edge.DistanceM),
		})
	}
	return updates
}
