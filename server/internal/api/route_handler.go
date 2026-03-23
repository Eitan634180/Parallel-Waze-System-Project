package api

import (
	"net/http"

	"nav-system/internal/routing"
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
	Routes []session.RoutePayload `json:"routes"`
}

func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	req, ok := decodeJSON[routeRequest](w, r)
	if !ok {
		return
	}

	routeCount := normalizedRouteCount(req.Alternatives)
	routes := s.router.Compute(req.SrcLat, req.SrcLon, req.DstLat, req.DstLon, routeCount, s.liveWeightFunc())
	if len(routes) == 0 {
		http.Error(w, "no route found", http.StatusNotFound)
		return
	}

	response := routeResponse{Routes: make([]session.RoutePayload, 0, len(routes))}
	for _, route := range routes {
		response.Routes = append(response.Routes, s.routeResponsePayload(route))
	}

	writeJSON(w, http.StatusOK, response)
}

func normalizedRouteCount(alternatives int) int {
	count := 1 + alternatives
	if count < 1 {
		return 1
	}
	if count > 5 {
		return 5
	}
	return count
}

func (s *Server) routeResponsePayload(route routing.Route) session.RoutePayload {
	route.CongestionAhead, route.CongestedEdges = session.RouteCongestionSummary(route, s.store, s.g)
	route = s.cacheRoute(route)

	return session.RoutePayload{
		ID:              route.ID,
		Steps:           route.Steps,
		TotalDistM:      route.TotalDistM,
		TotalTimeSec:    route.TotalTimeSec,
		CongestionAhead: route.CongestionAhead,
		CongestedEdges:  route.CongestedEdges,
	}
}
