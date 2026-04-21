package apihttp

import (
	"net"
	"net/url"
	"os"
	"strings"
)

const originNotAllowedMessage = "origin not allowed"

var allowedOriginOverrides = loadAllowedOriginOverrides()

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
