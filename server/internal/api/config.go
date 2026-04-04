package api

import (
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	defaultSearchLimit        = 5
	defaultSearchLanguage     = "he"
	defaultSearchCountryCodes = "il,ps"
	defaultSearchViewBox      = "34.15,33.45,35.90,29.45"
)

type searchConfig struct {
	Limit        int
	Language     string
	CountryCodes string
	ViewBox      string
}

var allowedOriginOverrides = loadAllowedOriginOverrides()

func loadSearchConfig() searchConfig {
	cfg := searchConfig{
		Limit:        defaultSearchLimit,
		Language:     defaultSearchLanguage,
		CountryCodes: defaultSearchCountryCodes,
		ViewBox:      defaultSearchViewBox,
	}

	if v := strings.TrimSpace(os.Getenv("NAV_SEARCH_LIMIT")); v != "" {
		if limit, err := strconv.Atoi(v); err == nil && limit > 0 {
			cfg.Limit = limit
		}
	}
	if v := strings.TrimSpace(os.Getenv("NAV_SEARCH_LANGUAGE")); v != "" {
		cfg.Language = v
	}
	if v := strings.TrimSpace(os.Getenv("NAV_SEARCH_COUNTRY_CODES")); v != "" {
		cfg.CountryCodes = v
	}
	if v := strings.TrimSpace(os.Getenv("NAV_SEARCH_VIEWBOX")); v != "" {
		cfg.ViewBox = v
	}

	return cfg
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

func isAllowedBrowserOrigin(origin string) bool {
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
