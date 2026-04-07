package integration_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"nav-system/test/testutil"
)

func FuzzSessionPathHandlingDoesNotPanic(f *testing.F) {
	fixture := testutil.BuildServerFixture(f, "diamond_graph.json", 2)
	f.Add("/session")
	f.Add("/session/")
	f.Add("/session/test")
	f.Add("/session/test/ws")
	f.Add("/session/test/ws/extra")

	f.Fuzz(func(t *testing.T, path string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		fixture.Server.ServeHTTP(rec, req)
		if rec.Code >= http.StatusInternalServerError {
			t.Fatalf("unexpected server error for path %q: %d", path, rec.Code)
		}
	})
}
