package handlers

import (
	"net/http"

	"nav-system/src/graph/model"
	transportweb "nav-system/src/transport/web"
)

type SystemHandler struct {
	Graph *model.Graph
}

type SystemResponse struct {
	BBox struct {
		MinLat float64 `json:"min_lat"`
		MaxLat float64 `json:"max_lat"`
		MinLon float64 `json:"min_lon"`
		MaxLon float64 `json:"max_lon"`
	} `json:"bbox"`
	CenterLat float64 `json:"center_lat"`
	CenterLon float64 `json:"center_lon"`
}

func (h *SystemHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		transportweb.MethodNotAllowed(w)
		return
	}

	bbox := h.Graph.BBox
	resp := SystemResponse{
		CenterLat: bbox.CenterLat(),
		CenterLon: bbox.CenterLon(),
	}
	resp.BBox.MinLat = bbox.MinLat
	resp.BBox.MaxLat = bbox.MaxLat
	resp.BBox.MinLon = bbox.MinLon
	resp.BBox.MaxLon = bbox.MaxLon

	transportweb.WriteJSON(w, http.StatusOK, resp)
}
