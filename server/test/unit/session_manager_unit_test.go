package unit_test

import (
	"testing"

	"nav-system/src/graph/model"
	navigationsessions "nav-system/src/navigation/sessions"
	routingentities "nav-system/src/routing/entities"
)

func TestUpdateRouteAtStepIfCurrentRejectsStaleVersion(t *testing.T) {
	manager := navigationsessions.NewManager()
	original := testRoute(1, 2)
	session := manager.CreateHeadless(original, original.InitialStepIndex())

	replacement := testRoute(3, 4)
	oldEdgeID, newEdgeID, ok := manager.UpdateRouteAtStepIfCurrent(
		session,
		replacement,
		replacement.InitialStepIndex(),
		session.StepIdx+1,
		session.RouteRevision,
	)
	if ok {
		t.Fatal("stale update should be rejected")
	}
	if oldEdgeID != nil || newEdgeID != nil {
		t.Fatalf("rejected update returned edge transition old=%v new=%v", oldEdgeID, newEdgeID)
	}

	session.Mu.RLock()
	gotRevision := session.RouteRevision
	gotEdge := session.CurrentEdgeID
	session.Mu.RUnlock()

	if gotRevision != 1 {
		t.Fatalf("route revision changed after stale update: got %d want 1", gotRevision)
	}
	if gotEdge == nil || *gotEdge != 1 {
		t.Fatalf("current edge changed after stale update: got %v want 1", gotEdge)
	}
	if len(manager.SubscribersOf(model.EdgeID(3))) != 0 {
		t.Fatal("stale replacement subscribed the session to the new route")
	}
}

func TestUpdateRouteAtStepIfCurrentSwapsSubscriptions(t *testing.T) {
	manager := navigationsessions.NewManager()
	original := testRoute(1, 2)
	session := manager.CreateHeadless(original, original.InitialStepIndex())

	replacement := testRoute(3, 4)
	oldEdgeID, newEdgeID, ok := manager.UpdateRouteAtStepIfCurrent(
		session,
		replacement,
		replacement.InitialStepIndex(),
		session.StepIdx,
		session.RouteRevision,
	)
	if !ok {
		t.Fatal("fresh update should be accepted")
	}
	if oldEdgeID == nil || *oldEdgeID != 1 {
		t.Fatalf("old edge transition: got %v want 1", oldEdgeID)
	}
	if newEdgeID == nil || *newEdgeID != 3 {
		t.Fatalf("new edge transition: got %v want 3", newEdgeID)
	}

	if len(manager.SubscribersOf(model.EdgeID(1))) != 0 {
		t.Fatal("old route edge still has a subscriber after route replacement")
	}
	if !containsSession(manager.SubscribersOf(model.EdgeID(3)), session.ID) {
		t.Fatal("new route edge does not include the session subscriber")
	}
}

func testRoute(edgeIDs ...uint32) routingentities.Route {
	steps := make([]routingentities.Step, 0, len(edgeIDs)+1)
	steps = append(steps, routingentities.Step{NodeIdx: 0})
	for i, edgeID := range edgeIDs {
		edgeID := edgeID
		steps = append(steps, routingentities.Step{
			NodeIdx: uint32(i + 1),
			EdgeID:  &edgeID,
		})
	}
	return routingentities.Route{Steps: steps}
}

func containsSession(sessionIDs []string, target string) bool {
	for _, sessionID := range sessionIDs {
		if sessionID == target {
			return true
		}
	}
	return false
}
