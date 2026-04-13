package integration_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"nav-system/test/testutil"
)

func FuzzRouteEndpointRejectsGarbageWithoutPanicking(f *testing.F) {
	fixture := testutil.BuildServerFixture(f, "diamond_graph.json", 2)
	f.Add([]byte(`{"src_lat":32.0}`))
	f.Add([]byte(`not-json`))
	f.Add([]byte(`{"src_lat":true}`))

	f.Fuzz(func(t *testing.T, payload []byte) {
		req := httptest.NewRequest(http.MethodPost, "/route", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		fixture.Server.ServeHTTP(rec, req)
		if rec.Code >= http.StatusInternalServerError {
			t.Fatalf("unexpected server error for payload %q: %d", string(payload), rec.Code)
		}
	})
}
