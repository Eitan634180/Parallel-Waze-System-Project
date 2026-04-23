package sessions

import (
	"sync"
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/navigation"
	"nav-system/src/routing"
	"nav-system/src/routing/engine"

	"github.com/google/uuid"
)

var (
	now       = time.Now
	newTicker = time.NewTicker
)

// Manager owns all active sessions and the reverse edge-to-session index.
type Manager struct {
	mu              sync.RWMutex
	sessions        map[string]*Session
	edgeSubscribers map[model.EdgeID]map[string]struct{}
}

// NewManager creates an empty Manager.
func NewManager() *Manager {
	return &Manager{
		sessions:        make(map[string]*Session),
		edgeSubscribers: make(map[model.EdgeID]map[string]struct{}),
	}
}

// Create registers a new session for the given route and returns it.
func (m *Manager) Create(route routing.Route) *Session {
	return m.create(route, engine.InitialStepIndex(route), make(chan OutMsg, navigation.SessionSendBufferSize))
}

// CreateHeadless registers a session without an attached outbound message queue.
func (m *Manager) CreateHeadless(route routing.Route, stepIdx int) *Session {
	return m.create(route, stepIdx, nil)
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
	if session.PumpCancel != nil {
		session.PumpCancel()
		session.PumpCancel = nil
	}
	remainingSteps := remainingStepsFrom(session.Route, session.StepIdx)
	session.Mu.Unlock()

	if conn != nil {
		conn.Close()
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

		m.unsubscribeEdge(sessionID, model.EdgeID(*edgeID))
	}
}

// UpdateRoute replaces a session's route and refreshes edge subscriptions.
func (m *Manager) UpdateRoute(s *Session, newRoute routing.Route) {
	s.Mu.Lock()
	oldSteps := append([]routing.Step(nil), s.Route.Steps[s.StepIdx:]...)
	s.Route = newRoute
	s.RouteRevision++
	s.StepIdx = engine.InitialStepIndex(newRoute)
	s.CurrentEdgeID = engine.CurrentEdgeForStep(newRoute, s.StepIdx)
	s.CurrentEdgeAt = now()
	newSteps := append([]routing.Step(nil), s.Route.Steps[s.StepIdx:]...)
	sessionID := s.ID
	s.Mu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.unsubscribeEdges(sessionID, oldSteps)
	m.subscribeEdges(sessionID, newSteps)
}

func (m *Manager) ActiveSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

func (m *Manager) create(route routing.Route, stepIdx int, sendChan chan OutMsg) *Session {
	id := uuid.NewString()
	if stepIdx < 0 {
		stepIdx = 0
	}
	if stepIdx >= len(route.Steps) {
		stepIdx = len(route.Steps) - 1
	}
	if len(route.Steps) == 0 {
		stepIdx = 0
	}

	session := &Session{
		ID:            id,
		Route:         route,
		RouteRevision: 1,
		StepIdx:       stepIdx,
		CurrentEdgeID: engine.CurrentEdgeForStep(route, stepIdx),
		LastPing:      now(),
		LastReroute:   now(),
		SendChan:      sendChan,
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[id] = session
	if stepIdx >= 0 && stepIdx < len(route.Steps) {
		m.subscribeEdges(id, route.Steps[stepIdx:])
	}
	return session
}

func remainingStepsFrom(route routing.Route, stepIdx int) []routing.Step {
	switch {
	case stepIdx < 0:
		return append([]routing.Step(nil), route.Steps...)
	case stepIdx >= len(route.Steps):
		return nil
	default:
		return append([]routing.Step(nil), route.Steps[stepIdx:]...)
	}
}
