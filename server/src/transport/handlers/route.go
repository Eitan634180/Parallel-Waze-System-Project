package handlers

import (
	"fmt"
	"net/http"
	"time"

	"nav-system/src/graph/model"
	navigationanalysis "nav-system/src/navigation/analysis"
	navigationsessions "nav-system/src/navigation/sessions"
	routingengine "nav-system/src/routing/engine"
	routingentities "nav-system/src/routing/entities"
	trafficstore "nav-system/src/traffic/store"
	transportweb "nav-system/src/transport/web"
	"nav-system/src/utilities"
)

const (
	minRouteLatitude  = utilities.MinLatitude
	maxRouteLatitude  = utilities.MaxLatitude
	minRouteLongitude = utilities.MinLongitude
	maxRouteLongitude = utilities.MaxLongitude
)

type RouteHandler struct {
	Graph                   *model.Graph
	Store                   *trafficstore.Store
	Router                  *routingengine.Router
	Cache                   *RouteCache
	SlowRequestLogThreshold time.Duration
	BaseRouteCount          int
	MaxRouteCount           int
}

type routeRequest struct {
	SrcLat       *float64 `json:"src_lat"`
	SrcLon       *float64 `json:"src_lon"`
	DstLat       *float64 `json:"dst_lat"`
	DstLon       *float64 `json:"dst_lon"`
	Alternatives int      `json:"alternatives"`
}

type routeResponse struct {
	Routes []navigationsessions.RoutePayload `json:"routes"`
}

func (h *RouteHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		transportweb.MethodNotAllowed(w)
		return
	}

	req, ok := transportweb.DecodeJSON[routeRequest](w, r)
	if !ok {
		return
	}
	if err := req.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	start := time.Now()
	routeCount := h.normalizedRouteCount(req.Alternatives)
	routes := h.Router.Compute(*req.SrcLat, *req.SrcLon, *req.DstLat, *req.DstLon, routeCount, h.liveWeightFunc())
	transportweb.LogSlowOperation(
		h.SlowRequestLogThreshold,
		start,
		"[transport] route compute alternatives=%d returned=%d",
		routeCount,
		len(routes),
	)
	if len(routes) == 0 {
		http.Error(w, "no route found", http.StatusNotFound)
		return
	}

	resp := routeResponse{Routes: make([]navigationsessions.RoutePayload, 0, len(routes))}
	for _, route := range routes {
		resp.Routes = append(resp.Routes, h.routeResponsePayload(route))
	}

	transportweb.WriteJSON(w, http.StatusOK, resp)
}

func (h *RouteHandler) routeResponsePayload(route routingentities.Route) navigationsessions.RoutePayload {
	route.CongestionAhead, route.CongestedEdges = navigationanalysis.RouteCongestionSummary(route, h.Store, h.Graph)
	route = h.Cache.Store(route)

	return navigationsessions.RoutePayload{
		ID:              route.ID,
		Steps:           route.Steps,
		TotalDistM:      route.TotalDistM,
		TotalTimeSec:    route.TotalTimeSec,
		CongestionAhead: route.CongestionAhead,
		CongestedEdges:  route.CongestedEdges,
	}
}

func (h *RouteHandler) liveWeightFunc() routingengine.WeightFunc {
	return func(e *model.Edge) float32 {
		return h.Store.LiveWeight(e.ID, e.BaseWeight)
	}
}

func (h *RouteHandler) normalizedRouteCount(alternatives int) int {
	count := h.BaseRouteCount + alternatives
	if count < h.BaseRouteCount {
		return h.BaseRouteCount
	}
	if count > h.MaxRouteCount {
		return h.MaxRouteCount
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

	if *r.SrcLat < minRouteLatitude || *r.SrcLat > maxRouteLatitude || *r.DstLat < minRouteLatitude || *r.DstLat > maxRouteLatitude {
		return fmt.Errorf("latitude must be between %.0f and %.0f", minRouteLatitude, maxRouteLatitude)
	}
	if *r.SrcLon < minRouteLongitude || *r.SrcLon > maxRouteLongitude || *r.DstLon < minRouteLongitude || *r.DstLon > maxRouteLongitude {
		return fmt.Errorf("longitude must be between %.0f and %.0f", minRouteLongitude, maxRouteLongitude)
	}

	return nil
}
