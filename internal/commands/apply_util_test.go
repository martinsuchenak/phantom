package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile_Basic(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "dst.txt")
	os.WriteFile(src, []byte("hello world"), 0644)

	if err := copyFile(src, dst, 0644); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal("dst should exist")
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestCopyFile_CreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "deep", "nested", "dst.txt")
	os.WriteFile(src, []byte("nested"), 0644)

	if err := copyFile(src, dst, 0644); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "nested" {
		t.Errorf("expected 'nested', got %q", string(data))
	}
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	err := copyFile(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "dst"), 0644)
	if err == nil {
		t.Error("expected error for missing source")
	}
}

func TestApplyFileCopy_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	mountDir := filepath.Join(tmpDir, "mount")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(mountDir, 0755)

	// Add a new file in mount
	os.WriteFile(filepath.Join(mountDir, "new.go"), []byte("package main"), 0644)

	ovl := testOverlay("apply-dry", baseDir, mountDir, filepath.Join(tmpDir, "upper"))

	err := applyFileCopy(&ovl, true)
	if err != nil {
		t.Fatalf("applyFileCopy dry-run failed: %v", err)
	}

	// File should NOT be in base (dry run)
	if _, err := os.Stat(filepath.Join(baseDir, "new.go")); !os.IsNotExist(err) {
		t.Error("file should not be copied in dry run")
	}
}

func TestApplyFileCopy_CopiesNewFiles(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	mountDir := filepath.Join(tmpDir, "mount")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(mountDir, 0755)

	os.WriteFile(filepath.Join(mountDir, "added.go"), []byte("package added"), 0644)

	ovl := testOverlay("apply-copy", baseDir, mountDir, filepath.Join(tmpDir, "upper"))

	err := applyFileCopy(&ovl, false)
	if err != nil {
		t.Fatalf("applyFileCopy failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(baseDir, "added.go"))
	if err != nil {
		t.Fatal("added.go should be in base")
	}
	if string(data) != "package added" {
		t.Errorf("content mismatch: %q", string(data))
	}
}

func TestApplyFileCopy_DeletesRemovedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	mountDir := filepath.Join(tmpDir, "mount")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(mountDir, 0755)

	// File exists in base but not in mount (deleted)
	os.WriteFile(filepath.Join(baseDir, "removed.go"), []byte("old"), 0644)

	ovl := testOverlay("apply-del", baseDir, mountDir, filepath.Join(tmpDir, "upper"))

	err := applyFileCopy(&ovl, false)
	if err != nil {
		t.Fatalf("applyFileCopy failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(baseDir, "removed.go")); !os.IsNotExist(err) {
		t.Error("removed.go should be deleted from base")
	}
}
