package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestSyncNonGitProcess_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "sync-nongit",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	mock.mounted["sync-nongit"] = true

	err := syncNonGit(context.Background(), ovl, mock, store, true)
	if err != nil {
		t.Fatalf("syncNonGit dry-run failed: %v", err)
	}

	// Should still be mounted (dry run)
	if !mock.mounted["sync-nongit"] {
		t.Error("overlay should still be mounted after dry run")
	}
}

func TestSyncNonGit_Remount(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "sync-remount",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	mock.mounted["sync-remount"] = true

	err := syncNonGit(context.Background(), ovl, mock, store, false)
	if err != nil {
		t.Fatalf("syncNonGit remount failed: %v", err)
	}

	// Should be mounted again after remount
	if !mock.mounted["sync-remount"] {
		t.Error("overlay should be mounted after remount")
	}
}

func TestSyncNonGit_UnmountFails(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	mock.unmountErr = os.ErrPermission

	ovl := &api.Overlay{
		Name:       "sync-fail",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		CreatedAt:  time.Now(),
	}
	mock.mounted["sync-fail"] = true

	err := syncNonGit(context.Background(), ovl, mock, store, false)
	if err == nil {
		t.Error("expected error when unmount fails")
	}
}
