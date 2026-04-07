package integration_test

import (
	"context"
	"testing"
	"time"

	"nav-system/test/testutil"
)

func TestTrafficObservationPropagatesSpeedUpdateToSubscribedSession(t *testing.T) {
	fixture := testutil.BuildServerFixture(t, "diamond_graph.json", 2)
	server := httptestServer(t, fixture)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go fixture.Manager.RunPropagation(ctx, fixture.Store, fixture.Graph)

	routeCase := testutil.LoadRouteCases(t, "diamond_cases.json")[0]
	routes := requestRoutes(t, server.URL, routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1)
	sessionID := createSession(t, server.URL, routes.Routes[0].ID)
	conn := dialWS(t, server.URL, "/session/"+sessionID+"/ws")
	defer conn.Close()

	firstEdgeID := *routes.Routes[0].Steps[1].EdgeID
	baseSpeed := fixture.Graph.Edges[firstEdgeID].SpeedKmh
	fixture.Store.EnterEdge(firstEdgeID)

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var message map[string]any
		if err := readWSJSON(conn, &message); err != nil {
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
