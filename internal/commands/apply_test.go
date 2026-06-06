package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestCopyFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-copyfile-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "subdir", "dst.txt")

	_ = os.WriteFile(src, []byte("hello world"), 0644)

	err = copyFile(src, dst, 0644)
	if err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read dst: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("content = %q, want %q", string(data), "hello world")
	}
}

func TestCopyFile_NonExistentSrc(t *testing.T) {
	tmpDir := t.TempDir()
	err := copyFile(filepath.Join(tmpDir, "nope.txt"), filepath.Join(tmpDir, "dst.txt"), 0644)
	if err == nil {
		t.Error("expected error for non-existent source")
	}
}

func TestApplyFileCopy(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-apply-fc-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	setupTestEnv(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	mountDir := filepath.Join(tmpDir, "mount")
	_ = os.MkdirAll(baseDir, 0755)
	_ = os.MkdirAll(mountDir, 0755)

	// Base has existing file
	_ = os.WriteFile(filepath.Join(baseDir, "existing.txt"), []byte("old"), 0644)
	// Mount has modified version + new file
	_ = os.WriteFile(filepath.Join(mountDir, "existing.txt"), []byte("new"), 0644)
	_ = os.WriteFile(filepath.Join(mountDir, "added.txt"), []byte("added"), 0644)

	ovl := &api.Overlay{
		Name:       "test-apply",
		BaseDir:    baseDir,
		MountPoint: mountDir,
	}

	// Dry run
	err = applyFileCopy(ovl, true)
	if err != nil {
		t.Errorf("applyFileCopy dry-run failed: %v", err)
	}

	// Real run
	err = applyFileCopy(ovl, false)
	if err != nil {
		t.Errorf("applyFileCopy failed: %v", err)
	}

	// Verify added file was copied
	data, err := os.ReadFile(filepath.Join(baseDir, "added.txt"))
	if err != nil {
		t.Fatalf("added.txt not copied: %v", err)
	}
	if string(data) != "added" {
		t.Errorf("added.txt content = %q, want %q", string(data), "added")
	}
}

func TestApplyFileCopy_Deletions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-apply-del-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	setupTestEnv(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	mountDir := filepath.Join(tmpDir, "mount")
	_ = os.MkdirAll(baseDir, 0755)
	_ = os.MkdirAll(mountDir, 0755)

	// Base has a file that's not in mount (deleted)
	_ = os.WriteFile(filepath.Join(baseDir, "deleted.txt"), []byte("gone"), 0644)
	// Mount has a file that's also in base
	_ = os.WriteFile(filepath.Join(baseDir, "kept.txt"), []byte("keep"), 0644)
	_ = os.WriteFile(filepath.Join(mountDir, "kept.txt"), []byte("keep"), 0644)

	ovl := &api.Overlay{
		Name:       "test-apply-del",
		BaseDir:    baseDir,
		MountPoint: mountDir,
	}

	err = applyFileCopy(ovl, false)
	if err != nil {
		t.Errorf("applyFileCopy failed: %v", err)
	}

	// deleted.txt should be removed from base
	if _, err := os.Stat(filepath.Join(baseDir, "deleted.txt")); !os.IsNotExist(err) {
		t.Error("deleted.txt should have been removed from base")
	}
}

func TestApplyFileCopy_NoMountPoint(t *testing.T) {
	setupTestEnv(t, t.TempDir())

	ovl := &api.Overlay{Name: "test", MountPoint: ""}
	err := applyFileCopy(ovl, false)
	if err == nil {
		t.Error("expected error for empty mount point")
	}
}

func TestApplyGit(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	oldLog := log
	log = &MockLogger{}
	defer func() {
		log = oldLog
	}()

	baseDir := filepath.Join(tmpDir, "base")
	_ = os.MkdirAll(baseDir, 0755)

	initGitRepo(t, baseDir)

	runGit(t, baseDir, "checkout", "-b", "phantom/overlay")

	// Make a change in the overlay
	_ = os.WriteFile(filepath.Join(baseDir, "overlay.txt"), []byte("overlay change"), 0644)

	ovl := &api.Overlay{
		Name:       "overlay",
		BaseDir:    baseDir,
		MountPoint: baseDir,
		Branch:     "phantom/overlay",
	}

	gitOps := git.NewOperations()

	// Dry run
	err := applyGit(context.Background(), ovl, gitOps, true)
	if err != nil {
		t.Fatalf("applyGit dryRun failed: %v", err)
	}

	// Normal apply
	runGit(t, baseDir, "checkout", "main")
	err = applyGit(context.Background(), ovl, gitOps, false)
	if err != nil {
		t.Fatalf("applyGit failed: %v", err)
	}
}
