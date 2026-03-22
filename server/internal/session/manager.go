package session

import (
	"context"
	"sync"
	"time"

	"nav-system/internal/routing"
	"nav-system/internal/traffic"
	"nav-system/map/builder"

	"github.com/google/uuid"
)

const (
	sessionExpiry       = 5 * time.Minute
	expiryCheckInterval = 60 * time.Second
	propagationInterval = 5 * time.Second
)

// ---------------------------------------------------------------------------
// Manager
// ---------------------------------------------------------------------------

// Manager owns all active sessions and the reverse edge-to-session index.
type Manager struct {
	mu              sync.RWMutex
	sessions        map[string]*Session
	edgeSubscribers map[builder.EdgeID]map[string]struct{} // edgeID → set of sessionIDs
}

// NewManager creates an empty Manager.
func NewManager() *Manager {
	return &Manager{
		sessions:        make(map[string]*Session),
		edgeSubscribers: make(map[builder.EdgeID]map[string]struct{}),
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// Create registers a new session for the given route and returns it.
func (m *Manager) Create(route routing.Route) *Session {
	id := uuid.NewString()
	stepIdx := InitialStepIndex(route)
	s := &Session{
		ID:            id,
		Route:         route,
		StepIdx:       stepIdx,
		CurrentEdgeID: CurrentEdgeForStep(route, stepIdx),
		LastPing:      time.Now(),
		LastReroute:   time.Now(),
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.subscribeEdges(s)
	m.mu.Unlock()
	return s
}

// Get retrieves a session by ID (nil if not found).
func (m *Manager) Get(id string) *Session {
	m.mu.RLock()
	s := m.sessions[id]
	m.mu.RUnlock()
	return s
}

// Delete removes a session, unsubscribing its edges.
func (m *Manager) Delete(id string) {
	s := m.Get(id)
	if s != nil {
		s.Mu.Lock()
		defer s.Mu.Unlock()
		if s.Conn != nil {
			s.Conn.Close()
		}
	}
	m.mu.Lock()
	if s, ok := m.sessions[id]; ok {
		m.unsubscribeEdges(s)
		delete(m.sessions, id)
	}
	m.mu.Unlock()
}

// AdvanceStep updates the session's StepIdx, adjusting edge subscriptions.
// Must be called with session already write-locked by the caller if needed.
// (Here we take the Manager lock since we touch edgeSubscribers.)
func (m *Manager) AdvanceStep(s *Session, newIdx int) {
	if newIdx <= s.StepIdx {
		return
	}
	m.mu.Lock()
	// Unsubscribe edges that were traversed (indices StepIdx..newIdx-1).
	for i := s.StepIdx; i < newIdx && i < len(s.Route.Steps); i++ {
		edgeID := s.Route.Steps[i].EdgeID
		if edgeID == nil {
			continue
		}
		eid := builder.EdgeID(*edgeID)
		if subs, ok := m.edgeSubscribers[eid]; ok {
			delete(subs, s.ID)
			if len(subs) == 0 {
				delete(m.edgeSubscribers, eid)
			}
		}
	}
	s.StepIdx = newIdx
	m.mu.Unlock()
}

// UpdateRoute replaces a session's route (reroute) and resubscribes edges.
func (m *Manager) UpdateRoute(s *Session, newRoute routing.Route) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	m.mu.Lock()
	m.unsubscribeEdges(s)
	s.Route = newRoute
	s.StepIdx = InitialStepIndex(newRoute)
	s.CurrentEdgeID = CurrentEdgeForStep(newRoute, s.StepIdx)
	s.CurrentEdgeAt = time.Now()
	m.subscribeEdges(s)
	m.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Edge subscription helpers (caller must hold m.mu write lock)
// ---------------------------------------------------------------------------

func (m *Manager) subscribeEdges(s *Session) {
	for _, step := range s.Route.Steps[s.StepIdx:] {
		if step.EdgeID == nil {
			continue
		}
		eid := builder.EdgeID(*step.EdgeID)
		if m.edgeSubscribers[eid] == nil {
			m.edgeSubscribers[eid] = make(map[string]struct{})
		}
		m.edgeSubscribers[eid][s.ID] = struct{}{}
	}
}

func (m *Manager) unsubscribeEdges(s *Session) {
	for _, step := range s.Route.Steps[s.StepIdx:] {
		if step.EdgeID == nil {
			continue
		}
		eid := builder.EdgeID(*step.EdgeID)
		if subs, ok := m.edgeSubscribers[eid]; ok {
			delete(subs, s.ID)
			if len(subs) == 0 {
				delete(m.edgeSubscribers, eid)
			}
		}
	}
}

// SubscribersOf returns a snapshot of session IDs subscribed to edgeID.
func (m *Manager) SubscribersOf(eid builder.EdgeID) []string {
	m.mu.RLock()
	subs := m.edgeSubscribers[eid]
	ids := make([]string, 0, len(subs))
	for id := range subs {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	return ids
}

// ---------------------------------------------------------------------------
// Background workers
// ---------------------------------------------------------------------------

// RunExpiry starts the session auto-expiry goroutine.
// It removes sessions whose last ping was a long time ago.
func (m *Manager) RunExpiry(ctx context.Context) {
	ticker := time.NewTicker(expiryCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.expireSessions()
		}
	}
}

func (m *Manager) expireSessions() {
	cutoff := time.Now().Add(-sessionExpiry)
	var toCheck []*Session

	m.mu.RLock()
	for _, s := range m.sessions {
		toCheck = append(toCheck, s)
	}
	m.mu.RUnlock()

	var expiredIDs []string
	for _, s := range toCheck {
		s.Mu.RLock()
		expired := s.LastPing.Before(cutoff)
		s.Mu.RUnlock()
		if expired {
			expiredIDs = append(expiredIDs, s.ID)
		}
	}

	for _, id := range expiredIDs {
		m.Delete(id)
	}
}

// RunPropagation starts the background speed-update propagation goroutine.
// It polls the traffic Store for significantly-changed edges every 5 s and
// pushes speed_update messages to all subscribed sessions.
func (m *Manager) RunPropagation(ctx context.Context, store *traffic.Store, g *builder.Graph) {
	ticker := time.NewTicker(propagationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.propagate(store, g)
		}
	}
}

func (m *Manager) propagate(store *traffic.Store, g *builder.Graph) {
	changed := store.DirtySnapshot()

	// Capture edges that experienced significant traffic relaxation
	var improvedEdges []traffic.ChangedEdge
	for _, ce := range changed {
		if ce.OldMultiplier-ce.NewMultiplier >= traffic.SignificantShift {
			improvedEdges = append(improvedEdges, ce)
		}
	}

	if len(improvedEdges) > 0 {
		m.mu.RLock()
		activeSessions := make([]*Session, 0, len(m.sessions))
		for _, s := range m.sessions {
			activeSessions = append(activeSessions, s)
		}
		m.mu.RUnlock()
		go evaluateHeuristics(activeSessions, improvedEdges, store, g)
	}

	for _, ce := range changed {
		// Find base speed for this edge.
		var baseKmh float32
		if int(ce.EdgeID) < len(g.Edges) {
			baseKmh = g.Edges[ce.EdgeID].SpeedKmh
		}
		if baseKmh <= 0 {
			continue
		}
		recSpeed := store.RecommendedSpeedKmh(ce.EdgeID, baseKmh, g.Edges[ce.EdgeID].DistanceM)

		eid32 := uint32(ce.EdgeID)
		recSpeedVal := recSpeed
		subscribers := m.SubscribersOf(ce.EdgeID)

		for _, sid := range subscribers {
			s := m.Get(sid)
			if s == nil {
				continue
			}
			s.Mu.RLock()
			hasConn := s.Conn != nil
			s.Mu.RUnlock()
			if !hasConn {
				continue
			}
			_ = s.Send(OutMsg{
				Type:                "speed_update",
				EdgeID:              &eid32,
				RecommendedSpeedKmh: &recSpeedVal,
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Optimization 2 Background Workers
// ---------------------------------------------------------------------------

func evaluateHeuristics(sessions []*Session, improvedEdges []traffic.ChangedEdge, store *traffic.Store, g *builder.Graph) {
	const maxSpeedMs = 120.0 / 3.6

	var wg sync.WaitGroup
	numWorkers := 10
	if len(sessions) < numWorkers {
		numWorkers = len(sessions)
	}
	if numWorkers == 0 {
		return
	}

	chunkSize := len(sessions) / numWorkers
	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if w == numWorkers-1 {
			end = len(sessions)
		}

		wg.Add(1)
		go func(sessChunk []*Session) {
			defer wg.Done()
			for _, s := range sessChunk {
				s.Mu.RLock()
				if len(s.Route.Steps) == 0 {
					s.Mu.RUnlock()
					continue
				}
				carX, carY := projectForCheck(s.LastLat, s.LastLon)
				destStep := s.Route.Steps[len(s.Route.Steps)-1]
				destNode := g.NodeByID(builder.NodeID(destStep.NodeID))
				eta := s.ETA
				s.Mu.RUnlock()

				if destNode == nil {
					continue
				}

				flagged := false
				for _, ce := range improvedEdges {
					if int(ce.EdgeID) >= len(g.Edges) {
						continue
					}
					edge := g.Edges[ce.EdgeID]
					u := g.NodeByID(edge.FromNodeID)
					v := g.NodeByID(edge.ToNodeID)
					if u == nil || v == nil {
						continue
					}

					// Lower-Bound calculation
					distCarU := distancePointToPoint(carX, carY, u.X, u.Y)
					distVDst := distancePointToPoint(v.X, v.Y, destNode.X, destNode.Y)

					idealTime := (distCarU / maxSpeedMs) + store.LiveWeight(ce.EdgeID, edge.Weight, edge.SpeedKmh, edge.DistanceM) + (distVDst / maxSpeedMs)

					if idealTime < eta {
						flagged = true
						break
					}
				}

				if flagged {
					s.Mu.Lock()
					s.CheckBetterRoute = true
					s.Mu.Unlock()
				}
			}
		}(sessions[start:end])
	}
	wg.Wait()
}

// RunOptimizationSweep runs in the background to execute A* for sessions flagged with CheckBetterRoute.
func (m *Manager) RunOptimizationSweep(ctx context.Context, g *builder.Graph, store *traffic.Store, router *routing.Router, wf routing.WeightFunc, prepareRoute func(routing.Route) routing.Route) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sweepOptimizations(g, store, router, wf, prepareRoute)
		}
	}
}

func (m *Manager) sweepOptimizations(g *builder.Graph, store *traffic.Store, router *routing.Router, wf routing.WeightFunc, prepareRoute func(routing.Route) routing.Route) {
	m.mu.RLock()
	var flagged []*Session
	for _, s := range m.sessions {
		s.Mu.RLock()
		toCheck := s.CheckBetterRoute
		s.Mu.RUnlock()
		if toCheck {
			flagged = append(flagged, s)
		}
	}
	m.mu.RUnlock()

	for _, s := range flagged {
		s.Mu.Lock()
		s.CheckBetterRoute = false
		if len(s.Route.Steps) == 0 {
			s.Mu.Unlock()
			continue
		}
		snapLat, snapLon := s.LastLat, s.LastLon
		dst := s.Route.Steps[len(s.Route.Steps)-1]
		oldETA := computeETALocked(s, g, store)
		s.Mu.Unlock()

		newRoutes := router.Compute(snapLat, snapLon, dst.Lat, dst.Lon, 1, wf)
		if len(newRoutes) == 0 {
			continue
		}

		newETA := newRoutes[0].TotalTimeSec
		etaGain := oldETA - newETA
		if oldETA > 0 && (etaGain/oldETA >= rerouteSpeedupMin || etaGain >= rerouteMinGainSec) && !sameRemainingRoute(s, newRoutes[0]) {
			newRoute := prepareRoute(newRoutes[0])

			s.Mu.RLock()
			currentEdgeID := s.CurrentEdgeID
			s.Mu.RUnlock()
			if currentEdgeID != nil {
				store.LeaveEdge(builder.EdgeID(*currentEdgeID))
			}

			m.UpdateRoute(s, newRoute)

			now := time.Now()
			s.Mu.Lock()
			if s.CurrentEdgeID != nil {
				store.EnterEdge(builder.EdgeID(*s.CurrentEdgeID))
			}
			s.LastReroute = now
			s.LastRerouteReason = "traffic_cleared"
			s.OffRouteViolations = 0
			s.Mu.Unlock()

			routePayload := RoutePayload{
				ID:           newRoute.ID,
				Steps:        newRoute.Steps,
				TotalDistM:   newRoute.TotalDistM,
				TotalTimeSec: newRoute.TotalTimeSec,
			}

			reason := "traffic_cleared"
			oldETAVal := oldETA
			newETAVal := newETA
			_ = s.Send(OutMsg{
				Type:          "reroute",
				Route:         &routePayload,
				RerouteReason: &reason,
				OldETASec:     &oldETAVal,
				NewETASec:     &newETAVal,
			})
		}
	}
}
