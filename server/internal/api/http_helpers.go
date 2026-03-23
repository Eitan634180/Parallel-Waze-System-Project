package api

import (
	"encoding/json"
	"net/http"
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
