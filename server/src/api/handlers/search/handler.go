package searchhandler

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

	apihttp "nav-system/src/api/http"
	coreconfig "nav-system/src/core/config"
)

const (
	nominatimSearchURL          = "https://nominatim.openstreetmap.org/search"
	searchUserAgent             = "navigation-prototype/1.0"
	searchUpstreamUnavailableMsg = "search upstream unavailable"
	searchUpstreamFailedMsg      = "search upstream failed"
	searchDecodeFailedMsg        = "failed to decode search results"
	searchRequestFailedMsg       = "failed to create search request"
)

var errWarmupDisabled = errors.New("search warmup query not configured")

type Handler struct {
	Client *http.Client
	Config coreconfig.SearchConfig
}

type nominatimResult struct {
	DisplayName string `json:"display_name"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
}

type Result struct {
	DisplayName string  `json:"displayName"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apihttp.MethodNotAllowed(w)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		apihttp.WriteJSON(w, http.StatusOK, []Result{})
		return
	}

	start := time.Now()
	req, err := h.newRequest(r.Context(), query, h.Config.Limit)
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

	results := make([]Result, 0, len(upstream))
	for _, item := range upstream {
		lat, errLat := strconv.ParseFloat(item.Lat, 64)
		lng, errLng := strconv.ParseFloat(item.Lon, 64)
		if errLat != nil || errLng != nil {
			continue
		}
		results = append(results, Result{
			DisplayName: item.DisplayName,
			Lat:         lat,
			Lng:         lng,
		})
	}

	apihttp.LogSlowOperation(
		coreconfig.APISlowSearchRequestLogThreshold,
		start,
		"[api] search query=%q returned=%d",
		query,
		len(results),
	)
	apihttp.WriteJSON(w, http.StatusOK, results)
}

func (h *Handler) Warm(ctx context.Context) error {
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

func (h *Handler) newRequest(ctx context.Context, query string, limit int) (*http.Request, error) {
	values := url.Values{}
	values.Set("q", query)
	values.Set("format", "jsonv2")
	values.Set("limit", strconv.Itoa(limit))
	values.Set("addressdetails", "0")
	if h.Config.Language != "" {
		values.Set("accept-language", h.Config.Language)
	}
	if h.Config.CountryCodes != "" {
		values.Set("countrycodes", h.Config.CountryCodes)
	}
	if h.Config.ViewBox != "" {
		values.Set("viewbox", h.Config.ViewBox)
		values.Set("bounded", "1")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nominatimSearchURL+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", searchUserAgent)
	if h.Config.Language != "" {
		req.Header.Set("Accept-Language", h.Config.Language)
	}

	return req, nil
}
