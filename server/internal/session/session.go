package session

import (
	"sync"
	"time"

	"nav-system/internal/routing"

	"github.com/gorilla/websocket"
)

// ---------------------------------------------------------------------------
// WS message types (server → client)
// ---------------------------------------------------------------------------

type RoutePayload struct {
	ID           string         `json:"id"`
	Steps        []routing.Step `json:"steps"`
	TotalDistM   float32        `json:"total_dist_m"`
	TotalTimeSec float32        `json:"total_time_sec"`
}

// OutMsg is any message the server sends to the client over the WebSocket.
type OutMsg struct {
	Type string `json:"type"` // "eta_update" | "reroute" | "speed_update"

	// eta_update
	ETASec *float32 `json:"eta_sec,omitempty"`

	// reroute
	Route         *RoutePayload `json:"route,omitempty"`
	RerouteReason *string       `json:"reroute_reason,omitempty"`
	OldETASec     *float32      `json:"old_eta_sec,omitempty"`
	NewETASec     *float32      `json:"new_eta_sec,omitempty"`

	// speed_update
	EdgeID              *uint32  `json:"edge_id,omitempty"`
	RecommendedSpeedKmh *float32 `json:"recommended_speed_kmh,omitempty"`

	// debug_update
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

// ---------------------------------------------------------------------------
// Session
// ---------------------------------------------------------------------------

// Session holds the full state of an active driving session.
type Session struct {
	Mu    sync.RWMutex
	ID    string
	Route routing.Route

	// Navigation state
	StepIdx       int // index into Route.Steps of the client's current position
	CurrentEdgeID *uint32
	CurrentEdgeAt time.Time
	LastLat       float64
	LastLon       float64

	// Timing
	LastPing    time.Time
	LastReroute time.Time
	LastETAPush time.Time

	// Current ETA (seconds remaining from StepIdx to end)
	ETA float32

	OffRouteViolations    int
	LastOffRouteDistanceM float32
	LastCongestionAhead   bool
	LastCongestedEdges    int
	LastRerouteReason     string

	// WebSocket — protected by WriteMu for concurrent writes.
	Conn    *websocket.Conn
	WriteMu sync.Mutex // serialise all writes to Conn
}

// Send writes a message to the client's WebSocket connection.
// It is safe to call from multiple goroutines.
func (s *Session) Send(msg OutMsg) error {
	s.Mu.RLock()
	conn := s.Conn
	s.Mu.RUnlock()
	if conn == nil {
		return nil
	}
	s.WriteMu.Lock()
	defer s.WriteMu.Unlock()
	s.Conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	return conn.WriteJSON(msg)
}

// RemainingSteps returns the steps from the current position to the end.
func (s *Session) RemainingSteps() []routing.Step {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return s.remainingStepsLocked()
}

func (s *Session) remainingStepsLocked() []routing.Step {
	if s.StepIdx < 0 {
		return s.Route.Steps
	}
	if s.StepIdx >= len(s.Route.Steps) {
		return nil
	}
	return s.Route.Steps[s.StepIdx:]
}

// RemainingEdges returns the EdgeIDs of all steps after the current position.
func (s *Session) RemainingEdges() []uint32 {
	s.Mu.RLock()
	steps := s.remainingStepsLocked()
	ids := make([]uint32, 0, len(steps))
	for _, st := range steps {
		if st.EdgeID != nil {
			ids = append(ids, *st.EdgeID)
		}
	}
	s.Mu.RUnlock()
	return ids
}

func InitialStepIndex(route routing.Route) int {
	if len(route.Steps) > 1 {
		return 1
	}
	return 0
}

func CurrentEdgeForStep(route routing.Route, stepIdx int) *uint32 {
	if stepIdx < 0 || stepIdx >= len(route.Steps) {
		return nil
	}
	if stepIdx == 0 {
		if len(route.Steps) < 2 {
			return nil
		}
		return route.Steps[1].EdgeID
	}
	return route.Steps[stepIdx].EdgeID
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
