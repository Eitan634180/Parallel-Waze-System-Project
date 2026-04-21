package tracker

import (
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/routing"
)

func (t *Tracker) recordEdgeObservations(events []EdgeObservation) {
	for _, event := range events {
		edgeID := model.EdgeID(event.EdgeID)
		if int(edgeID) >= len(t.Graph.Edges) || event.ObservedSec <= 0 {
			continue
		}

		edge := &t.Graph.Edges[edgeID]
		if edge.Weight <= 0 {
			continue
		}

		t.Store.RecordObservation(edgeID, event.ObservedSec, edge.Weight)
	}
}

func (t *Tracker) recordCurrentEdgeSpeedSample(edgeID *uint32, edgeAt, now time.Time, speedKmh float32) {
	if edgeID == nil || speedKmh <= 0 || edgeAt.IsZero() || now.Sub(edgeAt) < partialObservationMinEdgeAge {
		return
	}

	eid := model.EdgeID(*edgeID)
	if int(eid) >= len(t.Graph.Edges) {
		return
	}

	edge := &t.Graph.Edges[eid]
	if edge.Weight <= 0 || edge.DistanceM <= 0 {
		return
	}

	t.Store.RecordSpeedSample(eid, speedKmh, edge.Weight, edge.DistanceM)
}

func (t *Tracker) releaseTraversedEdges(route routing.Route, fromIdx, toIdx int) {
	for i := fromIdx; i < toIdx; i++ {
		if i < 0 || i >= len(route.Steps) {
			continue
		}

		edgeID := route.Steps[i].EdgeID
		if edgeID != nil {
			t.Store.LeaveEdge(model.EdgeID(*edgeID))
		}
	}
}
