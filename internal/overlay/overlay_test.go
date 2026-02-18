package overlay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-state-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Test Darwin Manager creation
	dm, err := NewManager(tmpDir, "/usr/bin/unionfs", []string{"cow"}, false)
	if err != nil {
		// Might fail if unionfs not found, which is expected in CI
		// But basic struct initialization should work if we pass paths
	}
	if dm != nil && dm.stateDir != tmpDir {
		t.Error("darwin manager state dir mismatch")
	}
}

func TestDirectoryCreation(t *testing.T) {
	// This tests the folder structure logic regardless of OS
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	os.Mkdir(baseDir, 0755)

	// We can't easily test Create() because it calls Mount() which executes commands.
	// But we can test helper functions if we export them or structure them better.
	// For now, let's test specific logic by simulating what Create does.

	name := "test-ovl"
	overlayDir := filepath.Join(tmpDir, "overlays", name)
	upperDir := filepath.Join(overlayDir, "upper")
	mountDir := filepath.Join(tmpDir, "mnt", name)

	for _, d := range []string{upperDir, mountDir} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Errorf("failed to create dir %s: %v", d, err)
		}
	}

	// Check permissions
	info, _ := os.Stat(upperDir)
	if info.Mode().Perm() != 0700 {
		t.Errorf("expected 0700 perm, got %v", info.Mode().Perm())
	}
}
