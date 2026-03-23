package session

import (
	"sync"

	"nav-system/internal/routing"
	"nav-system/map/builder"

	"github.com/google/uuid"
)

// Manager owns all active sessions and the reverse edge-to-session index.
type Manager struct {
	mu              sync.RWMutex
	sessions        map[string]*Session
	edgeSubscribers map[builder.EdgeID]map[string]struct{}
}

// NewManager creates an empty Manager.
func NewManager() *Manager {
	return &Manager{
		sessions:        make(map[string]*Session),
		edgeSubscribers: make(map[builder.EdgeID]map[string]struct{}),
	}
}

// Create registers a new session for the given route and returns it.
func (m *Manager) Create(route routing.Route) *Session {
	id := uuid.NewString()
	stepIdx := InitialStepIndex(route)
	session := &Session{
		ID:            id,
		Route:         route,
		StepIdx:       stepIdx,
		CurrentEdgeID: CurrentEdgeForStep(route, stepIdx),
		LastPing:      now(),
		LastReroute:   now(),
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[id] = session
	m.subscribeEdges(id, route.Steps[stepIdx:])
	return session
}

// Get retrieves a session by ID.
func (m *Manager) Get(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// Delete removes a session and unsubscribes it from the remaining route edges.
func (m *Manager) Delete(id string) {
	session := m.Get(id)
	if session == nil {
		return
	}

	session.Mu.Lock()
	conn := session.Conn
	session.Conn = nil
	remainingSteps := append([]routing.Step(nil), session.remainingStepsLocked()...)
	session.Mu.Unlock()

	if conn != nil {
		session.WriteMu.Lock()
		conn.Close()
		session.WriteMu.Unlock()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[id]; !ok {
		return
	}

	m.unsubscribeEdges(id, remainingSteps)
	delete(m.sessions, id)
}

// AdvanceStep removes subscriptions for edges that the session has already traversed.
func (m *Manager) AdvanceStep(sessionID string, route routing.Route, oldIdx, newIdx int) {
	if newIdx <= oldIdx {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i := oldIdx; i < newIdx && i < len(route.Steps); i++ {
		edgeID := route.Steps[i].EdgeID
		if edgeID == nil {
			continue
		}

		m.unsubscribeEdge(sessionID, builder.EdgeID(*edgeID))
	}
}

// UpdateRoute replaces a session's route and refreshes edge subscriptions.
func (m *Manager) UpdateRoute(s *Session, newRoute routing.Route) {
	s.Mu.Lock()
	oldSteps := append([]routing.Step(nil), s.Route.Steps[s.StepIdx:]...)
	s.Route = newRoute
	s.StepIdx = InitialStepIndex(newRoute)
	s.CurrentEdgeID = CurrentEdgeForStep(newRoute, s.StepIdx)
	s.CurrentEdgeAt = now()
	newSteps := append([]routing.Step(nil), s.Route.Steps[s.StepIdx:]...)
	sessionID := s.ID
	s.Mu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.unsubscribeEdges(sessionID, oldSteps)
	m.subscribeEdges(sessionID, newSteps)
}

// SubscribersOf returns a snapshot of session IDs subscribed to edgeID.
func (m *Manager) SubscribersOf(edgeID builder.EdgeID) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	subscribers := m.edgeSubscribers[edgeID]
	ids := make([]string, 0, len(subscribers))
	for id := range subscribers {
		ids = append(ids, id)
	}
	return ids
}

func (m *Manager) subscribeEdges(sessionID string, steps []routing.Step) {
	for _, step := range steps {
		if step.EdgeID == nil {
			continue
		}

		edgeID := builder.EdgeID(*step.EdgeID)
		if m.edgeSubscribers[edgeID] == nil {
			m.edgeSubscribers[edgeID] = make(map[string]struct{})
		}
		m.edgeSubscribers[edgeID][sessionID] = struct{}{}
	}
}

func (m *Manager) unsubscribeEdges(sessionID string, steps []routing.Step) {
	for _, step := range steps {
		if step.EdgeID == nil {
			continue
		}

		m.unsubscribeEdge(sessionID, builder.EdgeID(*step.EdgeID))
	}
}

func (m *Manager) unsubscribeEdge(sessionID string, edgeID builder.EdgeID) {
	subscribers, ok := m.edgeSubscribers[edgeID]
	if !ok {
		return
	}

	delete(subscribers, sessionID)
	if len(subscribers) == 0 {
		delete(m.edgeSubscribers, edgeID)
	}
}
