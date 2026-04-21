package manager

import (
	"context"

	coreconfig "nav-system/src/core/config"
)

// RunExpiry removes sessions whose last ping is too old.
func (m *Manager) RunExpiry(ctx context.Context) {
	ticker := newTicker(coreconfig.SessionExpiryCheckInterval)
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
	cutoff := now().Add(-coreconfig.SessionExpiry)

	m.mu.RLock()
	sessions := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		sessions = append(sessions, id)
	}
	m.mu.RUnlock()

	for _, sessionID := range sessions {
		session := m.Get(sessionID)
		if session == nil {
			continue
		}

		session.Mu.RLock()
		expired := session.LastPing.Before(cutoff)
		session.Mu.RUnlock()
		if expired {
			m.Delete(sessionID)
		}
	}
}
