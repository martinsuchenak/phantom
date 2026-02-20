package commands

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestConflictDetection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-conflicts-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)

	// Create two overlays with overlapping changes
	upper1 := filepath.Join(tmpDir, "state", "overlays", "ovl1", "upper")
	upper2 := filepath.Join(tmpDir, "state", "overlays", "ovl2", "upper")
	mount1 := filepath.Join(tmpDir, "state", "mnt", "ovl1")
	mount2 := filepath.Join(tmpDir, "state", "mnt", "ovl2")
	os.MkdirAll(upper1, 0755)
	os.MkdirAll(upper2, 0755)
	os.MkdirAll(mount1, 0755)
	os.MkdirAll(mount2, 0755)

	// Both modify the same file
	os.WriteFile(filepath.Join(upper1, "shared.txt"), []byte("version1"), 0644)
	os.WriteFile(filepath.Join(upper2, "shared.txt"), []byte("version2"), 0644)
	// Only ovl1 modifies this
	os.WriteFile(filepath.Join(upper1, "unique1.txt"), []byte("only1"), 0644)
	// Only ovl2 modifies this
	os.WriteFile(filepath.Join(upper2, "unique2.txt"), []byte("only2"), 0644)

	store := createTestStore(t, tmpDir)
	store.Save(&api.Overlay{
		Name: "ovl1", BaseDir: baseDir, MountPoint: mount1,
		UpperDir: upper1, CreatedAt: time.Now(),
	})
	store.Save(&api.Overlay{
		Name: "ovl2", BaseDir: baseDir, MountPoint: mount2,
		UpperDir: upper2, CreatedAt: time.Now(),
	})

	// Build file map manually (same logic as doConflicts)
	fileMap := make(map[string][]string)
	for _, name := range []string{"ovl1", "ovl2"} {
		ovl, _ := store.Load(name)
		filepath.Walk(ovl.UpperDir, func(path string, info os.FileInfo, err error) error {
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

	conflicts := 0
	for _, overlays := range fileMap {
		if len(overlays) > 1 {
			conflicts++
		}
	}

	if conflicts != 1 {
		t.Errorf("expected 1 conflict (shared.txt), got %d", conflicts)
	}
}

func TestConflictDetection_NoConflicts(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-noconflict-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	upper1 := filepath.Join(tmpDir, "upper1")
	upper2 := filepath.Join(tmpDir, "upper2")
	os.MkdirAll(upper1, 0755)
	os.MkdirAll(upper2, 0755)

	os.WriteFile(filepath.Join(upper1, "file1.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(upper2, "file2.txt"), []byte("b"), 0644)

	fileMap := make(map[string][]string)
	for i, dir := range []string{upper1, upper2} {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			relPath, _ := filepath.Rel(dir, path)
			name := []string{"ovl1", "ovl2"}[i]
			fileMap[relPath] = append(fileMap[relPath], name)
			return nil
		})
	}

	for _, overlays := range fileMap {
		if len(overlays) > 1 {
			t.Error("expected no conflicts")
		}
	}
}
