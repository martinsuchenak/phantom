package overlay

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/martinsuchenak/phantom/pkg/api"
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


func TestGetStatus(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-status-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Save original execCommand and restore after test
	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: filepath.Join(tmpDir, "overlays"),
		mountDir:    filepath.Join(tmpDir, "mnt"),
		unionfsPath: "unionfs-fuse",
		fuseOptions: []string{"cow"},
	}

	// Create upper dir with some files
	upperDir := filepath.Join(tmpDir, "overlays", "test", "upper")
	os.MkdirAll(upperDir, 0755)
	os.WriteFile(filepath.Join(upperDir, "file.txt"), []byte("hello"), 0644)

	mountPoint := filepath.Join(tmpDir, "mnt", "test")
	os.MkdirAll(mountPoint, 0755)

	overlay := &api.Overlay{
		Name:       "test",
		MountPoint: mountPoint,
		UpperDir:   upperDir,
	}

	// Set mounted paths env for the mock
	os.Setenv("GO_TEST_MOUNTED_PATHS", mountPoint)
	defer os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	status, err := dm.GetStatus(overlay)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.Name != "test" {
		t.Errorf("status name = %q, want %q", status.Name, "test")
	}
	if !status.Mounted {
		t.Error("expected mounted = true")
	}
	if status.SizeBytes != 5 {
		t.Errorf("size = %d, want 5", status.SizeBytes)
	}
}

func TestGetStatus_Unmounted(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-status-unmounted-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: filepath.Join(tmpDir, "overlays"),
		mountDir:    filepath.Join(tmpDir, "mnt"),
		unionfsPath: "unionfs-fuse",
		fuseOptions: []string{"cow"},
	}

	overlay := &api.Overlay{
		Name:       "test",
		MountPoint: "/nonexistent/mount",
		UpperDir:   "/nonexistent/upper",
	}

	// No mounted paths
	os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	status, err := dm.GetStatus(overlay)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.Mounted {
		t.Error("expected mounted = false")
	}
}

func TestIsMounted(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-ismounted-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: filepath.Join(tmpDir, "overlays"),
		mountDir:    filepath.Join(tmpDir, "mnt"),
		unionfsPath: "unionfs-fuse",
	}

	mountPoint := filepath.Join(tmpDir, "mnt", "test")
	overlay := &api.Overlay{MountPoint: mountPoint}

	// Not mounted
	os.Unsetenv("GO_TEST_MOUNTED_PATHS")
	mounted, err := dm.IsMounted(overlay)
	if err != nil {
		t.Fatalf("IsMounted failed: %v", err)
	}
	if mounted {
		t.Error("expected not mounted")
	}

	// Mounted
	os.Setenv("GO_TEST_MOUNTED_PATHS", mountPoint)
	defer os.Unsetenv("GO_TEST_MOUNTED_PATHS")
	mounted, err = dm.IsMounted(overlay)
	if err != nil {
		t.Fatalf("IsMounted failed: %v", err)
	}
	if !mounted {
		t.Error("expected mounted")
	}
}

func TestUnmount(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-unmount-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: filepath.Join(tmpDir, "overlays"),
		mountDir:    filepath.Join(tmpDir, "mnt"),
		unionfsPath: "unionfs-fuse",
	}

	mountPoint := filepath.Join(tmpDir, "mnt", "test")
	os.MkdirAll(mountPoint, 0755)

	overlay := &api.Overlay{
		Name:       "test",
		MountPoint: mountPoint,
		PID:        0,
	}

	// Set as mounted
	os.Setenv("GO_TEST_MOUNTED_PATHS", mountPoint)
	defer os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	err = dm.Unmount(overlay)
	if err != nil {
		t.Errorf("Unmount failed: %v", err)
	}
}

func TestUnmount_NotMounted(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-unmount-no-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	dm := &DarwinManager{
		stateDir: tmpDir,
	}

	overlay := &api.Overlay{MountPoint: "/nonexistent"}
	os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	err = dm.Unmount(overlay)
	if err != nil {
		t.Errorf("Unmount of not-mounted should succeed: %v", err)
	}
}

func TestCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-cleanup-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	overlaysDir := filepath.Join(tmpDir, "overlays")
	mountDir := filepath.Join(tmpDir, "mnt")
	os.MkdirAll(overlaysDir, 0755)
	os.MkdirAll(mountDir, 0755)

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: overlaysDir,
		mountDir:    mountDir,
		unionfsPath: "unionfs-fuse",
	}

	// Create overlay dirs
	overlayDir := filepath.Join(overlaysDir, "test")
	mountPoint := filepath.Join(mountDir, "test")
	os.MkdirAll(filepath.Join(overlayDir, "upper"), 0755)
	os.MkdirAll(mountPoint, 0755)

	overlay := &api.Overlay{
		Name:       "test",
		MountPoint: mountPoint,
		UpperDir:   filepath.Join(overlayDir, "upper"),
	}

	os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	err = dm.Cleanup(overlay)
	if err != nil {
		t.Errorf("Cleanup failed: %v", err)
	}

	if _, err := os.Stat(overlayDir); !os.IsNotExist(err) {
		t.Error("overlay dir should be removed")
	}
	if _, err := os.Stat(mountPoint); !os.IsNotExist(err) {
		t.Error("mount point should be removed")
	}
}

func TestPrune(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-prune-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	overlaysDir := filepath.Join(tmpDir, "overlays")
	mountDir := filepath.Join(tmpDir, "mnt")
	os.MkdirAll(overlaysDir, 0755)
	os.MkdirAll(mountDir, 0755)

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: overlaysDir,
		mountDir:    mountDir,
		unionfsPath: "unionfs-fuse",
	}

	// Create an overlay dir with no mount point (orphan)
	os.MkdirAll(filepath.Join(overlaysDir, "orphan"), 0755)

	os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	err = dm.Prune()
	if err != nil {
		t.Errorf("Prune failed: %v", err)
	}

	// Orphan should be cleaned up
	if _, err := os.Stat(filepath.Join(overlaysDir, "orphan")); !os.IsNotExist(err) {
		t.Error("orphan overlay dir should be removed")
	}
}

func TestForceUnmount(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-force-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	dm := &DarwinManager{
		stateDir: tmpDir,
	}

	overlay := &api.Overlay{
		MountPoint: filepath.Join(tmpDir, "mnt"),
		PID:        0,
	}

	err = dm.ForceUnmount(overlay)
	if err != nil {
		t.Errorf("ForceUnmount failed: %v", err)
	}
}


func TestCreate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-create-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	overlaysDir := filepath.Join(tmpDir, "overlays")
	mountDir := filepath.Join(tmpDir, "mnt")
	os.MkdirAll(overlaysDir, 0755)
	os.MkdirAll(mountDir, 0755)

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: overlaysDir,
		mountDir:    mountDir,
		unionfsPath: "unionfs-fuse",
		fuseOptions: []string{"cow"},
	}

	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)

	// Set mount point as mounted for verification
	mountPoint := filepath.Join(mountDir, "test-create")
	os.Setenv("GO_TEST_MOUNTED_PATHS", mountPoint)
	defer os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	opts := &api.CreateOptions{
		Name:    "test-create",
		BaseDir: baseDir,
		Branch:  "test-branch",
	}

	ovl, err := dm.Create(opts)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if ovl.Name != "test-create" {
		t.Errorf("name = %q, want %q", ovl.Name, "test-create")
	}
	if ovl.Branch != "test-branch" {
		t.Errorf("branch = %q, want %q", ovl.Branch, "test-branch")
	}
}

func TestCreate_InvalidBaseDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-create-bad-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: filepath.Join(tmpDir, "overlays"),
		mountDir:    filepath.Join(tmpDir, "mnt"),
		unionfsPath: "unionfs-fuse",
	}

	opts := &api.CreateOptions{
		Name:    "test",
		BaseDir: "/nonexistent/base",
	}

	_, err = dm.Create(opts)
	if err == nil {
		t.Error("expected error for non-existent base dir")
	}
}

func TestCreate_BaseDirIsFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-create-file-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: filepath.Join(tmpDir, "overlays"),
		mountDir:    filepath.Join(tmpDir, "mnt"),
		unionfsPath: "unionfs-fuse",
	}

	filePath := filepath.Join(tmpDir, "notadir")
	os.WriteFile(filePath, []byte("file"), 0644)

	opts := &api.CreateOptions{
		Name:    "test",
		BaseDir: filePath,
	}

	_, err = dm.Create(opts)
	if err == nil {
		t.Error("expected error for file as base dir")
	}
}

