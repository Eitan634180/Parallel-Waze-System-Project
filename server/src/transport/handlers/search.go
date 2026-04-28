package handlers

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

	transportweb "nav-system/src/transport/web"
)

const (
	searchUpstreamUnavailableMsg = "search upstream unavailable"
	searchUpstreamFailedMsg      = "search upstream failed"
	searchDecodeFailedMsg        = "failed to decode search results"
	searchRequestFailedMsg       = "failed to create search request"
)

var errWarmupDisabled = errors.New("search warmup query not configured")

type SearchHandler struct {
	Client                  *http.Client
	Limit                   int
	Language                string
	CountryCodes            string
	ViewBox                 string
	UpstreamURL             string
	UserAgent               string
	SlowRequestLogThreshold time.Duration
}

type nominatimResult struct {
	DisplayName string `json:"display_name"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
}

type SearchResult struct {
	DisplayName string  `json:"displayName"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
}

func (h *SearchHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		transportweb.MethodNotAllowed(w)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		transportweb.WriteJSON(w, http.StatusOK, []SearchResult{})
		return
	}

	start := time.Now()
	req, err := h.newRequest(r.Context(), query, h.Limit)
	if err != nil {
		http.Error(w, searchRequestFailedMsg, http.StatusInternalServerError)
		return
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		http.Error(w, searchUpstreamUnavailableMsg, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, searchUpstreamFailedMsg, http.StatusBadGateway)
		return
	}

	var upstream []nominatimResult
	if err := json.NewDecoder(resp.Body).Decode(&upstream); err != nil {
		http.Error(w, searchDecodeFailedMsg, http.StatusBadGateway)
		return
	}

	results := make([]SearchResult, 0, len(upstream))
	for _, item := range upstream {
		lat, errLat := strconv.ParseFloat(item.Lat, 64)
		lng, errLng := strconv.ParseFloat(item.Lon, 64)
		if errLat != nil || errLng != nil {
			continue
		}
		results = append(results, SearchResult{
			DisplayName: item.DisplayName,
			Lat:         lat,
			Lng:         lng,
		})
	}

	transportweb.LogSlowOperation(
		h.SlowRequestLogThreshold,
		start,
		"[transport] search query=%q returned=%d",
		query,
		len(results),
	)
	transportweb.WriteJSON(w, http.StatusOK, results)
}

func (h *SearchHandler) Warm(ctx context.Context) error {
	query := strings.TrimSpace(os.Getenv("NAV_SEARCH_WARMUP_QUERY"))
	if query == "" {
		return errWarmupDisabled
	}

	req, err := h.newRequest(ctx, query, 1)
	if err != nil {
		return err
	}

	resp, err := h.Client.Do(req)
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

func (h *SearchHandler) newRequest(ctx context.Context, query string, limit int) (*http.Request, error) {
	values := url.Values{}
	values.Set("q", query)
	values.Set("format", "jsonv2")
	values.Set("limit", strconv.Itoa(limit))
	values.Set("addressdetails", "0")
	if h.Language != "" {
		values.Set("accept-language", h.Language)
	}
	if h.CountryCodes != "" {
		values.Set("countrycodes", h.CountryCodes)
	}
	if h.ViewBox != "" {
		values.Set("viewbox", h.ViewBox)
		values.Set("bounded", "1")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.UpstreamURL+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", h.UserAgent)
	if h.Language != "" {
		req.Header.Set("Accept-Language", h.Language)
	}

	return req, nil
}
