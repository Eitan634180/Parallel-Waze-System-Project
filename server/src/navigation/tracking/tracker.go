package tracking

import (
	"time"

	"nav-system/src/graph/model"
	navigationanalysis "nav-system/src/navigation/analysis"
	navigationsessions "nav-system/src/navigation/sessions"
	"nav-system/src/routing"
	"nav-system/src/routing/engine"
	trafficstore "nav-system/src/traffic/store"
)

type EdgeObservation struct {
	EdgeID      uint32
	ObservedSec float32
}

type Tracker struct {
	Graph        *model.Graph
	Store        *trafficstore.Store
	Manager      *navigationsessions.Manager
	Router       *routing.Router
	LiveWeights  func() routing.WeightFunc
	PrepareRoute func(routing.Route) routing.Route
}

func (t *Tracker) CreateHeadless(route routing.Route, stepIdx int, lat, lon float64) *navigationsessions.Session {
	sess := t.Manager.CreateHeadless(route, stepIdx)
	sess.Mu.Lock()
	sess.LastLat = lat
	sess.LastLon = lon
	sess.Mu.Unlock()
	t.InitializeEdge(sess)
	return sess
}

func (t *Tracker) InitializeEdge(sess *navigationsessions.Session) {
	sess.Mu.Lock()
	defer sess.Mu.Unlock()
	if sess.CurrentEdgeID == nil {
		sess.CurrentEdgeID = engine.CurrentEdgeForStep(sess.Route, sess.StepIdx)
	}
	if sess.CurrentEdgeID == nil {
		return
	}
	sess.CurrentEdgeAt = time.Now()
	t.Store.EnterEdge(model.EdgeID(*sess.CurrentEdgeID))
}

func (t *Tracker) Destroy(sess *navigationsessions.Session) {
	sess.Mu.Lock()
	currentEdgeID := sess.CurrentEdgeID
	sess.CurrentEdgeID = nil
	sess.Mu.Unlock()
	if currentEdgeID != nil {
		t.Store.LeaveEdge(model.EdgeID(*currentEdgeID))
	}
	t.Manager.Delete(sess.ID)
}

func (t *Tracker) Advance(sess *navigationsessions.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []EdgeObservation) {
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
	t.recordEdgeObservations(edgeEvents)

	if newIdx > prevIdx {
		t.releaseTraversedEdges(sess.Route, prevIdx, newIdx)
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
			t.Store.LeaveEdge(model.EdgeID(*sess.CurrentEdgeID))
		}
		sess.CurrentEdgeID = nextEdgeID
		sess.CurrentEdgeAt = now
		if sess.CurrentEdgeID != nil {
			t.Store.EnterEdge(model.EdgeID(*sess.CurrentEdgeID))
		}
	}
	sess.LastLat = lat
	sess.LastLon = lon
	currentEdgeID = sess.CurrentEdgeID
	currentEdgeAt = sess.CurrentEdgeAt
	sess.Mu.Unlock()

	if advanceFrom >= 0 {
		t.Manager.AdvanceStep(advanceSessionID, advanceRoute, advanceFrom, advanceTo)
	}

	t.recordCurrentEdgeSpeedSample(currentEdgeID, currentEdgeAt, now, speedKmh)

	checkSession(
		sess,
		lat, lon,
		t.Graph,
		t.Store,
		t.Manager,
		t.Router,
		t.LiveWeights(),
		t.PrepareRoute,
	)
	debug := navigationanalysis.DebugSnapshot(sess, speedKmh)
	_ = sess.Send(navigationsessions.OutMsg{Type: "debug_update", Debug: &debug})
}

func (t *Tracker) SendInitialSpeedUpdates(sess *navigationsessions.Session) {
	for _, edgeID32 := range sess.RemainingEdges() {
		eid := model.EdgeID(edgeID32)
		edge, ok := t.Graph.Edge(eid)
		if !ok || edge.SpeedKmh <= 0 {
			continue
		}
		recSpeed := t.Store.RecommendedSpeedKmh(eid, edge.SpeedKmh, edge.DistanceM)
		if recSpeed == edge.SpeedKmh && t.Store.Density(eid) == 0 {
			continue
		}
		edgeIDVal := uint32(eid)
		recSpeedVal := recSpeed
		_ = sess.Send(navigationsessions.OutMsg{
			Type:                "speed_update",
			EdgeID:              &edgeIDVal,
			RecommendedSpeedKmh: &recSpeedVal,
		})
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
