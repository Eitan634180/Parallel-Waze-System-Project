package sessionruntime

import (
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/routing"
	"nav-system/src/routing/engine"
	routingmonitor "nav-system/src/routing/monitor"
	"nav-system/src/session"
	trafficstore "nav-system/src/traffic/store"
)

const partialObservationMinEdgeAge = 2 * time.Second

type EdgeObservation struct {
	EdgeID      uint32
	ObservedSec float32
}

type Runtime struct {
	Graph        *model.Graph
	Store        *trafficstore.Store
	Manager      *session.Manager
	Router       *routing.Router
	LiveWeights  func() routing.WeightFunc
	PrepareRoute func(routing.Route) routing.Route
}

func (rt *Runtime) CreateHeadless(route routing.Route, stepIdx int, lat, lon float64) *session.Session {
	sess := rt.Manager.CreateHeadless(route, stepIdx)
	sess.Mu.Lock()
	sess.LastLat = lat
	sess.LastLon = lon
	sess.Mu.Unlock()
	rt.InitializeEdge(sess)
	return sess
}

func (rt *Runtime) InitializeEdge(sess *session.Session) {
	sess.Mu.Lock()
	defer sess.Mu.Unlock()
	if sess.CurrentEdgeID == nil {
		sess.CurrentEdgeID = engine.CurrentEdgeForStep(sess.Route, sess.StepIdx)
	}
	if sess.CurrentEdgeID == nil {
		return
	}
	sess.CurrentEdgeAt = time.Now()
	rt.Store.EnterEdge(model.EdgeID(*sess.CurrentEdgeID))
}

func (rt *Runtime) Destroy(sess *session.Session) {
	sess.Mu.Lock()
	currentEdgeID := sess.CurrentEdgeID
	sess.CurrentEdgeID = nil
	sess.Mu.Unlock()
	if currentEdgeID != nil {
		rt.Store.LeaveEdge(model.EdgeID(*currentEdgeID))
	}
	rt.Manager.Delete(sess.ID)
}

func (rt *Runtime) Advance(sess *session.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []EdgeObservation) {
	now := time.Now()
	var advanceSessionID string
	var advanceRoute routing.Route
	advanceFrom := -1
	advanceTo := -1
	var currentEdgeID *uint32
	var currentEdgeAt time.Time

	sess.Mu.Lock()
	sess.LastPing = now

	prevIdx := sess.StepIdx
	if prevIdx < 0 {
		prevIdx = 0
	}

	newIdx := normalizeStepIndex(sess.Route, prevIdx, stepIdx)
	rt.recordEdgeObservations(edgeEvents)

	if newIdx > prevIdx {
		rt.releaseTraversedEdges(sess.Route, prevIdx, newIdx)
	}

	if newIdx > sess.StepIdx {
		advanceSessionID = sess.ID
		advanceRoute = sess.Route
		advanceFrom = sess.StepIdx
		advanceTo = newIdx
		sess.StepIdx = newIdx
	}

	nextEdgeID := engine.CurrentEdgeForStep(sess.Route, newIdx)
	if edgeChanged(sess.CurrentEdgeID, nextEdgeID) {
		if newIdx == prevIdx && sess.CurrentEdgeID != nil {
			rt.Store.LeaveEdge(model.EdgeID(*sess.CurrentEdgeID))
		}
		sess.CurrentEdgeID = nextEdgeID
		sess.CurrentEdgeAt = now
		if sess.CurrentEdgeID != nil {
			rt.Store.EnterEdge(model.EdgeID(*sess.CurrentEdgeID))
		}
	}
	sess.LastLat = lat
	sess.LastLon = lon
	currentEdgeID = sess.CurrentEdgeID
	currentEdgeAt = sess.CurrentEdgeAt
	sess.Mu.Unlock()

	if advanceFrom >= 0 {
		rt.Manager.AdvanceStep(advanceSessionID, advanceRoute, advanceFrom, advanceTo)
	}

	rt.recordCurrentEdgeSpeedSample(currentEdgeID, currentEdgeAt, now, speedKmh)

	routingmonitor.Check(
		sess,
		lat, lon,
		rt.Graph,
		rt.Store,
		rt.Manager,
		rt.Router,
		rt.LiveWeights(),
		rt.PrepareRoute,
	)
	debug := routingmonitor.DebugSnapshot(sess, speedKmh)
	_ = sess.Send(session.OutMsg{Type: "debug_update", Debug: &debug})
}

func (rt *Runtime) SendInitialSpeedUpdates(sess *session.Session) {
	for _, edgeID32 := range sess.RemainingEdges() {
		eid := model.EdgeID(edgeID32)
		if int(eid) >= len(rt.Graph.Edges) {
			continue
		}
		baseKmh := rt.Graph.Edges[eid].SpeedKmh
		if baseKmh <= 0 {
			continue
		}
		recSpeed := rt.Store.RecommendedSpeedKmh(eid, baseKmh, rt.Graph.Edges[eid].DistanceM)
		if recSpeed == baseKmh && rt.Store.Density(eid) == 0 {
			continue
		}
		edgeIDVal := uint32(eid)
		recSpeedVal := recSpeed
		_ = sess.Send(session.OutMsg{
			Type:                "speed_update",
			EdgeID:              &edgeIDVal,
			RecommendedSpeedKmh: &recSpeedVal,
		})
	}
}

func (rt *Runtime) recordEdgeObservations(events []EdgeObservation) {
	for _, event := range events {
		edgeID := model.EdgeID(event.EdgeID)
		if int(edgeID) >= len(rt.Graph.Edges) || event.ObservedSec <= 0 {
			continue
		}

		edge := &rt.Graph.Edges[edgeID]
		if edge.Weight <= 0 {
			continue
		}

		rt.Store.RecordObservation(edgeID, event.ObservedSec, edge.Weight)
	}
}

func (rt *Runtime) recordCurrentEdgeSpeedSample(edgeID *uint32, edgeAt, now time.Time, speedKmh float32) {
	if edgeID == nil || speedKmh <= 0 || edgeAt.IsZero() || now.Sub(edgeAt) < partialObservationMinEdgeAge {
		return
	}

	eid := model.EdgeID(*edgeID)
	if int(eid) >= len(rt.Graph.Edges) {
		return
	}

	edge := &rt.Graph.Edges[eid]
	if edge.Weight <= 0 || edge.DistanceM <= 0 {
		return
	}

	rt.Store.RecordSpeedSample(eid, speedKmh, edge.Weight, edge.DistanceM)
}

func (rt *Runtime) releaseTraversedEdges(route routing.Route, fromIdx, toIdx int) {
	for i := fromIdx; i < toIdx; i++ {
		if i < 0 || i >= len(route.Steps) {
			continue
		}

		edgeID := route.Steps[i].EdgeID
		if edgeID != nil {
			rt.Store.LeaveEdge(model.EdgeID(*edgeID))
		}
	}
}

func normalizeStepIndex(route routing.Route, previousIndex, reportedIndex int) int {
	if reportedIndex < previousIndex {
		reportedIndex = previousIndex
	}
	if reportedIndex >= len(route.Steps) {
		reportedIndex = len(route.Steps) - 1
	}
	return reportedIndex
}

func edgeChanged(current, next *uint32) bool {
	switch {
	case current == nil && next == nil:
		return false
	case current == nil || next == nil:
		return true
	default:
		return *current != *next
	}
}
