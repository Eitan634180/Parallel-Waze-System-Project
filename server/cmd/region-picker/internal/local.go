package picker

import "nav-system/src/mapdata"

type RegionInfo = mapdata.RegionInfo

func IsReady(dir string) bool {
	return mapdata.IsReady(dir)
}

func ListReady(root string) ([]RegionInfo, error) {
	return mapdata.ListReady(root)
}
