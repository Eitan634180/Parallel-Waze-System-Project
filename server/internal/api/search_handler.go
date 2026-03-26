package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	nominatimSearchURL = "https://nominatim.openstreetmap.org/search"
	searchUserAgent    = "navigation-prototype/1.0"
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
		methodNotAllowed(w)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusOK, []searchResult{})
		return
	}

	start := time.Now()
	values := url.Values{}
	values.Set("q", query)
	values.Set("format", "jsonv2")
	values.Set("limit", strconv.Itoa(s.search.Limit))
	values.Set("addressdetails", "0")
	if s.search.Language != "" {
		values.Set("accept-language", s.search.Language)
	}
	if s.search.CountryCodes != "" {
		values.Set("countrycodes", s.search.CountryCodes)
	}
	if s.search.ViewBox != "" {
		values.Set("viewbox", s.search.ViewBox)
		values.Set("bounded", "1")
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, nominatimSearchURL+"?"+values.Encode(), nil)
	if err != nil {
		http.Error(w, "failed to create search request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("User-Agent", searchUserAgent)
	if s.search.Language != "" {
		req.Header.Set("Accept-Language", s.search.Language)
	}

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

	logSlowOperation(
		slowSearchRequestLogThreshold,
		start,
		"[api] search query=%q returned=%d",
		query,
		len(results),
	)
	writeJSON(w, http.StatusOK, results)
}
