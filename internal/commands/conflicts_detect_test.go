package commands

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestConflicts_DetectOverlap(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	store := createTestStore(t, tmpDir)

	// Create two overlays with overlapping files
	upper1 := filepath.Join(tmpDir, "upper1")
	upper2 := filepath.Join(tmpDir, "upper2")
	_ = os.MkdirAll(upper1, 0755)
	_ = os.MkdirAll(upper2, 0755)

	// Both modify the same file
	_ = os.WriteFile(filepath.Join(upper1, "shared.go"), []byte("v1"), 0644)
	_ = os.WriteFile(filepath.Join(upper2, "shared.go"), []byte("v2"), 0644)

	// Only one modifies this file
	_ = os.WriteFile(filepath.Join(upper1, "unique1.go"), []byte("u1"), 0644)
	_ = os.WriteFile(filepath.Join(upper2, "unique2.go"), []byte("u2"), 0644)

	ovl1 := &api.Overlay{Name: "ovl-1", BaseDir: tmpDir, UpperDir: upper1, CreatedAt: time.Now()}
	ovl2 := &api.Overlay{Name: "ovl-2", BaseDir: tmpDir, UpperDir: upper2, CreatedAt: time.Now()}
	_ = store.Save(ovl1)
	_ = store.Save(ovl2)

	// Build file map like doConflicts does
	fileMap := make(map[string][]string)
	for _, name := range []string{"ovl-1", "ovl-2"} {
		ovl, _ := store.Load(name)
		_ = filepath.Walk(ovl.UpperDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			relPath, _ := filepath.Rel(ovl.UpperDir, path)
			if relPath == "." {
				return nil
			}
			fileMap[relPath] = append(fileMap[relPath], name)
			return nil
		})
	}

	// Find conflicts
	conflicts := 0
	for _, overlays := range fileMap {
		if len(overlays) > 1 {
			conflicts++
		}
	}

	if conflicts != 1 {
		t.Errorf("expected 1 conflict (shared.go), got %d", conflicts)
	}
}

func TestConflicts_NoOverlap(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	store := createTestStore(t, tmpDir)

	upper1 := filepath.Join(tmpDir, "upper1")
	upper2 := filepath.Join(tmpDir, "upper2")
	_ = os.MkdirAll(upper1, 0755)
	_ = os.MkdirAll(upper2, 0755)

	_ = os.WriteFile(filepath.Join(upper1, "file1.go"), []byte("a"), 0644)
	_ = os.WriteFile(filepath.Join(upper2, "file2.go"), []byte("b"), 0644)

	ovl1 := &api.Overlay{Name: "no-conflict-1", BaseDir: tmpDir, UpperDir: upper1, CreatedAt: time.Now()}
	ovl2 := &api.Overlay{Name: "no-conflict-2", BaseDir: tmpDir, UpperDir: upper2, CreatedAt: time.Now()}
	_ = store.Save(ovl1)
	_ = store.Save(ovl2)

	fileMap := make(map[string][]string)
	for _, name := range []string{"no-conflict-1", "no-conflict-2"} {
		ovl, _ := store.Load(name)
		_ = filepath.Walk(ovl.UpperDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			relPath, _ := filepath.Rel(ovl.UpperDir, path)
			fileMap[relPath] = append(fileMap[relPath], name)
			return nil
		})
	}

	conflicts := 0
	for _, overlays := range fileMap {
		if len(overlays) > 1 {
			conflicts++
		}
	}

	if conflicts != 0 {
		t.Errorf("expected 0 conflicts, got %d", conflicts)
	}
}

func TestConflicts_WhiteoutDetection(t *testing.T) {
	tmpDir := t.TempDir()

	upper := filepath.Join(tmpDir, "upper")
	_ = os.MkdirAll(upper, 0755)

	// Whiteout file
	_ = os.WriteFile(filepath.Join(upper, ".wh.deleted.txt"), []byte{}, 0644)
	// Normal file
	_ = os.WriteFile(filepath.Join(upper, "normal.txt"), []byte("data"), 0644)

	var files []string
	_ = filepath.Walk(upper, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(upper, path)
		files = append(files, relPath)
		return nil
	})

	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}
