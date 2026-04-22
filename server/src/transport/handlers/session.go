package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	navigationmanager "nav-system/src/navigation/manager"
	navigationsession "nav-system/src/navigation/session"
	navigationtracker "nav-system/src/navigation/tracker"
	transportweb "nav-system/src/transport/web"

	"github.com/gorilla/websocket"
)

const (
	sessionWSLogPrefix    = "transport: session websocket"
	sessionPathSeparator  = "/"
	sessionPathSplitLimit = 2
)

type SessionHandler struct {
	Manager                       *navigationmanager.Manager
	Tracker                       *navigationtracker.Tracker
	RouteCache                    *RouteCache
	OriginAllowed                 func(string) bool
	SlowSessionCreateLogThreshold time.Duration
}

type sessionCreateRequest struct {
	RouteID *string `json:"route_id"`
}

type sessionCreateResponse struct {
	SessionID string `json:"session_id"`
}

type sessionPingMessage struct {
	Type       string              `json:"type"`
	Lat        float64             `json:"lat"`
	Lon        float64             `json:"lon"`
	SpeedKmh   float32             `json:"speed_kmh"`
	StepIndex  int                 `json:"step_index"`
	EdgeEvents []sessionEdgeTravel `json:"edge_events,omitempty"`
}

type sessionEdgeTravel struct {
	EdgeID      uint32  `json:"edge_id"`
	ObservedSec float32 `json:"observed_sec"`
}

func (h *SessionHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.create(w, r)
	default:
		transportweb.MethodNotAllowed(w)
	}
}

func (h *SessionHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	sessionID, subpath, ok := parseSessionPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch subpath {
	case "ws":
		h.handleWS(w, r, sessionID)
	case "":
		if r.Method == http.MethodDelete {
			h.delete(w, sessionID)
		} else {
			transportweb.MethodNotAllowed(w)
		}
	default:
		http.NotFound(w, r)
	}
}

func (h *SessionHandler) create(w http.ResponseWriter, r *http.Request) {
	req, ok := transportweb.DecodeJSON[sessionCreateRequest](w, r)
	if !ok {
		return
	}
	if err := req.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	start := time.Now()
	route, ok := h.RouteCache.Get(*req.RouteID)
	if !ok {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}

	sess := h.Manager.Create(route)
	transportweb.LogSlowOperation(
		h.SlowSessionCreateLogThreshold,
		start,
		"[transport] session create route=%s",
		*req.RouteID,
	)
	transportweb.WriteJSON(w, http.StatusOK, sessionCreateResponse{SessionID: sess.ID})
}

func (h *SessionHandler) delete(w http.ResponseWriter, id string) {
	h.Manager.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}

func (r sessionCreateRequest) validate() error {
	if r.RouteID == nil || *r.RouteID == "" {
		return fmt.Errorf("missing required field route_id")
	}
	return nil
}

func (h *SessionHandler) handleWS(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess := h.Manager.Get(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	upgrader := transportweb.NewUpgrader(h.OriginAllowed)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("%s upgrade failed: %v", sessionWSLogPrefix, err)
		return
	}

	oldConn, needsEdgeInit := attachSessionConnection(sess, conn)
	if oldConn != nil {
		oldConn.Close()
	}

	ctx, cancel := context.WithCancel(context.Background())
	sess.Mu.Lock()
	if sess.PumpCancel != nil {
		sess.PumpCancel()
	}
	sess.PumpCancel = cancel
	sess.Mu.Unlock()

	go sess.WritePump(ctx, conn)

	if needsEdgeInit {
		h.Tracker.InitializeEdge(sess)
	}
	h.Tracker.SendInitialSpeedUpdates(sess)
	defer func() {
		ownsSession, _ := detachSessionConnection(sess, conn)
		cancel()
		conn.Close()
		if !ownsSession {
			return
		}
		h.Tracker.Destroy(sess)
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(
				err,
				websocket.CloseNormalClosure,
				websocket.CloseGoingAway,
				websocket.CloseNoStatusReceived,
			) {
				log.Printf("%s read failed for session %s: %v", sessionWSLogPrefix, sessionID, err)
			}
			return
		}

		var msg sessionPingMessage
		if err := json.Unmarshal(data, &msg); err != nil || msg.Type != "ping" {
			continue
		}

		h.processPing(sess, msg)
	}
}

func attachSessionConnection(sess *navigationsession.Session, conn *websocket.Conn) (*websocket.Conn, bool) {
	sess.Mu.Lock()
	defer sess.Mu.Unlock()

	oldConn := sess.Conn
	needsEdgeInit := oldConn == nil && sess.CurrentEdgeAt.IsZero()
	sess.Conn = conn
	return oldConn, needsEdgeInit
}

func detachSessionConnection(sess *navigationsession.Session, conn *websocket.Conn) (bool, *uint32) {
	sess.Mu.Lock()
	defer sess.Mu.Unlock()

	if sess.Conn != conn {
		return false, nil
	}

	currentEdgeID := sess.CurrentEdgeID
	sess.Conn = nil
	return true, currentEdgeID
}

func (h *SessionHandler) processPing(sess *navigationsession.Session, msg sessionPingMessage) {
	observations := make([]navigationtracker.EdgeObservation, len(msg.EdgeEvents))
	for i, event := range msg.EdgeEvents {
		observations[i] = navigationtracker.EdgeObservation{
			EdgeID:      event.EdgeID,
			ObservedSec: event.ObservedSec,
		}
	}
	h.Tracker.Advance(sess, msg.Lat, msg.Lon, msg.SpeedKmh, msg.StepIndex, observations)
}

func parseSessionPath(path string) (sessionID, subpath string, ok bool) {
	trimmed := strings.TrimPrefix(path, transportweb.SessionSubtreeRoutePath)
	parts := strings.SplitN(trimmed, sessionPathSeparator, sessionPathSplitLimit)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}

	if len(parts) == 2 {
		return parts[0], parts[1], true
	}
	return parts[0], "", true
}
