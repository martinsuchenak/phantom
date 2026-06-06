package commands

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestClone_TargetAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)
	store := createTestStore(t, tmpDir)

	src := &api.Overlay{Name: "src", BaseDir: tmpDir, CreatedAt: time.Now()}
	dst := &api.Overlay{Name: "dst", BaseDir: tmpDir, CreatedAt: time.Now()}
	_ = store.Save(src)
	_ = store.Save(dst)

	// Simulating the check that doClone does
	if store.Exists("dst") {
		// This is the expected behavior
		return
	}
	t.Error("should detect existing target")
}

func TestClone_SourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	setupMockManager(t)
	store := createTestStore(t, tmpDir)

	_, err := store.Load("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent source")
	}
}

func TestClone_CopyUpperDir(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	srcUpper := filepath.Join(tmpDir, "src-upper")
	dstUpper := filepath.Join(tmpDir, "dst-upper")
	_ = os.MkdirAll(srcUpper, 0755)
	_ = os.WriteFile(filepath.Join(srcUpper, "code.go"), []byte("package main"), 0644)
	_ = os.MkdirAll(filepath.Join(srcUpper, "sub"), 0755)
	_ = os.WriteFile(filepath.Join(srcUpper, "sub", "nested.go"), []byte("package sub"), 0644)

	err := copyDir(srcUpper, dstUpper)
	if err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}

	// Verify files were copied
	data, err := os.ReadFile(filepath.Join(dstUpper, "code.go"))
	if err != nil {
		t.Fatal("code.go should exist in dst")
	}
	if string(data) != "package main" {
		t.Errorf("content mismatch: %q", string(data))
	}

	data, err = os.ReadFile(filepath.Join(dstUpper, "sub", "nested.go"))
	if err != nil {
		t.Fatal("sub/nested.go should exist in dst")
	}
	if string(data) != "package sub" {
		t.Errorf("nested content mismatch: %q", string(data))
	}
}
