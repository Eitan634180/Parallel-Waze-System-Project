package routehandler

import (
	"fmt"
	"net/http"
	"time"

	apihttp "nav-system/src/api/http"
	coreconfig "nav-system/src/core/config"
	"nav-system/src/graph/model"
	"nav-system/src/routing"
	routingmonitor "nav-system/src/routing/monitor"
	"nav-system/src/session"
	trafficstore "nav-system/src/traffic/store"
)

type Handler struct {
	Graph  *model.Graph
	Store  *trafficstore.Store
	Router *routing.Router
	Cache  *Cache
}

type request struct {
	SrcLat       *float64 `json:"src_lat"`
	SrcLon       *float64 `json:"src_lon"`
	DstLat       *float64 `json:"dst_lat"`
	DstLon       *float64 `json:"dst_lon"`
	Alternatives int      `json:"alternatives"`
}

type response struct {
	Routes []session.RoutePayload `json:"routes"`
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apihttp.MethodNotAllowed(w)
		return
	}

	req, ok := apihttp.DecodeJSON[request](w, r)
	if !ok {
		return
	}
	if err := req.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	start := time.Now()
	routeCount := normalizedRouteCount(req.Alternatives)
	routes := h.Router.Compute(*req.SrcLat, *req.SrcLon, *req.DstLat, *req.DstLon, routeCount, h.liveWeightFunc())
	apihttp.LogSlowOperation(
		coreconfig.APISlowRouteRequestLogThreshold,
		start,
		"[api] route compute alternatives=%d returned=%d",
		routeCount,
		len(routes),
	)
	if len(routes) == 0 {
		http.Error(w, "no route found", http.StatusNotFound)
		return
	}

	resp := response{Routes: make([]session.RoutePayload, 0, len(routes))}
	for _, route := range routes {
		resp.Routes = append(resp.Routes, h.routeResponsePayload(route))
	}

	apihttp.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) routeResponsePayload(route routing.Route) session.RoutePayload {
	route.CongestionAhead, route.CongestedEdges = routingmonitor.RouteCongestionSummary(route, h.Store, h.Graph)
	route = h.Cache.Store(route)

	return session.RoutePayload{
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

func normalizedRouteCount(alternatives int) int {
	count := coreconfig.APIBaseRouteCount + alternatives
	if count < coreconfig.APIBaseRouteCount {
		return coreconfig.APIBaseRouteCount
	}
	if count > coreconfig.APIMaxRouteCount {
		return coreconfig.APIMaxRouteCount
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

	if *r.SrcLat < -90 || *r.SrcLat > 90 || *r.DstLat < -90 || *r.DstLat > 90 {
		return fmt.Errorf("latitude must be between -90 and 90")
	}
	if *r.SrcLon < -180 || *r.SrcLon > 180 || *r.DstLon < -180 || *r.DstLon > 180 {
		return fmt.Errorf("longitude must be between -180 and 180")
	}

	return nil
}
