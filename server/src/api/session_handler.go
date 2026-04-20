package api

import (
	"fmt"
	"net/http"
	"time"
)

type createSessionRequest struct {
	RouteID *string `json:"route_id"`
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
			s.deleteSession(w, sessionID)
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
	if err := req.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	start := time.Now()
	route, ok := s.cachedRoute(*req.RouteID)
	if !ok {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}

	session := s.mgr.Create(route)
	logSlowOperation(
		slowSessionCreationLogThreshold,
		start,
		"[api] session create route=%s",
		*req.RouteID,
	)
	writeJSON(w, http.StatusOK, createSessionResponse{SessionID: session.ID})
}

func (r createSessionRequest) validate() error {
	if r.RouteID == nil || *r.RouteID == "" {
		return fmt.Errorf("missing required field route_id")
	}
	return nil
}

func (s *Server) deleteSession(w http.ResponseWriter, id string) {
	s.mgr.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}
