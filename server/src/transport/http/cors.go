package transporthttp

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	headerOrigin            = "Origin"
	headerVary              = "Vary"
	headerContentType       = "Content-Type"
	headerAllowOrigin       = "Access-Control-Allow-Origin"
	headerAllowMethods      = "Access-Control-Allow-Methods"
	headerAllowHeaders      = "Access-Control-Allow-Headers"
	corsAllowedMethods      = "GET,POST,DELETE,OPTIONS"
	corsAllowedHeaders      = headerContentType
	originNotAllowedMessage = "origin not allowed"
)

var allowedOriginOverrides = loadAllowedOriginOverrides()

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

func IsAllowedBrowserOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return true
	}
	if _, ok := allowedOriginOverrides[origin]; ok {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func loadAllowedOriginOverrides() map[string]struct{} {
	raw := strings.TrimSpace(os.Getenv("NAV_ALLOWED_ORIGINS"))
	if raw == "" {
		return nil
	}

	origins := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		origins[origin] = struct{}{}
	}
	return origins
}
