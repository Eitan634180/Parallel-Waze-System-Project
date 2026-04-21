package apihttp

import "net/http"

const (
	headerOrigin      = "Origin"
	headerVary        = "Vary"
	headerContentType = "Content-Type"
	headerAllowOrigin = "Access-Control-Allow-Origin"
	headerAllowMethods = "Access-Control-Allow-Methods"
	headerAllowHeaders = "Access-Control-Allow-Headers"
	corsAllowedMethods = "GET,POST,DELETE,OPTIONS"
	corsAllowedHeaders = headerContentType
)

func WithCORS(originAllowed func(string) bool, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get(headerOrigin)
		if origin != "" {
			if !originAllowed(origin) {
				http.Error(w, originNotAllowedMessage, http.StatusForbidden)
				return
			}
			w.Header().Set(headerAllowOrigin, origin)
			w.Header().Set(headerVary, headerOrigin)
			w.Header().Set(headerAllowMethods, corsAllowedMethods)
			w.Header().Set(headerAllowHeaders, corsAllowedHeaders)
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}
