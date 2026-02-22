package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckPinDivergence_NotPinned(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	ovl := testOverlay("unpin-test", tmpDir, filepath.Join(tmpDir, "mnt"), filepath.Join(tmpDir, "upper"))
	// No pinned commit

	diverged, _, err := CheckPinDivergence(t.Context(), &ovl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diverged {
		t.Error("expected no divergence for unpinned overlay")
	}
}

func TestWriteIfNotExists_Nested(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	nested := filepath.Join(tmpDir, "a", "b", "c", "file.txt")
	os.MkdirAll(filepath.Dir(nested), 0755)
	err := writeIfNotExists(nested, "content", false)
	if err != nil {
		t.Fatalf("writeIfNotExists nested failed: %v", err)
	}
	data, _ := os.ReadFile(nested)
	if string(data) != "content" {
		t.Errorf("expected 'content', got %q", string(data))
	}
}
