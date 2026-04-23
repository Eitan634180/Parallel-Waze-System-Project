package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	routingengine "nav-system/src/routing/engine"
	"nav-system/test/testutil"
)

func TestRouteAndSessionLifecycleOverHTTPAndWebSocket(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)
	server := httptestServer(t, fixture)

	routeCase := testutil.LoadRouteCases(t, "diamond_cases.json")[0]
	routes := requestRoutes(t, server.URL, routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1)
	if len(routes.Routes) == 0 {
		t.Fatal("expected at least one route from /route")
	}
	testutil.AssertRouteValid(t, fixture.Graph, routes.Routes[0], routingengine.BaseWeight)

	sessionID := createSession(t, server.URL, routes.Routes[0].ID)
	conn := dialWS(t, server.URL, "/session/"+sessionID+"/ws")
	defer conn.Close()

	ping := map[string]any{
		"type":       "ping",
		"lat":        routeCase.Src.Lat,
		"lon":        routeCase.Src.Lon,
		"speed_kmh":  25,
		"step_index": 1,
	}
	if err := conn.WriteJSON(ping); err != nil {
		t.Fatalf("WriteJSON ping: %v", err)
	}

	var sawETA bool
	var sawDebug bool
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && (!sawETA || !sawDebug) {
		var message map[string]any
		if err := readWSJSON(conn, &message); err != nil {
			t.Fatalf("read websocket message: %v", err)
		}
		switch message["type"] {
		case "eta_update":
			sawETA = true
		case "debug_update":
			sawDebug = true
		}
	}

	if !sawETA || !sawDebug {
		t.Fatalf("expected eta and debug websocket messages, sawETA=%v sawDebug=%v", sawETA, sawDebug)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, server.URL+"/session/"+sessionID, nil)
	if err != nil {
		t.Fatalf("NewRequest delete session: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE session status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	var payload map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&payload)
}
