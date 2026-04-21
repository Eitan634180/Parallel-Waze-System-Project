package routehandler

import (
	"fmt"
	"net/http"
	"time"

	"nav-system/src/graph/model"
	navigationmonitor "nav-system/src/navigation/monitor"
	navigationsession "nav-system/src/navigation/session"
	"nav-system/src/routing"
	trafficstore "nav-system/src/traffic/store"
	transporthttp "nav-system/src/transport/http"
	"nav-system/src/utilities"
)

const (
	minRouteLatitude  = utilities.MinLatitude
	maxRouteLatitude  = utilities.MaxLatitude
	minRouteLongitude = utilities.MinLongitude
	maxRouteLongitude = utilities.MaxLongitude
)

type Handler struct {
	Graph                   *model.Graph
	Store                   *trafficstore.Store
	Router                  *routing.Router
	Cache                   *Cache
	SlowRequestLogThreshold time.Duration
	BaseRouteCount          int
	MaxRouteCount           int
}

type request struct {
	SrcLat       *float64 `json:"src_lat"`
	SrcLon       *float64 `json:"src_lon"`
	DstLat       *float64 `json:"dst_lat"`
	DstLon       *float64 `json:"dst_lon"`
	Alternatives int      `json:"alternatives"`
}

type response struct {
	Routes []navigationsession.RoutePayload `json:"routes"`
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		transporthttp.MethodNotAllowed(w)
		return
	}

	req, ok := transporthttp.DecodeJSON[request](w, r)
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
	transporthttp.LogSlowOperation(
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

	resp := response{Routes: make([]navigationsession.RoutePayload, 0, len(routes))}
	for _, route := range routes {
		resp.Routes = append(resp.Routes, h.routeResponsePayload(route))
	}

	transporthttp.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) routeResponsePayload(route routing.Route) navigationsession.RoutePayload {
	route.CongestionAhead, route.CongestedEdges = navigationmonitor.RouteCongestionSummary(route, h.Store, h.Graph)
	route = h.Cache.Store(route)

	return navigationsession.RoutePayload{
		ID:              route.ID,
		Steps:           route.Steps,
		TotalDistM:      route.TotalDistM,
		TotalTimeSec:    route.TotalTimeSec,
		CongestionAhead: route.CongestionAhead,
		CongestedEdges:  route.CongestedEdges,
	}
}

func (h *Handler) liveWeightFunc() routing.WeightFunc {
	return func(e *model.Edge) float32 {
		return h.Store.LiveWeight(e.ID, e.Weight)
	}
}

func (h *Handler) normalizedRouteCount(alternatives int) int {
	count := h.BaseRouteCount + alternatives
	if count < h.BaseRouteCount {
		return h.BaseRouteCount
	}
	if count > h.MaxRouteCount {
		return h.MaxRouteCount
	}
	return count
}

func (r request) validate() error {
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