func TestKillUnionFSProcess_NoPID(t *testing.T) {
	dm := &DarwinManager{}
	overlay := &api.Overlay{PID: 0}
	dm.killUnionFSProcess(overlay) // should be no-op
}

func TestKillUnionFSProcess_WithPID(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	dm := &DarwinManager{}
	overlay := &api.Overlay{PID: 99999}
	dm.killUnionFSProcess(overlay)
	// PID should be reset since the mock ps won't find a unionfs process
	if overlay.PID != 0 {
		t.Errorf("PID should be reset to 0, got %d", overlay.PID)
	}
}

func TestIsUnionFSProcess(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	dm := &DarwinManager{}
	// Mock ps won't return unionfs for arbitrary PIDs
	result := dm.isUnionFSProcess(99999)
	if result {
		t.Error("expected false for non-unionfs process")
	}
}

func TestFindUnionFS(t *testing.T) {
	// Just verify it doesn't panic — result depends on system
	_ = findUnionFS()
}

func TestNewManager_NoUnionFS(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-nounion-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Pass empty unionfs path and override PATH to empty dir
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", filepath.Join(tmpDir, "emptybin"))
	os.MkdirAll(filepath.Join(tmpDir, "emptybin"), 0755)
	defer os.Setenv("PATH", origPath)

	_, err = NewManager(tmpDir, "", []string{"cow"}, false)
	// On systems with unionfs installed in standard paths, this may succeed
	// Just verify it doesn't panic
	_ = err
}

func TestNewManager_WithPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-withpath-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a fake unionfs binary
	fakeBin := filepath.Join(tmpDir, "unionfs-fuse")
	os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755)

	dm, err := NewManager(tmpDir, fakeBin, nil, false)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	if dm.unionfsPath != fakeBin {
		t.Errorf("unionfsPath = %q, want %q", dm.unionfsPath, fakeBin)
	}
	// Default fuse options should be set
	if len(dm.fuseOptions) != 1 || dm.fuseOptions[0] != "cow" {
		t.Errorf("fuseOptions = %v, want [cow]", dm.fuseOptions)
	}
}


func TestCreate_SymlinkBaseDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-symlink-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: filepath.Join(tmpDir, "overlays"),
		mountDir:    filepath.Join(tmpDir, "mnt"),
		unionfsPath: "unionfs-fuse",
		fuseOptions: []string{"cow"},
	}
	os.MkdirAll(dm.overlaysDir, 0755)
	os.MkdirAll(dm.mountDir, 0755)

	// Create a real dir and a symlink to it
	realDir := filepath.Join(tmpDir, "real")
	os.MkdirAll(realDir, 0755)
	linkDir := filepath.Join(tmpDir, "link")
	os.Symlink(realDir, linkDir)

	opts := &api.CreateOptions{
		Name:    "test",
		BaseDir: linkDir,
	}

	_, err = dm.Create(opts)
	if err == nil {
		t.Error("expected error for symlink base dir")
	}
}

func TestPrune_WithMountedOverlay(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-prune-mounted-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	overlaysDir := filepath.Join(tmpDir, "overlays")
	mountDir := filepath.Join(tmpDir, "mnt")
	os.MkdirAll(overlaysDir, 0755)
	os.MkdirAll(mountDir, 0755)

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: overlaysDir,
		mountDir:    mountDir,
		unionfsPath: "unionfs-fuse",
	}

	// Create overlay dir with existing mount point (mounted)
	os.MkdirAll(filepath.Join(overlaysDir, "mounted-ovl"), 0755)
	mountPoint := filepath.Join(mountDir, "mounted-ovl")
	os.MkdirAll(mountPoint, 0755)

	// Set as mounted
	os.Setenv("GO_TEST_MOUNTED_PATHS", mountPoint)
	defer os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	err = dm.Prune()
	if err != nil {
		t.Errorf("Prune failed: %v", err)
	}

	// Mounted overlay should NOT be pruned
	if _, err := os.Stat(filepath.Join(overlaysDir, "mounted-ovl")); os.IsNotExist(err) {
		t.Error("mounted overlay should not be pruned")
	}
}

