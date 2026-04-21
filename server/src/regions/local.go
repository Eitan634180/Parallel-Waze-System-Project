// Package regions manages local built regions, remote region catalogs, and
// interactive/non-interactive region selection flows.
package regions

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
)

// requiredBinFiles are the six binary files that must be present for a region
// to be considered fully built and ready to load.
var requiredBinFiles = []string{
	"nodes.bin",
	"edges.bin",
	"base_adj.bin",
	"cells.bin",
	"boundary.bin",
	"overlay_adj.bin",
}

// RegionInfo describes a single ready-to-load region on disk.
type RegionInfo struct {
	// ID is the Geofabrik slug used as the subdirectory name.
	ID string
	// Dir is the region directory path under the configured map root.
	Dir string
}

// IsReady reports whether dir contains all required binary graph files.
func IsReady(dir string) bool {
	for _, name := range requiredBinFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
			return false
		} else if err != nil {
			return false
		}
	}
	return true
}

// ListReady walks the map tree and returns every ready region below root.
// It returns nil if root does not exist.
func ListReady(root string) ([]RegionInfo, error) {
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	var regions []RegionInfo
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || !entry.IsDir() {
			return nil
		}
		if !IsReady(path) {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		regions = append(regions, RegionInfo{
			ID:  filepath.ToSlash(relPath),
			Dir: path,
		})
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(regions, func(i, j int) bool {
		return regions[i].ID < regions[j].ID
	})
	return regions, nil
}
