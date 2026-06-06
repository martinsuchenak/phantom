package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestProcessStop_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "stop-test",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)
	mock.mounted["stop-test"] = true

	err := processStop(context.Background(), "stop-test", false, false, false)
	if err != nil {
		t.Fatalf("processStop failed: %v", err)
	}

	if mock.mounted["stop-test"] {
		t.Error("expected overlay to be unmounted")
	}
}

func TestProcessStop_WithCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "stop-cleanup",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)
	mock.mounted["stop-cleanup"] = true

	err := processStop(context.Background(), "stop-cleanup", true, false, false)
	if err != nil {
		t.Fatalf("processStop with cleanup failed: %v", err)
	}

	// State should be deleted
	if store.Exists("stop-cleanup") {
		t.Error("expected overlay state to be deleted after cleanup")
	}
}

func TestProcessStop_LockedWithCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "stop-locked",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		Locked:     true,
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)

	err := processStop(context.Background(), "stop-locked", true, false, false)
	if err == nil {
		t.Error("expected error when stopping locked overlay with cleanup")
	}
}

func TestProcessStop_LockedWithForce(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "stop-force",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		Locked:     true,
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)
	mock.mounted["stop-force"] = true

	err := processStop(context.Background(), "stop-force", true, false, true)
	if err != nil {
		t.Fatalf("processStop with force should succeed: %v", err)
	}
}

func TestProcessStop_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)
	createTestStore(t, tmpDir)

	err := processStop(context.Background(), "nonexistent", false, false, false)
	if err == nil {
		t.Error("expected error for nonexistent overlay")
	}
}

func TestProcessRestart(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "restart-test",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)
	mock.mounted["restart-test"] = false // unmounted

	err := processRestart("restart-test")
	if err != nil {
		t.Fatalf("processRestart failed: %v", err)
	}

	if !mock.mounted["restart-test"] {
		t.Error("expected overlay to be mounted after restart")
	}
}

func TestProcessRestart_AlreadyMounted(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "restart-mounted",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)
	mock.mounted["restart-mounted"] = true

	err := processRestart("restart-mounted")
	if err != nil {
		t.Fatalf("processRestart already mounted should succeed: %v", err)
	}
}

func TestProcessRestart_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)
	createTestStore(t, tmpDir)

	err := processRestart("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent overlay")
	}
}

func TestProcessPrune_NoOverlays(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)
	createTestStore(t, tmpDir)

	err := processPrune(false, false)
	if err != nil {
		t.Fatalf("processPrune with no overlays failed: %v", err)
	}
}

func TestProcessPrune_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	// Create an unmounted overlay
	ovl := &api.Overlay{
		Name:       "prune-target",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now().Add(-30 * 24 * time.Hour), // 30 days old
	}
	_ = store.Save(ovl)
	mock.mounted["prune-target"] = false

	err := processPrune(true, false)
	if err != nil {
		t.Fatalf("processPrune dry-run failed: %v", err)
	}

	// Should still exist (dry run)
	if !store.Exists("prune-target") {
		t.Error("overlay should still exist after dry run")
	}
}

func TestProcessPrune_RemovesUnmounted(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "prune-unmounted",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)
	mock.mounted["prune-unmounted"] = false

	err := processPrune(false, false)
	if err != nil {
		t.Fatalf("processPrune failed: %v", err)
	}

	if store.Exists("prune-unmounted") {
		t.Error("unmounted overlay should be pruned")
	}
}

func TestProcessPrune_SkipsPersistent(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "prune-persistent",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		Persistent: true,
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)
	mock.mounted["prune-persistent"] = false

	err := processPrune(false, false)
	if err != nil {
		t.Fatalf("processPrune failed: %v", err)
	}

	if !store.Exists("prune-persistent") {
		t.Error("persistent overlay should not be pruned")
	}
}

func TestProcessPrune_SkipsLocked(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "prune-locked",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		Locked:     true,
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)
	mock.mounted["prune-locked"] = false

	err := processPrune(false, false)
	if err != nil {
		t.Fatalf("processPrune failed: %v", err)
	}

	if !store.Exists("prune-locked") {
		t.Error("locked overlay should not be pruned")
	}
}

func TestProcessPrune_ForceRemovesMounted(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "prune-force",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now().Add(-30 * 24 * time.Hour), // expired
	}
	_ = store.Save(ovl)
	mock.mounted["prune-force"] = true

	err := processPrune(false, true)
	if err != nil {
		t.Fatalf("processPrune force failed: %v", err)
	}

	if store.Exists("prune-force") {
		t.Error("expired mounted overlay should be pruned with --force")
	}
}

func TestShowAllStatus(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	// Create some overlays
	for _, name := range []string{"status-1", "status-2"} {
		ovl := &api.Overlay{
			Name:       name,
			BaseDir:    tmpDir,
			MountPoint: filepath.Join(tmpDir, "mnt", name),
			UpperDir:   filepath.Join(tmpDir, "upper", name),
			CreatedAt:  time.Now(),
		}
		_ = os.MkdirAll(ovl.UpperDir, 0755)
		_ = store.Save(ovl)
		mock.mounted[name] = true
	}

	t.Run("table format", func(t *testing.T) {
		err := showAllStatus(store, mock, "table")
		if err != nil {
			t.Fatalf("showAllStatus table failed: %v", err)
		}
	})

	t.Run("json format", func(t *testing.T) {
		err := showAllStatus(store, mock, "json")
		if err != nil {
			t.Fatalf("showAllStatus json failed: %v", err)
		}
	})
}

func TestShowAllStatus_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	err := showAllStatus(store, mock, "table")
	if err != nil {
		t.Fatalf("showAllStatus empty failed: %v", err)
	}
}

func TestProcessHealth(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	// Create an overlay with missing base dir
	ovl := &api.Overlay{
		Name:       "health-test",
		BaseDir:    filepath.Join(tmpDir, "nonexistent-base"),
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)
	mock.mounted["health-test"] = false

	err := processHealth(context.Background(), "table", false)
	if err != nil {
		t.Fatalf("processHealth failed: %v", err)
	}
}

func TestProcessHealth_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)
	createTestStore(t, tmpDir)

	err := processHealth(context.Background(), "json", false)
	if err != nil {
		t.Fatalf("processHealth json failed: %v", err)
	}
}

func TestForceUnmount(t *testing.T) {
	mock := newMockOverlayManager()
	ovl := &api.Overlay{Name: "force-test"}
	mock.mounted["force-test"] = true

	err := forceUnmount(mock, ovl)
	if err != nil {
		t.Fatalf("forceUnmount failed: %v", err)
	}
}

func TestDoPrune(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	oldLog := log
	log = &MockLogger{}
	defer func() {
		log = oldLog
	}()

	cmd := NewPruneCommand()

	runCommandWithArgs(t, []string{"prune", "--dry-run", "--force"}, func() {
		err := doPrune(context.Background(), cmd)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
