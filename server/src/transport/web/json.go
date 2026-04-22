package transportweb

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

const jsonContentType = "application/json"

func MethodNotAllowed(w http.ResponseWriter) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func DecodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var payload T
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return payload, false
	}
	return payload, true
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set(headerContentType, jsonContentType)
	if status > 0 {
		w.WriteHeader(status)
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func LogSlowOperation(threshold time.Duration, start time.Time, format string, args ...any) {
	elapsed := time.Since(start)
	if elapsed < threshold {
		return
	}

	logArgs := append(args, elapsed.Round(time.Millisecond))
	log.Printf(format+" in %s", logArgs...)
}
