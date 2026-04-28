package transport

import (
	"os"
	"strconv"
	"strings"
	"time"

	"nav-system/src/utilities"
)

const (
	HTTPClientTimeout               = 5 * time.Second
	RouteCacheGCInterval            = 5 * time.Minute
	SlowRouteRequestLogThreshold    = 150 * time.Millisecond
	SlowSearchRequestLogThreshold   = 300 * time.Millisecond
	SlowSessionCreationLogThreshold = 50 * time.Millisecond
	BaseRouteCount                  = 1
	MaxRouteCount                   = 5

	SearchDefaultLimit    = 5
	SearchDefaultLanguage = ""
)

type SearchConfig struct {
	Limit        int
	Language     string
	CountryCodes string
	ViewBox      string
	UpstreamURL  string
	UserAgent    string
}

func LoadSearchConfigFromEnv() SearchConfig {
	cfg := SearchConfig{
		Limit:       SearchDefaultLimit,
		Language:    SearchDefaultLanguage,
		UpstreamURL: utilities.RequireEnv("NAV_SEARCH_UPSTREAM_URL"),
		UserAgent:   utilities.RequireEnv("NAV_SEARCH_USER_AGENT"),
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
