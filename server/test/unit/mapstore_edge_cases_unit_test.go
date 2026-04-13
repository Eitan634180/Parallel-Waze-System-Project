package unit_test

import (
	"os"
	"path/filepath"
	"testing"

	"nav-system/src/mapstore"
)

// ──────────────────────────────────────────────
// IsReady edge cases
// ──────────────────────────────────────────────

func TestMapstoreIsReadyReturnsFalseForNonExistentDir(t *testing.T) {
	if mapstore.IsReady(filepath.Join(t.TempDir(), "does-not-exist")) {
		t.Fatal("non-existent directory should not be ready")
	}
}

func TestMapstoreIsReadyReturnsFalseWhenEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if mapstore.IsReady(dir) {
		t.Fatal("empty directory should not be ready")
	}
}

func TestMapstoreIsReadyReturnsFalseWhenOneFileMissing(t *testing.T) {
	dir := t.TempDir()
	allFiles := []string{
		"nodes.bin", "edges.bin", "base_adj.bin",
		"cells.bin", "boundary.bin", "overlay_adj.bin",
	}
	// Write all except the last one.
	for _, f := range allFiles[:len(allFiles)-1] {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	if mapstore.IsReady(dir) {
		t.Fatal("directory missing one file should not be ready")
	}
}

// ──────────────────────────────────────────────
// ListReady edge cases
// ──────────────────────────────────────────────

func TestMapstoreListReadyReturnsNilForNonExistentRoot(t *testing.T) {
	regions, err := mapstore.ListReady(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Fatalf("ListReady on missing root should not error, got: %v", err)
	}
	if regions != nil {
		t.Fatalf("ListReady on missing root should return nil, got %v", regions)
	}
}

func TestMapstoreListReadyReturnsNilForEmptyRoot(t *testing.T) {
	root := t.TempDir()
	regions, err := mapstore.ListReady(root)
	if err != nil {
		t.Fatalf("ListReady on empty root should not error: %v", err)
	}
	if len(regions) != 0 {
		t.Fatalf("empty root should produce no regions, got %d", len(regions))
	}
}

func TestMapstoreListReadyIsAlphabeticallySorted(t *testing.T) {
	root := t.TempDir()
	required := []string{
		"nodes.bin", "edges.bin", "base_adj.bin",
		"cells.bin", "boundary.bin", "overlay_adj.bin",
	}
	for _, name := range []string{"region-z", "region-a", "region-m"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		for _, f := range required {
			if err := os.WriteFile(filepath.Join(dir, f), []byte("ok"), 0o644); err != nil {
				t.Fatalf("write %s/%s: %v", name, f, err)
			}
		}
	}

	regions, err := mapstore.ListReady(root)
	if err != nil {
		t.Fatalf("ListReady: %v", err)
	}
	if len(regions) != 3 {
		t.Fatalf("expected 3 regions, got %d", len(regions))
	}
	expected := []string{"region-a", "region-m", "region-z"}
	for i, want := range expected {
		if regions[i].ID != want {
			t.Fatalf("region[%d].ID = %q want %q", i, regions[i].ID, want)
		}
	}
}

func TestMapstoreListReadyDirFieldMatchesPath(t *testing.T) {
	root := t.TempDir()
	regionDir := filepath.Join(root, "my-region")
	if err := os.MkdirAll(regionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, f := range []string{"nodes.bin", "edges.bin", "base_adj.bin", "cells.bin", "boundary.bin", "overlay_adj.bin"} {
		if err := os.WriteFile(filepath.Join(regionDir, f), []byte("ok"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	regions, err := mapstore.ListReady(root)
	if err != nil {
		t.Fatalf("ListReady: %v", err)
	}
	if len(regions) != 1 {
		t.Fatalf("expected 1 region, got %d", len(regions))
	}
	if regions[0].Dir != regionDir {
		t.Fatalf("Dir field mismatch: got %q want %q", regions[0].Dir, regionDir)
	}
}
