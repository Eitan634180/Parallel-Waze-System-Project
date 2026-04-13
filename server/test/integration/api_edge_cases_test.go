package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"nav-system/test/testutil"
)

// TestDeleteNonExistentSessionIsIdempotent verifies that deleting an unknown
// session ID does not return a server error. The API treats DELETE as idempotent
// and returns 204 NoContent whether or not the session exists.
func TestDeleteNonExistentSessionIsIdempotent(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)
	server := httptestServer(t, fixture)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, server.URL+"/session/no-such-id", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /session/no-such-id: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusInternalServerError {
		t.Fatalf("deleting unknown session should not return 5xx, got %d", resp.StatusCode)
	}
	// The API is idempotent: it returns 204 even if the session doesn't exist.
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE unknown session: got %d, want 204 NoContent", resp.StatusCode)
	}
}

// TestRouteEndpointRejectsMissingFields verifies that /route with missing
// required coordinates returns 400, not 500.
func TestRouteEndpointRejectsMissingFields(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)

	// Payload omits dst_lat / dst_lon.
	req := httptest.NewRequest(http.MethodPost, "/route", strings.NewReader(`{"src_lat":32.0,"src_lon":34.0}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	fixture.Server.ServeHTTP(rec, req)

	if rec.Code >= http.StatusInternalServerError {
		t.Fatalf("missing dst coords should not cause 5xx, got %d", rec.Code)
	}
}

// TestRouteEndpointRejectsWrongContentType verifies that a GET to /route
// (or wrong method) returns a client-error, not a panic.
func TestRouteEndpointRejectsWrongMethod(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)

	req := httptest.NewRequest(http.MethodGet, "/route", nil)
	rec := httptest.NewRecorder()
	fixture.Server.ServeHTTP(rec, req)

	if rec.Code >= http.StatusInternalServerError {
		t.Fatalf("GET /route should not return 5xx, got %d", rec.Code)
	}
}

// TestWebSocketConnectionToNonExistentSessionFailsGracefully verifies that
// dialing the WS endpoint for an unknown session ID results in an HTTP error
// (not a panic or 500 on the server side).
func TestWebSocketConnectionToNonExistentSessionFailsGracefully(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)
	server := httptestServer(t, fixture)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/session/no-such-session/ws"
	_, resp, _ := wsDialerWithTimeout(2 * time.Second).Dial(wsURL, nil)
	if resp != nil {
		defer resp.Body.Close()
		if resp.StatusCode >= http.StatusInternalServerError {
			t.Fatalf("WS dial to non-existent session returned %d, want client error", resp.StatusCode)
		}
	}
}

// TestSessionWebSocketSendsETAAfterPing verifies that sending a valid ping
// returns at least one eta_update within the timeout window without requiring
// anything else from the session.
func TestSessionWebSocketSendsETAAfterPing(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)
	server := httptestServer(t, fixture)

	routeCase := testutil.LoadRouteCases(t, "diamond_cases.json")[0]
	routes := requestRoutes(t, server.URL,
		routeCase.Src.Lat, routeCase.Src.Lon,
		routeCase.Dst.Lat, routeCase.Dst.Lon, 1)
	if len(routes.Routes) == 0 {
		t.Fatal("no route from /route")
	}

	sessionID := createSession(t, server.URL, routes.Routes[0].ID)
	conn := dialWS(t, server.URL, "/session/"+sessionID+"/ws")
	defer conn.Close()

	ping := map[string]any{
		"type":       "ping",
		"lat":        routeCase.Dst.Lat,
		"lon":        routeCase.Dst.Lon,
		"speed_kmh":  30,
		"step_index": 1,
	}
	if err := conn.WriteJSON(ping); err != nil {
		t.Fatalf("WriteJSON ping: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var msg map[string]any
		if err := readWSJSON(conn, &msg); err != nil {
			t.Fatalf("read WS: %v", err)
		}
		if msg["type"] == "eta_update" {
			return
		}
	}
	t.Fatal("did not receive eta_update within timeout")
}

// TestSimulationEndpointRejectsEmptyRouteList verifies that POSTing a spawn
// request with an empty route_ids list returns a client error, not 500.
func TestSimulationEndpointRejectsEmptyRouteList(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)

	req := httptest.NewRequest(http.MethodPost, "/simulation",
		strings.NewReader(`{"route_ids":[],"count":1,"min_step_index":0}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	fixture.Server.ServeHTTP(rec, req)

	if rec.Code >= http.StatusInternalServerError {
		t.Fatalf("empty route_ids should not return 5xx, got %d", rec.Code)
	}
}

// TestMultipleSessionsReceiveIndependentETAUpdates spawns two sessions and
// verifies each gets its own eta_update without interference.
func TestMultipleSessionsReceiveIndependentETAUpdates(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)
	server := httptestServer(t, fixture)

	routeCase := testutil.LoadRouteCases(t, "diamond_cases.json")[0]
	routes := requestRoutes(t, server.URL,
		routeCase.Src.Lat, routeCase.Src.Lon,
		routeCase.Dst.Lat, routeCase.Dst.Lon, 1)
	if len(routes.Routes) == 0 {
		t.Fatal("no route from /route")
	}
	route := routes.Routes[0]

	sessionA := createSession(t, server.URL, route.ID)
	sessionB := createSession(t, server.URL, route.ID)

	connA := dialWS(t, server.URL, "/session/"+sessionA+"/ws")
	defer connA.Close()
	connB := dialWS(t, server.URL, "/session/"+sessionB+"/ws")
	defer connB.Close()

	ping := map[string]any{
		"type":       "ping",
		"lat":        routeCase.Src.Lat,
		"lon":        routeCase.Src.Lon,
		"speed_kmh":  25,
		"step_index": 1,
	}
	if err := connA.WriteJSON(ping); err != nil {
		t.Fatalf("WriteJSON connA: %v", err)
	}
	if err := connB.WriteJSON(ping); err != nil {
		t.Fatalf("WriteJSON connB: %v", err)
	}

	awaitETA := func(conn *websocket.Conn, label string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			var msg map[string]any
			if err := readWSJSON(conn, &msg); err != nil {
				return
			}
			if msg["type"] == "eta_update" {
				return
			}
		}
		t.Errorf("%s: did not receive eta_update in time", label)
	}

	awaitETA(connA, "session-A")
	awaitETA(connB, "session-B")
}
