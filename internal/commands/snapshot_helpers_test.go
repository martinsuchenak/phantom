package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-copydir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	src := filepath.Join(tmpDir, "src")
	dst := filepath.Join(tmpDir, "dst")

	// Create source structure
	os.MkdirAll(filepath.Join(src, "subdir"), 0755)
	os.WriteFile(filepath.Join(src, "file1.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(src, "subdir", "file2.txt"), []byte("world"), 0644)

	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}

	// Verify files copied
	data, err := os.ReadFile(filepath.Join(dst, "file1.txt"))
	if err != nil {
		t.Fatalf("file1.txt not copied: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("file1.txt content = %q, want %q", string(data), "hello")
	}

	data, err = os.ReadFile(filepath.Join(dst, "subdir", "file2.txt"))
	if err != nil {
		t.Fatalf("subdir/file2.txt not copied: %v", err)
	}
	if string(data) != "world" {
		t.Errorf("subdir/file2.txt content = %q, want %q", string(data), "world")
	}
}

func TestCopyDir_Empty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-copydir-empty-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	src := filepath.Join(tmpDir, "src")
	dst := filepath.Join(tmpDir, "dst")
	os.MkdirAll(src, 0755)

	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir empty failed: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("dst not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected dst to be a directory")
	}
}

func TestDirSize(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-dirsize-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("hello"), 0644) // 5 bytes
	os.WriteFile(filepath.Join(tmpDir, "b.txt"), []byte("world!"), 0644) // 6 bytes

	size := dirSize(tmpDir)
	if size != 11 {
		t.Errorf("dirSize = %d, want 11", size)
	}
}

func TestDirSize_Empty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-dirsize-empty-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	size := dirSize(tmpDir)
	if size != 0 {
		t.Errorf("dirSize of empty dir = %d, want 0", size)
	}
}

func TestDirSize_NonExistent(t *testing.T) {
	size := dirSize("/nonexistent/path/xyz")
	if size != 0 {
		t.Errorf("dirSize of nonexistent = %d, want 0", size)
	}
}
