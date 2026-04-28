package external_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"nav-system/src/graph/store"
	routingengine "nav-system/src/routing/engine"
	"nav-system/test/testutil"
)

type graphHopperResponse struct {
	Paths []struct {
		Distance float64 `json:"distance"`
		TimeMS   int64   `json:"time"`
	} `json:"paths"`
}

func TestGraphHopperSanity(t *testing.T) {
	baseURL := os.Getenv("GRAPHHOPPER_BASE_URL")
	dataDir := os.Getenv("GRAPHHOPPER_GRAPH_DIR")
	if baseURL == "" || dataDir == "" {
		t.Skip("set GRAPHHOPPER_BASE_URL and GRAPHHOPPER_GRAPH_DIR to enable external sanity checks")
	}

	g, err := store.LoadGraph(dataDir)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported graph format version") {
			t.Skipf("graph data must be rebuilt before external sanity checks: %v", err)
		}
		t.Fatalf("LoadGraph(%s): %v", dataDir, err)
	}
	router := routingengine.NewRouter(g, routingengine.BuildSnapIndex(g))
	client := &http.Client{Timeout: 15 * time.Second}

	routeCase := testutil.LoadRouteCases(t, "diamond_cases.json")[0]
	internal := router.Compute(routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1, routingengine.BaseWeight)
	if len(internal) != 1 {
		t.Fatalf("internal router returned %d routes, want 1", len(internal))
	}

	u, err := url.Parse(baseURL + "/route")
	if err != nil {
		t.Fatalf("parse GRAPHHOPPER_BASE_URL: %v", err)
	}
	query := u.Query()
	query.Add("point", formatPoint(routeCase.Src.Lat, routeCase.Src.Lon))
	query.Add("point", formatPoint(routeCase.Dst.Lat, routeCase.Dst.Lon))
	query.Set("profile", envOrDefault("GRAPHHOPPER_PROFILE", "car"))
	query.Set("instructions", "false")
	query.Set("calc_points", "false")
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	if err != nil {
		t.Fatalf("new graphhopper request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("graphhopper request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("graphhopper status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var decoded graphHopperResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode graphhopper response: %v", err)
	}
	if len(decoded.Paths) == 0 {
		t.Fatal("graphhopper returned no paths")
	}

	externalDistance := float32(decoded.Paths[0].Distance)
	externalTime := float32(decoded.Paths[0].TimeMS) / 1000
	if !withinPercent(internal[0].TotalDistM, externalDistance, 0.20) {
		t.Fatalf("distance mismatch: internal=%.2f external=%.2f", internal[0].TotalDistM, externalDistance)
	}
	if !withinPercent(internal[0].TotalTimeSec, externalTime, 0.35) {
		t.Fatalf("time mismatch: internal=%.2f external=%.2f", internal[0].TotalTimeSec, externalTime)
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func formatPoint(lat, lon float64) string {
	return fmt.Sprintf("%.6f,%.6f", lat, lon)
}

func withinPercent(actual, expected float32, ratio float32) bool {
	diff := actual - expected
	if diff < 0 {
		diff = -diff
	}
	limit := expected * ratio
	if limit < 1 {
		limit = 1
	}
	return diff <= limit
}
