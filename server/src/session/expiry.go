package session

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
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.RUnlock()

	for _, session := range sessions {
		session.Mu.RLock()
		expired := session.LastPing.Before(cutoff)
		sessionID := session.ID
		session.Mu.RUnlock()
		if expired {
			m.Delete(sessionID)
		}
	}
}
