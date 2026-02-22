package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestSyncNonGit_Basic(t *testing.T) {
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
	store.Save(ovl)
	mock.mounted["sync-nongit"] = true

	err := syncNonGit(context.Background(), ovl, mock, store, false)
	if err != nil {
		t.Fatalf("syncNonGit failed: %v", err)
	}

	// Should have unmounted and remounted
	if !mock.mounted["sync-nongit"] {
		t.Error("overlay should be mounted after sync")
	}
}

func TestSyncNonGit_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "sync-dry",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	store.Save(ovl)
	mock.mounted["sync-dry"] = true

	err := syncNonGit(context.Background(), ovl, mock, store, true)
	if err != nil {
		t.Fatalf("syncNonGit dry-run failed: %v", err)
	}
}

func TestSyncNonGit_UnmountError(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "sync-uerr",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	store.Save(ovl)
	mock.mounted["sync-uerr"] = true
	mock.unmountErr = fmt.Errorf("unmount failed")

	err := syncNonGit(context.Background(), ovl, mock, store, false)
	if err == nil {
		t.Error("expected error when unmount fails")
	}
}
