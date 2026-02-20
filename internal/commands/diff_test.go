package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountFileChanges(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-diff-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	upperDir := filepath.Join(tmpDir, "upper")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(upperDir, 0755)

	// Create base files
	os.WriteFile(filepath.Join(baseDir, "existing.txt"), []byte("hello"), 0644)

	// Upper: modified file (exists in base)
	os.WriteFile(filepath.Join(upperDir, "existing.txt"), []byte("modified"), 0644)
	// Upper: added file (not in base)
	os.WriteFile(filepath.Join(upperDir, "new.txt"), []byte("new"), 0644)
	// Upper: whiteout (deletion marker)
	os.WriteFile(filepath.Join(upperDir, ".wh.deleted.txt"), []byte{}, 0644)

	added, modified, deleted := countFileChanges(upperDir, baseDir)

	if added != 1 {
		t.Errorf("expected 1 added, got %d", added)
	}
	if modified != 1 {
		t.Errorf("expected 1 modified, got %d", modified)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}
}

func TestCountFileChanges_EmptyUpperDir(t *testing.T) {
	added, modified, deleted := countFileChanges("", "/tmp/base")
	if added != 0 || modified != 0 || deleted != 0 {
		t.Errorf("expected all zeros for empty upper dir, got %d/%d/%d", added, modified, deleted)
	}
}

func TestCountFileChanges_SkipsWorkDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-diff-work-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	upperDir := filepath.Join(tmpDir, "upper")
	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(filepath.Join(upperDir, "work"), 0755)
	os.WriteFile(filepath.Join(upperDir, "work", "internal.txt"), []byte("skip"), 0644)
	os.WriteFile(filepath.Join(upperDir, "real.txt"), []byte("count"), 0644)

	added, modified, deleted := countFileChanges(upperDir, baseDir)
	if added != 1 {
		t.Errorf("expected 1 added (skipping work/), got %d", added)
	}
	if modified != 0 || deleted != 0 {
		t.Errorf("expected 0 modified/deleted, got %d/%d", modified, deleted)
	}
}

func TestProcessDiff(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-processdiff-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	upperDir := filepath.Join(tmpDir, "state", "overlays", "test-diff", "upper")
	mountDir := filepath.Join(tmpDir, "state", "mnt", "test-diff")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(upperDir, 0755)
	os.MkdirAll(mountDir, 0755)

	// Create base file and modified version in upper
	os.WriteFile(filepath.Join(baseDir, "file.txt"), []byte("original"), 0644)
	os.WriteFile(filepath.Join(upperDir, "file.txt"), []byte("changed"), 0644)
	os.WriteFile(filepath.Join(upperDir, "added.txt"), []byte("new"), 0644)

	// Save overlay state
	store := createTestStore(t, tmpDir)
	ovl := testOverlay("test-diff", baseDir, mountDir, upperDir)
	store.Save(&ovl)

	// Test all formats
	for _, format := range []string{"table", "json", "simple"} {
		if err := processDiff("test-diff", format, false); err != nil {
			t.Errorf("processDiff(%s) failed: %v", format, err)
		}
	}

	// Test stat-only
	if err := processDiff("test-diff", "table", true); err != nil {
		t.Errorf("processDiff stat-only failed: %v", err)
	}
}
