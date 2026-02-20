package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteIfNotExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-init-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	path := filepath.Join(tmpDir, "test.yaml")

	// First write should succeed
	err = writeIfNotExists(path, "content1", false)
	if err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "content1" {
		t.Errorf("expected 'content1', got %q", string(data))
	}

	// Second write without force should skip (not overwrite)
	err = writeIfNotExists(path, "content2", false)
	if err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	data, _ = os.ReadFile(path)
	if string(data) != "content1" {
		t.Errorf("expected 'content1' (not overwritten), got %q", string(data))
	}

	// Third write with force should overwrite
	err = writeIfNotExists(path, "content3", true)
	if err != nil {
		t.Fatalf("force write failed: %v", err)
	}

	data, _ = os.ReadFile(path)
	if string(data) != "content3" {
		t.Errorf("expected 'content3' (force overwritten), got %q", string(data))
	}
}

func TestWriteIfNotExists_Permissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-init-perm-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	path := filepath.Join(tmpDir, "secure.yaml")
	writeIfNotExists(path, "secret", false)

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %v", info.Mode().Perm())
	}
}
