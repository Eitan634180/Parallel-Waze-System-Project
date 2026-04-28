package utilities

import (
	"fmt"
	"os"
	"path/filepath"
)

const defaultModuleSibling = "server"

// FindModuleRoot locates the server module root by searching for go.mod.
func FindModuleRoot() (string, error) {
	candidates := make([]string, 0, 4)
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd, filepath.Join(cwd, defaultModuleSibling))
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, exeDir, filepath.Join(exeDir, defaultModuleSibling))
	}

	for _, candidate := range candidates {
		if root, ok := findGoModUp(candidate); ok {
			return root, nil
		}
	}

	return "", fmt.Errorf("go.mod not found from cwd or executable path")
}

// ResolveModulePath resolves a possibly-relative path against the server module root.
func ResolveModulePath(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}

	root, err := FindModuleRoot()
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(root, filepath.FromSlash(value))), nil
}

// ResolveMapPath resolves a path relative to a provided map root unless it is already absolute.
func ResolveMapPath(mapRoot, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(mapRoot, filepath.FromSlash(value)))
}

func findGoModUp(start string) (string, bool) {
	dir := filepath.Clean(start)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
