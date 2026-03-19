package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"nav-system/internal/session"
	"nav-system/map/builder"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
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

	sess.Mu.Lock()
	sess.Conn = conn
	sess.Mu.Unlock()
	s.initializeSessionEdge(sess)
	s.sendInitialSpeedUpdates(sess)
	defer func() {
		sess.Mu.Lock()
		currentEdgeID := sess.CurrentEdgeID
		sess.Conn = nil
		sess.Mu.Unlock()
		if currentEdgeID != nil {
			s.store.LeaveEdge(builder.EdgeID(*currentEdgeID))
		}

		sess.WriteMu.Lock()
		conn.Close()
		sess.WriteMu.Unlock()
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
	sess.Mu.Lock()
	sess.LastPing = now

	prevIdx := sess.StepIdx
	if prevIdx < 0 {
		prevIdx = 0
	}

	newIdx := msg.StepIndex
	if newIdx < prevIdx {
		newIdx = prevIdx
	}
	if newIdx >= len(sess.Route.Steps) {
		newIdx = len(sess.Route.Steps) - 1
	}

	for _, evt := range msg.EdgeEvents {
		eid := builder.EdgeID(evt.EdgeID)
		if int(eid) >= len(s.g.Edges) || evt.ObservedSec <= 0 {
			continue
		}
		e := &s.g.Edges[eid]
		if e.Weight <= 0 {
			continue
		}
		s.store.RecordObservation(eid, evt.ObservedSec, e.Weight)
	}

	if newIdx > prevIdx {
		for i := prevIdx; i < newIdx; i++ {
			if i < 0 || i >= len(sess.Route.Steps) {
				continue
			}
			edgeID := sess.Route.Steps[i].EdgeID
			if edgeID != nil {
				s.store.LeaveEdge(builder.EdgeID(*edgeID))
			}
		}
	}

	s.mgr.AdvanceStep(sess, newIdx)
	nextEdgeID := session.CurrentEdgeForStep(sess.Route, newIdx)
	currentEdgeChanged := (nextEdgeID == nil) != (sess.CurrentEdgeID == nil)
	if !currentEdgeChanged && nextEdgeID != nil && sess.CurrentEdgeID != nil {
		currentEdgeChanged = *nextEdgeID != *sess.CurrentEdgeID
	}
	if currentEdgeChanged {
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

	session.Check(
		sess,
		msg.Lat, msg.Lon,
		s.g,
		s.store,
		s.mgr,
		s.router,
		s.liveWeightFunc(),
		s.prepareRoute,
	)
	debug := session.DebugSnapshot(sess, msg.SpeedKmh)
	_ = sess.Send(session.OutMsg{Type: "debug_update", Debug: &debug})
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
		recSpeed := s.store.RecommendedSpeedKmh(eid, baseKmh, s.g.Edges[eid].DistanceM)
		edgeIDVal := uint32(eid)
		recSpeedVal := recSpeed
		_ = sess.Send(session.OutMsg{
			Type:                "speed_update",
			EdgeID:              &edgeIDVal,
			RecommendedSpeedKmh: &recSpeedVal,
		})
	}
}