func TestPrune_NonExistentDir(t *testing.T) {
	dm := &DarwinManager{
		overlaysDir: "/nonexistent/overlays",
		mountDir:    "/nonexistent/mnt",
	}

	err := dm.Prune()
	if err != nil {
		t.Errorf("Prune with nonexistent dir should not error: %v", err)
	}
}

func TestCleanup_WithMounted(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-cleanup-mounted-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	overlaysDir := filepath.Join(tmpDir, "overlays")
	mountDir := filepath.Join(tmpDir, "mnt")
	os.MkdirAll(overlaysDir, 0755)
	os.MkdirAll(mountDir, 0755)

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: overlaysDir,
		mountDir:    mountDir,
		unionfsPath: "unionfs-fuse",
	}

	overlayDir := filepath.Join(overlaysDir, "test")
	mountPoint := filepath.Join(mountDir, "test")
	os.MkdirAll(filepath.Join(overlayDir, "upper"), 0755)
	os.MkdirAll(mountPoint, 0755)

	overlay := &api.Overlay{
		Name:       "test",
		MountPoint: mountPoint,
		UpperDir:   filepath.Join(overlayDir, "upper"),
		PID:        0,
	}

	// Set as mounted — Cleanup should unmount first
	os.Setenv("GO_TEST_MOUNTED_PATHS", mountPoint)
	defer os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	err = dm.Cleanup(overlay)
	if err != nil {
		t.Errorf("Cleanup with mounted overlay failed: %v", err)
	}
}


func TestKillUnionFSProcess_ValidPID(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	dm := &DarwinManager{}
	// Use a PID that the mock ps will return empty for (not unionfs)
	overlay := &api.Overlay{PID: 12345}
	dm.killUnionFSProcess(overlay)
	// Should reset PID since mock ps returns empty (not unionfs)
	if overlay.PID != 0 {
		t.Errorf("PID should be 0 after kill attempt, got %d", overlay.PID)
	}
}

func TestMount_MountPointCreation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-mount-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: filepath.Join(tmpDir, "overlays"),
		mountDir:    filepath.Join(tmpDir, "mnt"),
		unionfsPath: "unionfs-fuse",
		fuseOptions: []string{"cow"},
	}

	mountPoint := filepath.Join(tmpDir, "mnt", "test-mount")
	upperDir := filepath.Join(tmpDir, "overlays", "test-mount", "upper")
	os.MkdirAll(upperDir, 0755)

	overlay := &api.Overlay{
		Name:       "test-mount",
		MountPoint: mountPoint,
		UpperDir:   upperDir,
		BaseDir:    tmpDir,
	}

	// Set as mounted for verification
	os.Setenv("GO_TEST_MOUNTED_PATHS", mountPoint)
	defer os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	err = dm.Mount(overlay)
	if err != nil {
		t.Errorf("Mount failed: %v", err)
	}

	// Mount point should be created
	if _, err := os.Stat(mountPoint); os.IsNotExist(err) {
		t.Error("mount point should be created")
	}
}


func TestCleanup_AlreadyCleaned(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-overlay-cleanup-clean-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origExec := execCommand
	defer func() { execCommand = origExec }()
	execCommand = fakeExecCommand

	dm := &DarwinManager{
		stateDir:    tmpDir,
		overlaysDir: filepath.Join(tmpDir, "overlays"),
		mountDir:    filepath.Join(tmpDir, "mnt"),
		unionfsPath: "unionfs-fuse",
	}
	os.MkdirAll(dm.overlaysDir, 0755)
	os.MkdirAll(dm.mountDir, 0755)

	// Overlay with dirs that don't exist (already cleaned)
	overlay := &api.Overlay{
		Name:       "gone",
		MountPoint: filepath.Join(dm.mountDir, "gone"),
		UpperDir:   filepath.Join(dm.overlaysDir, "gone", "upper"),
	}

	os.Unsetenv("GO_TEST_MOUNTED_PATHS")

	err = dm.Cleanup(overlay)
	if err != nil {
		t.Errorf("Cleanup of already-cleaned overlay should succeed: %v", err)
	}
}

func TestFindUnionFS_WithMockPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-findunionfs-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a fake unionfs binary in a temp dir
	fakeBin := filepath.Join(tmpDir, "unionfs")
	os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+origPath)
	defer os.Setenv("PATH", origPath)

	result := findUnionFS()
	if result == "" {
		t.Error("expected to find unionfs in PATH")
	}
}
