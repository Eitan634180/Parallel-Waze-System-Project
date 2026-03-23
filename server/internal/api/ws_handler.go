package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"nav-system/internal/routing"
	"nav-system/internal/session"
	"nav-system/map/builder"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return isAllowedBrowserOrigin(r.Header.Get("Origin"))
	},
}

type pingMsg struct {
	Type       string       `json:"type"`
	Lat        float64      `json:"lat"`
	Lon        float64      `json:"lon"`
	SpeedKmh   float32      `json:"speed_kmh"`
	StepIndex  int          `json:"step_index"`
	EdgeEvents []edgeTravel `json:"edge_events,omitempty"`
}

type edgeTravel struct {
	EdgeID      uint32  `json:"edge_id"`
	ObservedSec float32 `json:"observed_sec"`
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess := s.mgr.Get(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade: %v", err)
		return
	}

	oldConn, needsEdgeInit := attachSessionConnection(sess, conn)
	if oldConn != nil {
		oldConn.Close()
	}
	if needsEdgeInit {
		s.initializeSessionEdge(sess)
	}
	s.sendInitialSpeedUpdates(sess)
	defer func() {
		ownsSession, currentEdgeID := detachSessionConnection(sess, conn)
		sess.WriteMu.Lock()
		conn.Close()
		sess.WriteMu.Unlock()
		if !ownsSession {
			return
		}
		if currentEdgeID != nil {
			s.store.LeaveEdge(builder.EdgeID(*currentEdgeID))
		}
		s.mgr.Delete(sessionID)
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("ws read [%s]: %v", sessionID, err)
			}
			return
		}

		var msg pingMsg
		if err := json.Unmarshal(data, &msg); err != nil || msg.Type != "ping" {
			continue
		}

		s.processPing(sess, msg)
	}
}

func attachSessionConnection(sess *session.Session, conn *websocket.Conn) (*websocket.Conn, bool) {
	sess.Mu.Lock()
	defer sess.Mu.Unlock()

	oldConn := sess.Conn
	needsEdgeInit := oldConn == nil && sess.CurrentEdgeAt.IsZero()
	sess.Conn = conn
	return oldConn, needsEdgeInit
}

func detachSessionConnection(sess *session.Session, conn *websocket.Conn) (bool, *uint32) {
	sess.Mu.Lock()
	defer sess.Mu.Unlock()

	if sess.Conn != conn {
		return false, nil
	}

	currentEdgeID := sess.CurrentEdgeID
	sess.Conn = nil
	return true, currentEdgeID
}

func (s *Server) initializeSessionEdge(sess *session.Session) {
	sess.Mu.Lock()
	defer sess.Mu.Unlock()
	if sess.CurrentEdgeID == nil {
		sess.CurrentEdgeID = session.CurrentEdgeForStep(sess.Route, sess.StepIdx)
	}
	if sess.CurrentEdgeID == nil {
		return
	}
	sess.CurrentEdgeAt = time.Now()
	s.store.EnterEdge(builder.EdgeID(*sess.CurrentEdgeID))
}

func (s *Server) processPing(sess *session.Session, msg pingMsg) {
	now := time.Now()
	var advanceSessionID string
	var advanceRoute routing.Route
	advanceFrom := -1
	advanceTo := -1

	sess.Mu.Lock()
	sess.LastPing = now

	prevIdx := sess.StepIdx
	if prevIdx < 0 {
		prevIdx = 0
	}

	newIdx := normalizeSessionStepIndex(sess.Route, prevIdx, msg.StepIndex)
	s.recordEdgeObservations(msg.EdgeEvents)

	if newIdx > prevIdx {
		s.releaseTraversedEdges(sess.Route, prevIdx, newIdx)
	}

	if newIdx > sess.StepIdx {
		advanceSessionID = sess.ID
		advanceRoute = sess.Route
		advanceFrom = sess.StepIdx
		advanceTo = newIdx
		sess.StepIdx = newIdx
	}

	nextEdgeID := session.CurrentEdgeForStep(sess.Route, newIdx)
	if edgeChanged(sess.CurrentEdgeID, nextEdgeID) {
		if newIdx == prevIdx && sess.CurrentEdgeID != nil {
			s.store.LeaveEdge(builder.EdgeID(*sess.CurrentEdgeID))
		}
		sess.CurrentEdgeID = nextEdgeID
		sess.CurrentEdgeAt = now
		if sess.CurrentEdgeID != nil {
			s.store.EnterEdge(builder.EdgeID(*sess.CurrentEdgeID))
		}
	}
	sess.LastLat = msg.Lat
	sess.LastLon = msg.Lon
	sess.Mu.Unlock()

	if advanceFrom >= 0 {
		s.mgr.AdvanceStep(advanceSessionID, advanceRoute, advanceFrom, advanceTo)
	}

	session.Check(
		sess,
		msg.Lat, msg.Lon,
		s.g,
		s.store,
		s.mgr,
		s.router,
		s.liveWeightFunc(),
		s.cacheRoute,
	)
	debug := session.DebugSnapshot(sess, msg.SpeedKmh)
	_ = sess.Send(session.OutMsg{Type: "debug_update", Debug: &debug})
}

func (s *Server) recordEdgeObservations(events []edgeTravel) {
	for _, event := range events {
		edgeID := builder.EdgeID(event.EdgeID)
		if int(edgeID) >= len(s.g.Edges) || event.ObservedSec <= 0 {
			continue
		}

		edge := &s.g.Edges[edgeID]
		if edge.Weight <= 0 {
			continue
		}

		s.store.RecordObservation(edgeID, event.ObservedSec, edge.Weight)
	}
}

func (s *Server) releaseTraversedEdges(route routing.Route, fromIdx, toIdx int) {
	for i := fromIdx; i < toIdx; i++ {
		if i < 0 || i >= len(route.Steps) {
			continue
		}

		edgeID := route.Steps[i].EdgeID
		if edgeID != nil {
			s.store.LeaveEdge(builder.EdgeID(*edgeID))
		}
	}
}

func normalizeSessionStepIndex(route routing.Route, previousIndex, reportedIndex int) int {
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

func (s *Server) sendInitialSpeedUpdates(sess *session.Session) {
	for _, edgeID32 := range sess.RemainingEdges() {
		eid := builder.EdgeID(edgeID32)
		if int(eid) >= len(s.g.Edges) {
			continue
		}
		multiplier := s.store.Multiplier(eid)
		if multiplier == 1.0 && s.store.Density(eid) == 0 {
			continue
		}
		baseKmh := s.g.Edges[eid].SpeedKmh
		if baseKmh <= 0 {
			continue
		}
		recSpeed := s.store.LiveSpeedKmh(eid, s.g.Edges[eid].Weight, baseKmh, s.g.Edges[eid].DistanceM)
		edgeIDVal := uint32(eid)
		recSpeedVal := recSpeed
		_ = sess.Send(session.OutMsg{
			Type:                "speed_update",
			EdgeID:              &edgeIDVal,
			RecommendedSpeedKmh: &recSpeedVal,
		})
	}
}
