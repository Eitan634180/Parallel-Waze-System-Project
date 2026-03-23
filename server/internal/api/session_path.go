package api

import "strings"

func parseSessionPath(path string) (sessionID, subpath string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/session/")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}

	if len(parts) == 2 {
		return parts[0], parts[1], true
	}
	return parts[0], "", true
}
