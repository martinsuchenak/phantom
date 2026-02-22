package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestProcessCommit_NotMounted(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "commit-unmounted",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	store.Save(ovl)
	mock.mounted["commit-unmounted"] = false

	err := processCommit(context.Background(), "commit-unmounted", "test commit", false)
	if err == nil {
		t.Error("expected error for unmounted overlay")
	}
}

func TestProcessCommit_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)
	createTestStore(t, tmpDir)

	err := processCommit(context.Background(), "nonexistent", "msg", false)
	if err == nil {
		t.Error("expected error for nonexistent overlay")
	}
}

func TestProcessApply_NotMounted(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "apply-unmounted",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	store.Save(ovl)
	mock.mounted["apply-unmounted"] = false

	err := processApply(context.Background(), "apply-unmounted", false, false, false)
	if err == nil {
		t.Error("expected error for unmounted overlay")
	}
}

func TestProcessApply_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)
	createTestStore(t, tmpDir)

	err := processApply(context.Background(), "nonexistent", false, false, false)
	if err == nil {
		t.Error("expected error for nonexistent overlay")
	}
}

func TestProcessApply_FileCopy(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	mountDir := filepath.Join(tmpDir, "mnt")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(mountDir, 0755)

	// Create files in mount (simulating overlay changes)
	os.WriteFile(filepath.Join(mountDir, "new.txt"), []byte("new content"), 0644)
	os.WriteFile(filepath.Join(baseDir, "existing.txt"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(mountDir, "existing.txt"), []byte("updated"), 0644)

	ovl := &api.Overlay{
		Name:       "apply-fc",
		BaseDir:    baseDir,
		MountPoint: mountDir,
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	store.Save(ovl)
	mock.mounted["apply-fc"] = true

	// Non-git apply should use file copy
	err := processApply(context.Background(), "apply-fc", false, false, false)
	if err != nil {
		t.Fatalf("processApply file copy failed: %v", err)
	}

	// Verify new file was copied
	data, err := os.ReadFile(filepath.Join(baseDir, "new.txt"))
	if err != nil {
		t.Fatal("new.txt should be copied to base")
	}
	if string(data) != "new content" {
		t.Errorf("expected 'new content', got %q", string(data))
	}
}

func TestProcessApply_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	mountDir := filepath.Join(tmpDir, "mnt")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(mountDir, 0755)
	os.WriteFile(filepath.Join(mountDir, "new.txt"), []byte("new"), 0644)

	ovl := &api.Overlay{
		Name:       "apply-dry",
		BaseDir:    baseDir,
		MountPoint: mountDir,
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	store.Save(ovl)
	mock.mounted["apply-dry"] = true

	err := processApply(context.Background(), "apply-dry", true, false, false)
	if err != nil {
		t.Fatalf("processApply dry-run failed: %v", err)
	}

	// File should NOT be copied in dry run
	if _, err := os.Stat(filepath.Join(baseDir, "new.txt")); !os.IsNotExist(err) {
		t.Error("file should not be copied in dry run")
	}
}

func TestProcessStatus(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "status-test",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	store.Save(ovl)
	mock.mounted["status-test"] = true

	t.Run("single overlay", func(t *testing.T) {
		err := processStatus(context.Background(), "status-test", "table")
		if err != nil {
			t.Fatalf("processStatus single failed: %v", err)
		}
	})

	t.Run("single overlay json", func(t *testing.T) {
		err := processStatus(context.Background(), "status-test", "json")
		if err != nil {
			t.Fatalf("processStatus single json failed: %v", err)
		}
	})

	t.Run("all overlays", func(t *testing.T) {
		err := processStatus(context.Background(), "", "table")
		if err != nil {
			t.Fatalf("processStatus all failed: %v", err)
		}
	})

	t.Run("all overlays json", func(t *testing.T) {
		err := processStatus(context.Background(), "", "json")
		if err != nil {
			t.Fatalf("processStatus all json failed: %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		err := processStatus(context.Background(), "nonexistent", "table")
		if err == nil {
			t.Error("expected error for nonexistent overlay")
		}
	})
}
