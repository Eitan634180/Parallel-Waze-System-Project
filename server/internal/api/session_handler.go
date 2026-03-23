package api

import (
	"net/http"
)

type createSessionRequest struct {
	RouteID string `json:"route_id"`
}
type createSessionResponse struct {
	SessionID string `json:"session_id"`
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createSession(w, r)
	default:
		methodNotAllowed(w)
	}
}

// handleSessionID routes requests for /session/:id and /session/:id/ws.
func (s *Server) handleSessionID(w http.ResponseWriter, r *http.Request) {
	sessionID, subpath, ok := parseSessionPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch subpath {
	case "ws":
		s.handleWS(w, r, sessionID)
	case "":
		if r.Method == http.MethodDelete {
			s.deleteSession(w, r, sessionID)
		} else {
			methodNotAllowed(w)
		}
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[createSessionRequest](w, r)
	if !ok {
		return
	}

	route, ok := s.cachedRoute(req.RouteID)
	if !ok {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}

	session := s.mgr.Create(route)
	writeJSON(w, http.StatusOK, createSessionResponse{SessionID: session.ID})
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request, id string) {
	s.mgr.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}
