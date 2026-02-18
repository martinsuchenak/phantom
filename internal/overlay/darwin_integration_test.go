//go:build darwin

package overlay

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestDarwinManager_Flow(t *testing.T) {
	// Swap execCommand
	oldExec := execCommand
	execCommand = fakeExecCommand
	defer func() { execCommand = oldExec }()

	tmpDir, err := os.MkdirTemp("", "phantom-ovl-integration-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	stateDir := filepath.Join(tmpDir, "state")

	// Prepare expected mount point for mocking IsMounted
	expectedMountPoint := filepath.Join(stateDir, "mnt", "test-ovl")
	os.Setenv("GO_TEST_MOUNTED_PATHS", expectedMountPoint)
	defer os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	manager, err := NewManager(stateDir, "/usr/local/bin/unionfs-fuse", []string{"cow"})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// 1. Create Overlay
	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)

	opts := &api.CreateOptions{
		Name:    "test-ovl",
		BaseDir: baseDir,
		Branch:  "main",
	}

	ovl, err := manager.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if ovl.Name != "test-ovl" {
		t.Errorf("Expected name test-ovl, got %s", ovl.Name)
	}

	// Verify directories created
	if _, err := os.Stat(ovl.MountPoint); os.IsNotExist(err) {
		t.Error("Mount point was not created")
	}
	if _, err := os.Stat(ovl.UpperDir); os.IsNotExist(err) {
		t.Error("Upper dir was not created")
	}

	// 2. Mount Overlay (Implicitly done by Create, but let's check state logic)
	// Since we mocked exec, the actual mount didn't happen on the OS.
	// But the code execution path was exercised.

	// 3. Check IsMounted
	// Our real implementation uses os.Stat or similar checks.
	// Since we didn't really mount, IsMounted might return false unless we mock that too.
	// DarwinManager.IsMounted checks mount entry or file existence?
	// It uses syscall.Statfs -> Not easily mocked without interface.
	// However, we can test Unmount logic.

	// 4. Unmount
	err = manager.Unmount(ovl)
	if err != nil {
		t.Errorf("Unmount failed: %v", err)
	}

	// 5. Cleanup
	err = manager.Cleanup(ovl)
	if err != nil {
		t.Errorf("Cleanup failed: %v", err)
	}

	// Verify dirs gone
	if _, err := os.Stat(ovl.MountPoint); !os.IsNotExist(err) {
		t.Error("Mount point should be deleted")
	}
}

func TestDarwinManager_Lifecycle(t *testing.T) {
	// Swap execCommand
	oldExec := execCommand
	execCommand = fakeExecCommand
	defer func() { execCommand = oldExec }()

	tmpDir, err := os.MkdirTemp("", "phantom-lifecycle-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	stateDir := filepath.Join(tmpDir, "state")

	// Prepare expected mount point
	expectedMountPoint := filepath.Join(stateDir, "mnt", "demo")
	os.Setenv("GO_TEST_MOUNTED_PATHS", expectedMountPoint)
	defer os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	manager, err := NewManager(stateDir, "unionfs-fuse", nil)
	if err != nil {
		t.Fatal(err)
	}

	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)

	// Create
	ovl, err := manager.Create(&api.CreateOptions{Name: "demo", BaseDir: baseDir})
	if err != nil {
		t.Fatal(err)
	}

	// Force Unmount
	err = manager.ForceUnmount(ovl)
	if err != nil {
		t.Errorf("ForceUnmount failed: %v", err)
	}
}
