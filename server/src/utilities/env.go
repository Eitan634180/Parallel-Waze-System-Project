package utilities

import (
	"fmt"
	"os"
	"strings"
)

// LookupEnvTrimmed returns a trimmed env var value and whether it is non-empty.
func LookupEnvTrimmed(name string) (string, bool) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", false
	}
	return value, true
}

// LookupAnyEnvTrimmed returns the first non-empty trimmed env var value.
func LookupAnyEnvTrimmed(names ...string) (string, bool) {
	for _, name := range names {
		if value, ok := LookupEnvTrimmed(name); ok {
			return value, true
		}
	}
	return "", false
}

// RequireEnv returns a non-empty trimmed env var value or panics with a setup error.
func RequireEnv(name string) string {
	value, ok := LookupEnvTrimmed(name)
	if !ok {
		panic(fmt.Sprintf("%s must be set in the environment", name))
	}
	return value
}

// RequireAnyEnv returns the first non-empty trimmed env var value or panics.
func RequireAnyEnv(names ...string) string {
	value, ok := LookupAnyEnvTrimmed(names...)
	if !ok {
		panic(fmt.Sprintf("%s must be set in the environment", strings.Join(names, " or ")))
	}
	return value
}
