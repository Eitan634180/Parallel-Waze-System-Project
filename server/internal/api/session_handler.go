package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// POST /session
type createSessionRequest struct {
	RouteID string `json:"route_id"`
}
type createSessionResponse struct {
	SessionID string `json:"session_id"`
}

// DELETE /session/:id  →  204

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createSession(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSessionID dispatches /session/:id and /session/:id/ws
func (s *Server) handleSessionID(w http.ResponseWriter, r *http.Request) {
	// Strip /session/ prefix and parse sub-path
	path := strings.TrimPrefix(r.URL.Path, "/session/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	sessionID := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}

	switch sub {
	case "ws":
		s.handleWS(w, r, sessionID)
	case "":
		if r.Method == http.MethodDelete {
			s.deleteSession(w, r, sessionID)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	entry, ok := s.routeCache[req.RouteID]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}

	sess := s.mgr.Create(entry.route)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(createSessionResponse{SessionID: sess.ID})
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request, id string) {
	s.mgr.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}
