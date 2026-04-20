package integration_test

import (
	"context"
	"testing"
	"time"

	"nav-system/test/testutil"
)

func TestActiveSessionsPropagateSpeedUpdateToSubscribedSession(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)
	server := httptestServer(t, fixture)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go fixture.Manager.RunPropagation(ctx, fixture.Store, fixture.Graph)

	routeCase := testutil.LoadRouteCases(t, "diamond_cases.json")[0]
	routes := requestRoutes(t, server.URL, routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1)
	routeID := routes.Routes[0].ID
	firstEdgeID := *routes.Routes[0].Steps[1].EdgeID
	baseSpeed := fixture.Graph.Edges[firstEdgeID].SpeedKmh

	primarySessionID := createSession(t, server.URL, routeID)
	primaryConn := dialWS(t, server.URL, "/session/"+primarySessionID+"/ws")
	defer primaryConn.Close()

	var extraConns []interface{ Close() error }
	for i := 0; i < 24; i++ {
		sessionID := createSession(t, server.URL, routeID)
		conn := dialWS(t, server.URL, "/session/"+sessionID+"/ws")
		extraConns = append(extraConns, conn)
	}
	defer func() {
		for _, conn := range extraConns {
			_ = conn.Close()
		}
	}()

	if density := fixture.Store.Density(firstEdgeID); density < 2 {
		t.Fatalf("expected connected sessions to increase edge density, got %d", density)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var message map[string]any
		if err := readWSJSON(primaryConn, &message); err != nil {
			t.Fatalf("read websocket message: %v", err)
		}

		if message["type"] != "speed_update" {
			continue
		}
		if uint32(message["edge_id"].(float64)) != firstEdgeID {
			continue
		}

		gotSpeed := float32(message["recommended_speed_kmh"].(float64))
		if gotSpeed >= baseSpeed {
			continue
		}
		return
	}

	t.Fatalf("expected reduced speed_update for edge %d", firstEdgeID)
}
