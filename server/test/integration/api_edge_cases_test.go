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

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing dst coords: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestRouteEndpointRejectsWrongContentType verifies that a GET to /route
// (or wrong method) returns a client-error, not a panic.
func TestRouteEndpointRejectsWrongMethod(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)

	req := httptest.NewRequest(http.MethodGet, "/route", nil)
	rec := httptest.NewRecorder()
	fixture.Server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /route: got %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestRouteEndpointRejectsOutOfRangeCoordinates(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)

	req := httptest.NewRequest(http.MethodPost, "/route",
		strings.NewReader(`{"src_lat":95.0,"src_lon":34.0,"dst_lat":32.0,"dst_lon":34.1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	fixture.Server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("out-of-range coordinates: got %d, want %d", rec.Code, http.StatusBadRequest)
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

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty route_ids: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateSessionRejectsMissingRouteID(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)

	req := httptest.NewRequest(http.MethodPost, "/session", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	fixture.Server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing route_id: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateSessionRejectsUnknownRouteID(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)

	req := httptest.NewRequest(http.MethodPost, "/session", strings.NewReader(`{"route_id":"missing-route"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	fixture.Server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown route_id: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRouteEndpointReturnsNotFoundWhenNoPathExists(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "disconnected_graph.json", 2)

	req := httptest.NewRequest(http.MethodPost, "/route",
		strings.NewReader(`{"src_lat":32.0000,"src_lon":34.0000,"dst_lat":32.0110,"dst_lon":34.0110}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	fixture.Server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("no-path route request: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestSimulationEndpointRejectsNegativeMinStepIndex(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)

	req := httptest.NewRequest(http.MethodPost, "/simulation",
		strings.NewReader(`{"route_ids":["route-1"],"count":1,"min_step_index":-1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	fixture.Server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negative min_step_index: got %d, want %d", rec.Code, http.StatusBadRequest)
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

	awaitOwnedSessionUpdates := func(conn *websocket.Conn, label, expectedSessionID string) {
		t.Helper()
		var sawETA bool
		var sawDebug bool
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) && (!sawETA || !sawDebug) {
			var msg map[string]any
			if err := readWSJSON(conn, &msg); err != nil {
				t.Fatalf("%s: read websocket message: %v", label, err)
			}

			switch msg["type"] {
			case "eta_update":
				sawETA = true
			case "debug_update":
				debug, ok := msg["debug"].(map[string]any)
				if !ok {
					t.Fatalf("%s: debug_update missing debug payload: %+v", label, msg)
				}
				if debug["session_id"] != expectedSessionID {
					t.Fatalf("%s: debug_update belonged to session %v, want %s", label, debug["session_id"], expectedSessionID)
				}
				sawDebug = true
			}
		}

		if !sawETA || !sawDebug {
			t.Fatalf("%s: expected eta and owned debug updates, sawETA=%v sawDebug=%v", label, sawETA, sawDebug)
		}
	}

	awaitOwnedSessionUpdates(connA, "session-A", sessionA)
	awaitOwnedSessionUpdates(connB, "session-B", sessionB)
}
