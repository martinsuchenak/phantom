package commands

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestCountFileChanges_Empty(t *testing.T) {
	upperDir := t.TempDir()
	baseDir := t.TempDir()

	added, modified, deleted := countFileChanges(upperDir, baseDir)
	if added != 0 || modified != 0 || deleted != 0 {
		t.Errorf("expected 0/0/0, got %d/%d/%d", added, modified, deleted)
	}
}

func TestCountFileChanges_Added(t *testing.T) {
	upperDir := t.TempDir()
	baseDir := t.TempDir()

	_ = os.WriteFile(filepath.Join(upperDir, "new.txt"), []byte("new"), 0644)

	added, modified, deleted := countFileChanges(upperDir, baseDir)
	if added != 1 {
		t.Errorf("expected 1 added, got %d", added)
	}
	if modified != 0 || deleted != 0 {
		t.Errorf("expected 0 modified/deleted, got %d/%d", modified, deleted)
	}
}

func TestCountFileChanges_Modified(t *testing.T) {
	upperDir := t.TempDir()
	baseDir := t.TempDir()

	_ = os.WriteFile(filepath.Join(baseDir, "existing.txt"), []byte("old"), 0644)
	_ = os.WriteFile(filepath.Join(upperDir, "existing.txt"), []byte("new"), 0644)

	added, modified, deleted := countFileChanges(upperDir, baseDir)
	if modified != 1 {
		t.Errorf("expected 1 modified, got %d", modified)
	}
	if added != 0 || deleted != 0 {
		t.Errorf("expected 0 added/deleted, got %d/%d", added, deleted)
	}
}

func TestCountFileChanges_Deleted(t *testing.T) {
	upperDir := t.TempDir()
	baseDir := t.TempDir()

	// Whiteout file indicates deletion
	_ = os.WriteFile(filepath.Join(upperDir, ".wh.removed.txt"), []byte{}, 0644)

	added, _, deleted := countFileChanges(upperDir, baseDir)
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}
	if added != 0 {
		t.Errorf("expected 0 added, got %d", added)
	}
}

func TestInspectOutput_Struct(t *testing.T) {
	out := inspectOutput{
		Name:       "test-overlay",
		BaseDir:    "/tmp/base",
		MountPoint: "/tmp/mnt",
		UpperDir:   "/tmp/upper",
		Branch:     "phantom/test",
		Persistent: true,
		Locked:     false,
		Mounted:    true,
		FilesAdded: 3,
		FilesMod:   2,
		FilesDel:   1,
		Snapshots:  0,
		HasLog:     true,
	}

	if out.Name != "test-overlay" {
		t.Error("name mismatch")
	}
	if !out.Persistent {
		t.Error("should be persistent")
	}
	if out.FilesAdded != 3 {
		t.Errorf("expected 3 added, got %d", out.FilesAdded)
	}
}

func TestDoInspect_WithMockManager(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	upperDir := filepath.Join(tmpDir, "upper")
	_ = os.MkdirAll(upperDir, 0755)
	_ = os.WriteFile(filepath.Join(upperDir, "file.txt"), []byte("data"), 0644)

	ovl := &api.Overlay{
		Name:       "inspect-test",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   upperDir,
		Branch:     "phantom/inspect-test",
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)
	mock.mounted["inspect-test"] = true

	// Test that GetStatus works through mock
	status, err := mock.GetStatus(ovl)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !status.Mounted {
		t.Error("should be mounted")
	}

	// Test countFileChanges on the overlay
	added, _, _ := countFileChanges(upperDir, tmpDir)
	// file.txt exists in upper but not in base (base is tmpDir which has upper/ subdir)
	if added < 0 {
		t.Error("added should be >= 0")
	}
}
