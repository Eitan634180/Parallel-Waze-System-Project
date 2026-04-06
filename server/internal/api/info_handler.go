package api

import "net/http"

// systemInfoResponse is returned by GET /system/info.
type systemInfoResponse struct {
	BBox struct {
		MinLat float64 `json:"min_lat"`
		MaxLat float64 `json:"max_lat"`
		MinLon float64 `json:"min_lon"`
		MaxLon float64 `json:"max_lon"`
	} `json:"bbox"`
	CenterLat float64 `json:"center_lat"`
	CenterLon float64 `json:"center_lon"`
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	bbox := s.g.BBox
	resp := systemInfoResponse{
		CenterLat: bbox.CenterLat(),
		CenterLon: bbox.CenterLon(),
	}
	resp.BBox.MinLat = bbox.MinLat
	resp.BBox.MaxLat = bbox.MaxLat
	resp.BBox.MinLon = bbox.MinLon
	resp.BBox.MaxLon = bbox.MaxLon

	writeJSON(w, http.StatusOK, resp)
}
