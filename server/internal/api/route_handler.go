package api

import (
	"encoding/json"
	"net/http"

	"nav-system/internal/session"
)

type routeRequest struct {
	SrcLat       float64 `json:"src_lat"`
	SrcLon       float64 `json:"src_lon"`
	DstLat       float64 `json:"dst_lat"`
	DstLon       float64 `json:"dst_lon"`
	Alternatives int     `json:"alternatives"` // number of alternatives (total = 1 + alternatives)
}

type routeResponse struct {
	Routes []routeWithID `json:"routes"`
}

type routeWithID struct {
	ID           string      `json:"id"`
	Steps        interface{} `json:"steps"`
	TotalDistM   float32     `json:"total_dist_m"`
	TotalTimeSec float32     `json:"total_time_sec"`
	CongestionAhead bool     `json:"congestion_ahead"`
	CongestedEdges  int      `json:"congested_edges"`
}

func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req routeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	k := 1 + req.Alternatives
	if k < 1 {
		k = 1
	}
	if k > 5 {
		k = 5
	}

	routes := s.router.Compute(req.SrcLat, req.SrcLon, req.DstLat, req.DstLon, k, s.liveWeightFunc())
	if len(routes) == 0 {
		http.Error(w, "no route found", http.StatusNotFound)
		return
	}

	// Assign stable UUIDs for routes that will be referenced by POST /session.
	resp := routeResponse{Routes: make([]routeWithID, len(routes))}
	for i, rt := range routes {
		rt.CongestionAhead, rt.CongestedEdges = session.RouteCongestionSummary(rt, s.store, s.g)
		rt = s.prepareRoute(rt)
		routes[i] = rt
		resp.Routes[i] = routeWithID{
			ID:           rt.ID,
			Steps:        rt.Steps,
			TotalDistM:   rt.TotalDistM,
			TotalTimeSec: rt.TotalTimeSec,
			CongestionAhead: rt.CongestionAhead,
			CongestedEdges:  rt.CongestedEdges,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
