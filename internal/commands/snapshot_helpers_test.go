package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDir(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src")
	dst := filepath.Join(tmpDir, "dst")

	// Create source structure
	os.MkdirAll(filepath.Join(src, "subdir"), 0755)
	os.WriteFile(filepath.Join(src, "file1.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(src, "subdir", "file2.txt"), []byte("world"), 0644)

	err := copyDir(src, dst)
	if err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}

	// Verify files were copied
	data, err := os.ReadFile(filepath.Join(dst, "file1.txt"))
	if err != nil {
		t.Fatalf("file1.txt not copied: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("file1.txt content = %q, want 'hello'", string(data))
	}

	data, err = os.ReadFile(filepath.Join(dst, "subdir", "file2.txt"))
	if err != nil {
		t.Fatalf("subdir/file2.txt not copied: %v", err)
	}
	if string(data) != "world" {
		t.Errorf("subdir/file2.txt content = %q, want 'world'", string(data))
	}
}

func TestCopyDirEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "empty-src")
	dst := filepath.Join(tmpDir, "empty-dst")
	os.MkdirAll(src, 0755)

	err := copyDir(src, dst)
	if err != nil {
		t.Fatalf("copyDir on empty dir failed: %v", err)
	}

	// dst should exist
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		t.Error("destination dir should exist")
	}
}

func TestCopyDirPreservesPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src")
	dst := filepath.Join(tmpDir, "dst")
	os.MkdirAll(src, 0755)

	os.WriteFile(filepath.Join(src, "script.sh"), []byte("#!/bin/sh\necho hi"), 0755)

	err := copyDir(src, dst)
	if err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}

	info, err := os.Stat(filepath.Join(dst, "script.sh"))
	if err != nil {
		t.Fatal(err)
	}
	// Check executable bit is preserved
	if info.Mode().Perm()&0100 == 0 {
		t.Errorf("expected executable permission, got %v", info.Mode().Perm())
	}
}

func TestDirSize(t *testing.T) {
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "b.txt"), []byte("world!"), 0644)

	size := dirSize(tmpDir)
	if size != 11 { // 5 + 6
		t.Errorf("expected size 11, got %d", size)
	}
}

func TestDirSizeEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	size := dirSize(tmpDir)
	if size != 0 {
		t.Errorf("expected size 0 for empty dir, got %d", size)
	}
}

func TestDirSizeNested(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "sub"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "sub", "b.txt"), []byte("bbbb"), 0644)

	size := dirSize(tmpDir)
	if size != 7 { // 3 + 4
		t.Errorf("expected size 7, got %d", size)
	}
}
