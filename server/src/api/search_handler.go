package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	nominatimSearchURL = "https://nominatim.openstreetmap.org/search"
	searchUserAgent    = "navigation-prototype/1.0"
)

var errSearchWarmupDisabled = errors.New("search warmup query not configured")

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
	req, err := s.newSearchRequest(r.Context(), query, s.search.Limit)
	if err != nil {
		http.Error(w, searchRequestFailedMsg, http.StatusInternalServerError)
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		http.Error(w, searchUpstreamUnavailableMsg, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, searchUpstreamFailedMsg, http.StatusBadGateway)
		return
	}

	var upstream []nominatimSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&upstream); err != nil {
		http.Error(w, searchDecodeFailedMsg, http.StatusBadGateway)
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

func (s *Server) WarmSearch(ctx context.Context) error {
	query := strings.TrimSpace(os.Getenv("NAV_SEARCH_WARMUP_QUERY"))
	if query == "" {
		return errSearchWarmupDisabled
	}

	req, err := s.newSearchRequest(ctx, query, 1)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("search warmup upstream status %d", resp.StatusCode)
	}

	return nil
}

func (s *Server) newSearchRequest(ctx context.Context, query string, limit int) (*http.Request, error) {
	values := url.Values{}
	values.Set("q", query)
	values.Set("format", "jsonv2")
	values.Set("limit", strconv.Itoa(limit))
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nominatimSearchURL+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", searchUserAgent)
	if s.search.Language != "" {
		req.Header.Set(headerAcceptLanguage, s.search.Language)
	}

	return req, nil
}
