package mapdata

import "nav-system/src/utilities"

func DefaultMapRoot() string {
	return utilities.RequireEnv("NAV_MAP_ROOT")
}
