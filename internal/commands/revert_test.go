package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestRevertCommand(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)

	// Create a base directory
	baseDir := filepath.Join(tmpDir, "base")
	_ = os.MkdirAll(baseDir, 0755)

	// Base files
	_ = os.WriteFile(filepath.Join(baseDir, "existing.txt"), []byte("base content"), 0644)
	_ = os.WriteFile(filepath.Join(baseDir, "deleted.txt"), []byte("to be deleted"), 0644)

	// Create an overlay
	ovlStore, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		t.Fatal(err)
	}
	ovl := &api.Overlay{
		Name:       "test-revert-ovl",
		BaseDir:    baseDir,
		UpperDir:   filepath.Join(tmpDir, "upper"),
		MountPoint: filepath.Join(tmpDir, "mnt"),
	}
	_ = os.MkdirAll(ovl.UpperDir, 0755)
	_ = os.MkdirAll(ovl.MountPoint, 0755)
	_ = ovlStore.Save(ovl)

	// Simulate overlay changes
	// 1. Modified file
	_ = os.WriteFile(filepath.Join(ovl.UpperDir, "existing.txt"), []byte("modified content"), 0644)
	// 2. Added file
	_ = os.WriteFile(filepath.Join(ovl.UpperDir, "new.txt"), []byte("new content"), 0644)
	// 3. Deleted file
	_ = os.WriteFile(filepath.Join(ovl.UpperDir, ".wh.deleted.txt"), []byte(""), 0644)

	t.Run("Revert modified file", func(t *testing.T) {
		err := processRevert("test-revert-ovl", "existing.txt")
		if err != nil {
			t.Fatalf("Expected revert to succeed, got: %v", err)
		}

		// The file should no longer exist in UpperDir
		if _, err := os.Stat(filepath.Join(ovl.UpperDir, "existing.txt")); !os.IsNotExist(err) {
			t.Errorf("UpperDir/existing.txt still exists after revert")
		}
	})

	t.Run("Revert added file", func(t *testing.T) {
		err := processRevert("test-revert-ovl", "new.txt")
		if err != nil {
			t.Fatalf("Expected revert to succeed, got: %v", err)
		}

		// The file should no longer exist in UpperDir
		if _, err := os.Stat(filepath.Join(ovl.UpperDir, "new.txt")); !os.IsNotExist(err) {
			t.Errorf("UpperDir/new.txt still exists after revert")
		}
	})

	t.Run("Revert deleted file", func(t *testing.T) {
		err := processRevert("test-revert-ovl", "deleted.txt")
		if err != nil {
			t.Fatalf("Expected revert to succeed, got: %v", err)
		}

		// The whiteout file should no longer exist in UpperDir
		if _, err := os.Stat(filepath.Join(ovl.UpperDir, ".wh.deleted.txt")); !os.IsNotExist(err) {
			t.Errorf("UpperDir/.wh.deleted.txt still exists after revert")
		}
	})

	t.Run("Revert missing file (no-op)", func(t *testing.T) {
		// Should succeed without error even if there's nothing to revert
		err := processRevert("test-revert-ovl", "missing.txt")
		if err != nil {
			t.Fatalf("Expected revert to succeed for missing file, got: %v", err)
		}
	})

	t.Run("Revert with invalid path escaping dir", func(t *testing.T) {
		err := processRevert("test-revert-ovl", "../escaped.txt")
		if err == nil {
			t.Fatalf("Expected revert to fail for path escaping directory")
		}
	})
}
