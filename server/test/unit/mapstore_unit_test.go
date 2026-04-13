package unit_test

import (
	"os"
	"path/filepath"
	"testing"

	"nav-system/src/mapstore"
)

func TestMapstoreReadyAndListReady(t *testing.T) {
	root := t.TempDir()
	ready := filepath.Join(root, "region-a")
	notReady := filepath.Join(root, "region-b")

	if err := os.MkdirAll(ready, 0o755); err != nil {
		t.Fatalf("mkdir ready: %v", err)
	}
	if err := os.MkdirAll(notReady, 0o755); err != nil {
		t.Fatalf("mkdir notReady: %v", err)
	}

	required := []string{
		"nodes.bin",
		"edges.bin",
		"base_adj.bin",
		"cells.bin",
		"boundary.bin",
		"overlay_adj.bin",
	}
	for _, name := range required {
		if err := os.WriteFile(filepath.Join(ready, name), []byte("ok"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(notReady, "nodes.bin"), []byte("partial"), 0o644); err != nil {
		t.Fatalf("write partial fixture: %v", err)
	}

	if !mapstore.IsReady(ready) {
		t.Fatal("ready directory should be considered ready")
	}
	if mapstore.IsReady(notReady) {
		t.Fatal("partial directory should not be considered ready")
	}

	regions, err := mapstore.ListReady(root)
	if err != nil {
		t.Fatalf("ListReady: %v", err)
	}
	if len(regions) != 1 {
		t.Fatalf("ListReady returned %d regions, want 1", len(regions))
	}
	if regions[0].ID != "region-a" {
		t.Fatalf("ready region id = %q, want region-a", regions[0].ID)
	}
}
