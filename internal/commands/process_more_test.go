package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestProcessStart_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	_ = mock

	baseDir := filepath.Join(tmpDir, "myrepo")
	_ = os.MkdirAll(baseDir, 0755)

	err := processStart(context.Background(), baseDir, "test-start", "", false)
	if err != nil {
		t.Fatalf("processStart failed: %v", err)
	}
}

func TestProcessStart_InvalidName(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)

	err := processStart(context.Background(), tmpDir, "invalid name!", "", false)
	if err == nil {
		t.Error("expected error for invalid overlay name")
	}
}

func TestProcessStart_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)
	_ = mock

	ovl := testOverlay("existing", tmpDir, filepath.Join(tmpDir, "mnt"), filepath.Join(tmpDir, "upper"))
	_ = store.Save(&ovl)

	err := processStart(context.Background(), tmpDir, "existing", "", false)
	if err == nil {
		t.Error("expected error for already existing overlay")
	}
}

func TestProcessStart_Persistent(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)

	baseDir := filepath.Join(tmpDir, "repo")
	_ = os.MkdirAll(baseDir, 0755)

	err := processStart(context.Background(), baseDir, "persistent-test", "", true)
	if err != nil {
		t.Fatalf("processStart persistent failed: %v", err)
	}
}

func TestProcessStart_WithBranch(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)

	baseDir := filepath.Join(tmpDir, "repo")
	_ = os.MkdirAll(baseDir, 0755)

	err := processStart(context.Background(), baseDir, "branch-test", "feature/test", false)
	if err != nil {
		t.Fatalf("processStart with branch failed: %v", err)
	}
}

func TestProcessStart_InvalidBranch(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)

	err := processStart(context.Background(), tmpDir, "bad-branch", "-invalid", false)
	if err == nil {
		t.Error("expected error for invalid branch name")
	}
}

func TestRunAutoCleanup_WithExpiredOverlays(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	cfg.Overlay.AutoCleanupDays = 7

	// Create an expired, unmounted overlay
	ovl := &api.Overlay{
		Name:       "expired-overlay",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now().Add(-30 * 24 * time.Hour),
	}
	_ = store.Save(ovl)
	mock.mounted["expired-overlay"] = false

	runAutoCleanup()

	if store.Exists("expired-overlay") {
		t.Error("expired overlay should be auto-cleaned")
	}
}

func TestRunAutoCleanup_SkipsPersistent(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	cfg.Overlay.AutoCleanupDays = 7

	ovl := &api.Overlay{
		Name:       "persistent-old",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		Persistent: true,
		CreatedAt:  time.Now().Add(-30 * 24 * time.Hour),
	}
	_ = store.Save(ovl)
	mock.mounted["persistent-old"] = false

	runAutoCleanup()

	if !store.Exists("persistent-old") {
		t.Error("persistent overlay should not be auto-cleaned")
	}
}

func TestRunAutoCleanup_SkipsLocked(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	cfg.Overlay.AutoCleanupDays = 7

	ovl := &api.Overlay{
		Name:       "locked-old",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		Locked:     true,
		CreatedAt:  time.Now().Add(-30 * 24 * time.Hour),
	}
	_ = store.Save(ovl)
	mock.mounted["locked-old"] = false

	runAutoCleanup()

	if !store.Exists("locked-old") {
		t.Error("locked overlay should not be auto-cleaned")
	}
}

func TestRunAutoCleanup_SkipsMounted(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	cfg.Overlay.AutoCleanupDays = 7

	ovl := &api.Overlay{
		Name:       "mounted-old",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now().Add(-30 * 24 * time.Hour),
	}
	_ = store.Save(ovl)
	mock.mounted["mounted-old"] = true

	runAutoCleanup()

	if !store.Exists("mounted-old") {
		t.Error("mounted overlay should not be auto-cleaned")
	}
}

func TestDoConfigValidate_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	// Write a valid config file
	configPath := filepath.Join(tmpDir, "config.yaml")
	validConfig := `state_dir: "` + tmpDir + `"
logging:
  level: info
overlay:
  auto_cleanup_days: 7
agent:
  default_timeout_minutes: 60
`
	_ = os.WriteFile(configPath, []byte(validConfig), 0600)

	_, err := config.Load(configPath)
	if err != nil {
		t.Errorf("valid config should load: %v", err)
	}
}

func TestDoConfigValidate_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "bad-config.yaml")
	invalidConfig := `logging:
  level: "invalid-level"
`
	_ = os.WriteFile(configPath, []byte(invalidConfig), 0600)

	_, err := config.Load(configPath)
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestProcessStop_Unmounted(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "stop-unmounted",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)
	mock.mounted["stop-unmounted"] = false

	// Should succeed even if not mounted
	err := processStop(context.Background(), "stop-unmounted", false, false, false)
	if err != nil {
		t.Fatalf("processStop unmounted should succeed: %v", err)
	}
}

func TestProcessStop_CleanupDeletesState(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mock := setupMockManager(t)
	store := createTestStore(t, tmpDir)

	ovl := &api.Overlay{
		Name:       "stop-delete",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   filepath.Join(tmpDir, "upper"),
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)
	mock.mounted["stop-delete"] = false

	err := processStop(context.Background(), "stop-delete", true, false, false)
	if err != nil {
		t.Fatalf("processStop cleanup failed: %v", err)
	}

	if store.Exists("stop-delete") {
		t.Error("state should be deleted after cleanup")
	}
}
