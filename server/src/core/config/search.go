package config

import (
	"os"
	"strconv"
	"strings"
)

const (
	SearchDefaultLimit    = 5
	SearchDefaultLanguage = ""
)

type SearchConfig struct {
	Limit        int
	Language     string
	CountryCodes string
	ViewBox      string
}

func LoadSearchConfigFromEnv() SearchConfig {
	cfg := SearchConfig{
		Limit:    SearchDefaultLimit,
		Language: SearchDefaultLanguage,
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
