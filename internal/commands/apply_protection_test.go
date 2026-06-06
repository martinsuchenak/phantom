package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestApplyProtection(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	mockMgr := setupMockManager(t)

	// Create a base directory
	baseDir := filepath.Join(tmpDir, "base")
	_ = os.MkdirAll(baseDir, 0755)

	protectedFile := filepath.Join(baseDir, "protected.txt")
	_ = os.WriteFile(protectedFile, []byte("original content"), 0644)

	// Create .phantomignore
	_ = os.WriteFile(filepath.Join(baseDir, ".phantomignore"), []byte("protected.txt\nconfig/"), 0644)

	// Create an overlay
	ovlStore, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		t.Fatal(err)
	}
	ovl := &api.Overlay{
		Name:       "test-ovl",
		BaseDir:    baseDir,
		UpperDir:   filepath.Join(tmpDir, "upper"),
		MountPoint: filepath.Join(tmpDir, "mnt"),
	}
	_ = os.MkdirAll(ovl.UpperDir, 0755)
	_ = os.MkdirAll(ovl.MountPoint, 0755)
	_ = ovlStore.Save(ovl)
	mockMgr.mounted[ovl.Name] = true

	t.Run("Modifying protected file should fail apply", func(t *testing.T) {
		// Simulate modification in upper dir
		_ = os.WriteFile(filepath.Join(ovl.UpperDir, "protected.txt"), []byte("hacked content"), 0644)

		err := processApply(context.Background(), "test-ovl", false, false, false)
		if err == nil {
			t.Fatal("Expected apply to fail due to protected path violation")
		}
		if !strings.Contains(err.Error(), "overlay attempts to modify protected file: protected.txt") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("Modifying protected directory should fail apply", func(t *testing.T) {
		// Simulate modification in a protected directory
		_ = os.MkdirAll(filepath.Join(ovl.UpperDir, "config"), 0755)
		_ = os.WriteFile(filepath.Join(ovl.UpperDir, "config", "settings.json"), []byte("{}"), 0644)

		err := processApply(context.Background(), "test-ovl", false, false, false)
		if err == nil {
			t.Fatal("Expected apply to fail due to protected directory violation")
		}
		if !strings.Contains(err.Error(), "protected file: config/settings.json") && !strings.Contains(err.Error(), "protected directory: config") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("Deleting protected file should fail apply", func(t *testing.T) {
		// Simulate deletion (whiteout) in upper dir
		_ = os.WriteFile(filepath.Join(ovl.UpperDir, ".wh.protected.txt"), []byte(""), 0644)

		err := processApply(context.Background(), "test-ovl", false, false, false)
		if err == nil {
			t.Fatal("Expected apply to fail due to protected path deletion")
		}
		if !strings.Contains(err.Error(), "overlay attempts to delete protected path: protected.txt") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("Modifying non-protected file should succeed", func(t *testing.T) {
		// Cleanup upper dir first
		_ = os.RemoveAll(ovl.UpperDir)
		_ = os.MkdirAll(ovl.UpperDir, 0755)

		_ = os.WriteFile(filepath.Join(ovl.UpperDir, "safe.txt"), []byte("new file"), 0644)

		// Map safe.txt to mount point so applyFileCopy sees it (processApply calls applyFileCopy which walks MountPoint, NOT UpperDir for non-git)
		_ = os.WriteFile(filepath.Join(ovl.MountPoint, "safe.txt"), []byte("new file"), 0644)

		err := processApply(context.Background(), "test-ovl", false, false, false)
		if err != nil {
			t.Fatalf("Expected apply to succeed for non-protected path, got: %v", err)
		}

		// Verify file was copied
		if _, err := os.Stat(filepath.Join(baseDir, "safe.txt")); err != nil {
			t.Errorf("safe.txt was not copied to base: %v", err)
		}
	})
}
