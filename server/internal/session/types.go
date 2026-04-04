package session

import (
	"context"
	"errors"
	"sync"
	"time"

	"nav-system/internal/routing"

	"github.com/gorilla/websocket"
)

type RoutePayload struct {
	ID              string         `json:"id"`
	Steps           []routing.Step `json:"steps"`
	TotalDistM      float32        `json:"total_dist_m"`
	TotalTimeSec    float32        `json:"total_time_sec"`
	CongestionAhead bool           `json:"congestion_ahead"`
	CongestedEdges  int            `json:"congested_edges"`
}

// OutMsg is any message the server sends to the client over the WebSocket.
type OutMsg struct {
	Type string `json:"type"`

	ETASec *float32 `json:"eta_sec,omitempty"`

	Route         *RoutePayload `json:"route,omitempty"`
	RerouteReason *string       `json:"reroute_reason,omitempty"`
	OldETASec     *float32      `json:"old_eta_sec,omitempty"`
	NewETASec     *float32      `json:"new_eta_sec,omitempty"`

	EdgeID              *uint32  `json:"edge_id,omitempty"`
	RecommendedSpeedKmh *float32 `json:"recommended_speed_kmh,omitempty"`

	Debug *NavigationDebug `json:"debug,omitempty"`
}

type NavigationDebug struct {
	SessionID           string  `json:"session_id"`
	StepIndex           int     `json:"step_index"`
	CurrentEdgeID       *uint32 `json:"current_edge_id,omitempty"`
	OffRouteDistanceM   float32 `json:"off_route_distance_m"`
	OffRouteThresholdM  float32 `json:"off_route_threshold_m"`
	OffRouteViolations  int     `json:"off_route_violations"`
	CongestionAhead     bool    `json:"congestion_ahead"`
	CongestedEdges      int     `json:"congested_edges"`
	ETASec              float32 `json:"eta_sec"`
	SpeedKmh            float32 `json:"speed_kmh"`
	LastRerouteReason   string  `json:"last_reroute_reason,omitempty"`
	LastRerouteAtUnixMs int64   `json:"last_reroute_at_unix_ms,omitempty"`
}

// Session holds the full state of an active driving session.
type Session struct {
	Mu    sync.RWMutex
	ID    string
	Route routing.Route

	StepIdx       int
	CurrentEdgeID *uint32
	CurrentEdgeAt time.Time
	LastLat       float64
	LastLon       float64

	LastPing    time.Time
	LastReroute time.Time
	LastETAPush time.Time

	ETA float32

	OffRouteViolations    int
	LastOffRouteDistanceM float32
	LastCongestionAhead   bool
	LastCongestedEdges    int
	LastRerouteReason     string
	CheckBetterRoute      bool

	Conn       *websocket.Conn
	SendChan   chan OutMsg
	PumpCancel context.CancelFunc
}

// Send queues a message to be written to the client's WebSocket connection.
// It does not block. If the client buffer is full, the message is dropped.
func (s *Session) Send(msg OutMsg) error {
	select {
	case s.SendChan <- msg:
		return nil
	default:
		// Client is too slow, dropping message to prevent server blockage
		return errors.New("client message buffer full")
	}
}

// WritePump pushes queued messages to the websocket connection.
// It enforces a 2-second write deadline to prevent hanging the goroutine.
func (s *Session) WritePump(ctx context.Context, conn *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.SendChan:
			conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if err := conn.WriteJSON(msg); err != nil {
				conn.Close()
				return
			}
		}
	}
}

func DebugSnapshot(s *Session, speedKmh float32) NavigationDebug {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	var currentEdgeID *uint32
	if s.CurrentEdgeID != nil {
		edgeID := *s.CurrentEdgeID
		currentEdgeID = &edgeID
	}

	debug := NavigationDebug{
		SessionID:          s.ID,
		StepIndex:          s.StepIdx,
		CurrentEdgeID:      currentEdgeID,
		OffRouteDistanceM:  s.LastOffRouteDistanceM,
		OffRouteThresholdM: offRouteDistM,
		OffRouteViolations: s.OffRouteViolations,
		CongestionAhead:    s.LastCongestionAhead,
		CongestedEdges:     s.LastCongestedEdges,
		ETASec:             s.ETA,
		SpeedKmh:           speedKmh,
		LastRerouteReason:  s.LastRerouteReason,
	}
	if !s.LastReroute.IsZero() {
		debug.LastRerouteAtUnixMs = s.LastReroute.UnixMilli()
	}
	return debug
}
