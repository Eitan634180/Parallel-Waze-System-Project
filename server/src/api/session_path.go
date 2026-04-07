package api

import "strings"

func parseSessionPath(path string) (sessionID, subpath string, ok bool) {
	trimmed := strings.TrimPrefix(path, sessionPathPrefix)
	parts := strings.SplitN(trimmed, sessionPathSeparator, sessionPathSplitLimit)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}

	if len(parts) == 2 {
		return parts[0], parts[1], true
	}
	return parts[0], "", true
}
