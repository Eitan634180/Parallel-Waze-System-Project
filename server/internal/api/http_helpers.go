package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

const (
	slowRouteRequestLogThreshold      = 150 * time.Millisecond
	slowSearchRequestLogThreshold     = 300 * time.Millisecond
	slowSimulationRouteLogThreshold   = 250 * time.Millisecond
	slowSessionCreationLogThreshold   = 50 * time.Millisecond
)

func methodNotAllowed(w http.ResponseWriter) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var payload T
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return payload, false
	}
	return payload, true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if status > 0 {
		w.WriteHeader(status)
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func logSlowOperation(threshold time.Duration, start time.Time, format string, args ...any) {
	elapsed := time.Since(start)
	if elapsed < threshold {
		return
	}

	logArgs := append(args, elapsed.Round(time.Millisecond))
	log.Printf(format+" in %s", logArgs...)
}
