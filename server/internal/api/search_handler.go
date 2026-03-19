package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	nominatimSearchURL = "https://nominatim.openstreetmap.org/search"
	searchLimit        = 5
	searchUserAgent    = "navigation-prototype/1.0"
	searchCountryCodes = "il,ps"
	searchViewBox      = "34.15,33.45,35.90,29.45"
)

type nominatimSearchResult struct {
	DisplayName string `json:"display_name"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
}

type searchResult struct {
	DisplayName string  `json:"displayName"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]searchResult{})
		return
	}

	values := url.Values{}
	values.Set("q", query)
	values.Set("format", "jsonv2")
	values.Set("limit", strconv.Itoa(searchLimit))
	values.Set("addressdetails", "0")
	values.Set("accept-language", "he")
	values.Set("countrycodes", searchCountryCodes)
	values.Set("viewbox", searchViewBox)
	values.Set("bounded", "1")

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, nominatimSearchURL+"?"+values.Encode(), nil)
	if err != nil {
		http.Error(w, "failed to create search request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("User-Agent", searchUserAgent)
	req.Header.Set("Accept-Language", "he")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		http.Error(w, "search upstream unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "search upstream failed", http.StatusBadGateway)
		return
	}

	var upstream []nominatimSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&upstream); err != nil {
		http.Error(w, "failed to decode search results", http.StatusBadGateway)
		return
	}

	// Convert lat lng in results from strings to floats
	results := make([]searchResult, 0, len(upstream))
	for _, item := range upstream {
		lat, errLat := strconv.ParseFloat(item.Lat, 64)
		lng, errLng := strconv.ParseFloat(item.Lon, 64)
		if errLat != nil || errLng != nil {
			continue
		}
		results = append(results, searchResult{
			DisplayName: item.DisplayName,
			Lat:         lat,
			Lng:         lng,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}
