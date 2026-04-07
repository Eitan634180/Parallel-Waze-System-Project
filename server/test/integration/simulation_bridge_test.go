package integration_test

import (
	"context"
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"nav-system/test/testutil"
)

func TestSimulationWebSocketPublishesSnapshotsAfterSpawn(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)
	server := httptestServer(t, fixture)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go fixture.Simulation.Run(ctx)

	conn := dialWS(t, server.URL, "/simulation/ws")
	defer conn.Close()

	routeCase := testutil.LoadRouteCases(t, "diamond_cases.json")[0]
	routes := requestRoutes(t, server.URL, routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1)
	payload, err := json.Marshal(map[string]any{
		"route_ids":      []string{routes.Routes[0].ID},
		"count":          1,
		"min_step_index": 0,
	})
	if err != nil {
		t.Fatalf("marshal simulation request: %v", err)
	}

	resp, err := http.Post(server.URL+"/simulation", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /simulation: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /simulation status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var message map[string]any
		if err := readWSJSON(conn, &message); err != nil {
			t.Fatalf("read simulation websocket message: %v", err)
		}
		if message["type"] != "snapshot" {
			continue
		}
		cars, ok := message["cars"].([]any)
		if ok && len(cars) > 0 {
			return
		}
	}

	t.Fatal("expected simulation snapshot with at least one car")
}
