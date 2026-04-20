package api

import (
	"fmt"
	"net/http"
	"time"

	"nav-system/src/routing"
	"nav-system/src/session"
)

type routeRequest struct {
	SrcLat       *float64 `json:"src_lat"`
	SrcLon       *float64 `json:"src_lon"`
	DstLat       *float64 `json:"dst_lat"`
	DstLon       *float64 `json:"dst_lon"`
	Alternatives int      `json:"alternatives"` // number of alternatives (total = 1 + alternatives)
}

type routeResponse struct {
	Routes []session.RoutePayload `json:"routes"`
}

const (
	baseRouteCount = 1
	maxRouteCount  = 5
)

func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	req, ok := decodeJSON[routeRequest](w, r)
	if !ok {
		return
	}
	if err := req.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	start := time.Now()
	routeCount := normalizedRouteCount(req.Alternatives)
	routes := s.router.Compute(*req.SrcLat, *req.SrcLon, *req.DstLat, *req.DstLon, routeCount, s.liveWeightFunc())
	logSlowOperation(
		slowRouteRequestLogThreshold,
		start,
		"[api] route compute alternatives=%d returned=%d",
		routeCount,
		len(routes),
	)
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
	count := baseRouteCount + alternatives
	if count < baseRouteCount {
		return baseRouteCount
	}
	if count > maxRouteCount {
		return maxRouteCount
	}
	return count
}

func (r routeRequest) validate() error {
	switch {
	case r.SrcLat == nil:
		return fmt.Errorf("missing required field src_lat")
	case r.SrcLon == nil:
		return fmt.Errorf("missing required field src_lon")
	case r.DstLat == nil:
		return fmt.Errorf("missing required field dst_lat")
	case r.DstLon == nil:
		return fmt.Errorf("missing required field dst_lon")
	}

	if *r.SrcLat < -90 || *r.SrcLat > 90 || *r.DstLat < -90 || *r.DstLat > 90 {
		return fmt.Errorf("latitude must be between -90 and 90")
	}
	if *r.SrcLon < -180 || *r.SrcLon > 180 || *r.DstLon < -180 || *r.DstLon > 180 {
		return fmt.Errorf("longitude must be between -180 and 180")
	}

	return nil
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
