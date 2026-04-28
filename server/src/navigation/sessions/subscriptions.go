package sessions

import (
	"nav-system/src/graph/model"
	routingentities "nav-system/src/routing/entities"
)

// SubscribersOf returns a snapshot of session IDs subscribed to edgeID.
func (m *Manager) SubscribersOf(edgeID model.EdgeID) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	subscribers := m.edgeSubscribers[edgeID]
	ids := make([]string, 0, len(subscribers))
	for id := range subscribers {
		ids = append(ids, id)
	}
	return ids
}

func (m *Manager) subscribeEdges(sessionID string, steps []routingentities.Step) {
	for _, step := range steps {
		if step.EdgeID == nil {
			continue
		}

		edgeID := model.EdgeID(*step.EdgeID)
		if m.edgeSubscribers[edgeID] == nil {
			m.edgeSubscribers[edgeID] = make(map[string]struct{})
		}
		m.edgeSubscribers[edgeID][sessionID] = struct{}{}
	}
}

func (m *Manager) unsubscribeEdges(sessionID string, steps []routingentities.Step) {
	for _, step := range steps {
		if step.EdgeID == nil {
			continue
		}

		m.unsubscribeEdge(sessionID, model.EdgeID(*step.EdgeID))
	}
}

func (m *Manager) unsubscribeEdge(sessionID string, edgeID model.EdgeID) {
	subscribers, ok := m.edgeSubscribers[edgeID]
	if !ok {
		return
	}

	delete(subscribers, sessionID)
	if len(subscribers) == 0 {
		delete(m.edgeSubscribers, edgeID)
	}
}
