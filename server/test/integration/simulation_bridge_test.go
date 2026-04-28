package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"testing"
	"time"

	"nav-system/test/testutil"
)

type simulationSpawnResult struct {
	Created int `json:"created"`
	Active  int `json:"active"`
}

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
	if len(routes.Routes) < 2 {
		t.Fatalf("expected at least 2 routes for simulation selection, got %d", len(routes.Routes))
	}
	selectedRoute := routes.Routes[1]
	payload, err := json.Marshal(map[string]any{
		"route_ids":      []string{selectedRoute.ID},
		"count":          1,
		"min_step_index": 1,
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
	var spawn simulationSpawnResult
	if err := json.NewDecoder(resp.Body).Decode(&spawn); err != nil {
		t.Fatalf("decode /simulation response: %v", err)
	}
	if spawn.Created != 1 || spawn.Active != 1 {
		t.Fatalf("unexpected simulation spawn result: %+v", spawn)
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
		if !ok || len(cars) != 1 {
			continue
		}
		car, ok := cars[0].(map[string]any)
		if !ok {
			t.Fatalf("snapshot car payload had unexpected shape: %+v", cars[0])
		}
		if car["id"] == "" {
			t.Fatalf("snapshot car id was empty: %+v", car)
		}

		expectedStart := selectedRoute.Steps[1]
		lat, latOK := car["lat"].(float64)
		lon, lonOK := car["lon"].(float64)
		if !latOK || !lonOK {
			t.Fatalf("snapshot car coordinates had unexpected shape: %+v", car)
		}
		if math.Abs(lat-expectedStart.Lat) > 0.0004 || math.Abs(lon-expectedStart.Lon) > 0.0004 {
			t.Fatalf("snapshot car spawned at %.6f,%.6f; want near %.6f,%.6f", lat, lon, expectedStart.Lat, expectedStart.Lon)
		}
		return
	}

	t.Fatal("expected simulation snapshot with at least one car")
}
