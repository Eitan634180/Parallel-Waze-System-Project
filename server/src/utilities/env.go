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

// RequireEnv returns a non-empty trimmed env var value or panics with a setup error.
func RequireEnv(name string) string {
	value, ok := LookupEnvTrimmed(name)
	if !ok {
		panic(fmt.Sprintf("%s must be set in the environment", name))
	}
	return value
}
