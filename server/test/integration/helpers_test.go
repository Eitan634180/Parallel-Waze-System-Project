package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"nav-system/src/routing"
	"nav-system/test/testutil"
)

type routeResponse struct {
	Routes []routing.Route `json:"routes"`
}

type sessionResponse struct {
	SessionID string `json:"session_id"`
}

func httptestServer(t *testing.T, fixture *testutil.ServerFixture) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(fixture.Server)
	t.Cleanup(server.Close)
	return server
}

func requestRoutes(t *testing.T, baseURL string, srcLat, srcLon, dstLat, dstLon float64, alternatives int) routeResponse {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"src_lat":      srcLat,
		"src_lon":      srcLon,
		"dst_lat":      dstLat,
		"dst_lon":      dstLon,
		"alternatives": alternatives,
	})
	if err != nil {
		t.Fatalf("marshal route request: %v", err)
	}

	resp, err := http.Post(baseURL+"/route", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /route: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /route status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var decoded routeResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode /route response: %v", err)
	}
	return decoded
}

func createSession(t *testing.T, baseURL, routeID string) string {
	t.Helper()

	payload, err := json.Marshal(map[string]string{"route_id": routeID})
	if err != nil {
		t.Fatalf("marshal session request: %v", err)
	}

	resp, err := http.Post(baseURL+"/session", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /session status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var decoded sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode /session response: %v", err)
	}
	return decoded.SessionID
}

func dialWS(t *testing.T, baseURL, path string) *websocket.Conn {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + path
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial %s: %v", wsURL, err)
	}
	return conn
}

func readWSJSON(conn *websocket.Conn, payload any) error {
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	return conn.ReadJSON(payload)
}

func drainWebSocketMessages(conn *websocket.Conn, duration time.Duration) []map[string]any {
	_ = conn.SetReadDeadline(time.Now().Add(duration))
	var out []map[string]any
	for {
		var message map[string]any
		if err := conn.ReadJSON(&message); err != nil {
			break
		}
		out = append(out, message)
	}
	_ = conn.SetReadDeadline(time.Time{})
	return out
}

func wsDialerWithTimeout(d time.Duration) *websocket.Dialer {
	return &websocket.Dialer{
		HandshakeTimeout: d,
	}
}
