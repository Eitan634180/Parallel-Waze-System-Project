package sessionhandler

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
	routehandler "nav-system/src/transport/handlers/route"
	transporthttp "nav-system/src/transport/http"
	internalconfig "nav-system/src/transport/internal/config"
	transportws "nav-system/src/transport/ws"

	"github.com/gorilla/websocket"
)

const (
	sessionWSLogPrefix    = "transport: session websocket"
	sessionPathSeparator  = "/"
	sessionPathSplitLimit = 2
)

type Handler struct {
	Manager       *navigationmanager.Manager
	Tracker       *navigationtracker.Tracker
	RouteCache    *routehandler.Cache
	OriginAllowed func(string) bool
}

type createRequest struct {
	RouteID *string `json:"route_id"`
}

type createResponse struct {
	SessionID string `json:"session_id"`
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

func (h *Handler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.create(w, r)
	default:
		transporthttp.MethodNotAllowed(w)
	}
}

func (h *Handler) HandleByID(w http.ResponseWriter, r *http.Request) {
	sessionID, subpath, ok := parsePath(r.URL.Path)
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
			transporthttp.MethodNotAllowed(w)
		}
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	req, ok := transporthttp.DecodeJSON[createRequest](w, r)
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
	transporthttp.LogSlowOperation(
		internalconfig.SlowSessionCreationLogThreshold,
		start,
		"[transport] session create route=%s",
		*req.RouteID,
	)
	transporthttp.WriteJSON(w, http.StatusOK, createResponse{SessionID: sess.ID})
}

func (h *Handler) delete(w http.ResponseWriter, id string) {
	h.Manager.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}

func (r createRequest) validate() error {
	if r.RouteID == nil || *r.RouteID == "" {
		return fmt.Errorf("missing required field route_id")
	}
	return nil
}

func (h *Handler) handleWS(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess := h.Manager.Get(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	upgrader := transportws.NewUpgrader(h.OriginAllowed)
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

		var msg pingMsg
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

func (h *Handler) processPing(sess *navigationsession.Session, msg pingMsg) {
	observations := make([]navigationtracker.EdgeObservation, len(msg.EdgeEvents))
	for i, event := range msg.EdgeEvents {
		observations[i] = navigationtracker.EdgeObservation{
			EdgeID:      event.EdgeID,
			ObservedSec: event.ObservedSec,
		}
	}
	h.Tracker.Advance(sess, msg.Lat, msg.Lon, msg.SpeedKmh, msg.StepIndex, observations)
}

func parsePath(path string) (sessionID, subpath string, ok bool) {
	trimmed := strings.TrimPrefix(path, transporthttp.SessionSubtreeRoutePath)
	parts := strings.SplitN(trimmed, sessionPathSeparator, sessionPathSplitLimit)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}

	if len(parts) == 2 {
		return parts[0], parts[1], true
	}
	return parts[0], "", true
}
