//go:build darwin

package overlay

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

// execCommand is a variable that points to exec.Command.
// It can be replaced in tests to mock command execution without actually
// running external processes (unionfs-fuse, umount).
//
// Pattern: Function variable injection for testing
// This pattern is used instead of interface-based mocking because:
//   - exec.Command is a simple function call, not a complex interface
//   - Tests can easily override: execCommand = func(name string, args ...string) *exec.Cmd { ... }
//   - See exec_helper_test.go for mock implementation examples
//   - Alternative (interface-based) would add unnecessary complexity for this use case
//
// Example test usage:
//
//	execCommand = func(name string, args ...string) *exec.Cmd {
//	    return exec.Command("echo", "mocked")
//	}
//	defer func() { execCommand = exec.Command }()
var execCommand = exec.Command

// DarwinManager implements overlay filesystem management using unionfs-fuse
type DarwinManager struct {
	stateDir    string
	overlaysDir string
	mountDir    string
	unionfsPath string
	fuseOptions []string
}

// NewManager creates a new Darwin overlay manager
func NewManager(stateDir string, unionfsPath string, fuseOptions []string, _ bool) (*DarwinManager, error) {
	overlaysDir := filepath.Join(stateDir, "overlays")
	mountDir := filepath.Join(stateDir, "mnt")

	// Ensure directories exist
	for _, dir := range []string{overlaysDir, mountDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Auto-detect unionfs path if not specified
	if unionfsPath == "" {
		unionfsPath = findUnionFS()
	}

	if unionfsPath == "" {
		return nil, api.NewError(api.ErrFUSENotFound, "unionfs not found (install with: brew install unionfs-fuse)", nil)
	}

	// Default FUSE options
	if len(fuseOptions) == 0 {
		fuseOptions = []string{"cow"}
	}

	return &DarwinManager{
		stateDir:    stateDir,
		overlaysDir: overlaysDir,
		mountDir:    mountDir,
		unionfsPath: unionfsPath,
		fuseOptions: fuseOptions,
	}, nil
}

// findUnionFS attempts to locate unionfs binary
func findUnionFS() string {
	// Common paths to check - try both "unionfs" and "unionfs-fuse"
	paths := []string{
		"/usr/local/bin/unionfs",
		"/opt/homebrew/bin/unionfs",
		"/usr/bin/unionfs",
		"/usr/local/bin/unionfs-fuse",
		"/opt/homebrew/bin/unionfs-fuse",
		"/usr/bin/unionfs-fuse",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Check PATH - try unionfs first, then unionfs-fuse
	if path, err := exec.LookPath("unionfs"); err == nil {
		return path
	}
	if path, err := exec.LookPath("unionfs-fuse"); err == nil {
		return path
	}

	return ""
}

// Create creates and mounts a new overlay filesystem
func (m *DarwinManager) Create(opts *api.CreateOptions) (*api.Overlay, error) {
	// Validate base directory - check for symlinks
	info, err := os.Lstat(opts.BaseDir)
	if err != nil {
		return nil, api.NewError(api.ErrMountFailed, "base directory does not exist", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, api.NewError(api.ErrMountFailed, "base directory cannot be a symlink", nil)
	}
	if !info.IsDir() {
		return nil, api.NewError(api.ErrMountFailed, "base path is not a directory", nil)
	}

	// Create overlay directories
	overlayDir := filepath.Join(m.overlaysDir, opts.Name)
	upperDir := filepath.Join(overlayDir, "upper")
	mountPoint := filepath.Join(m.mountDir, opts.Name)

	for _, dir := range []string{upperDir, mountPoint} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, api.NewError(api.ErrMountFailed, fmt.Sprintf("failed to create %s", dir), err)
		}
	}

	overlay := &api.Overlay{
		Name:       opts.Name,
		BaseDir:    opts.BaseDir,
		MountPoint: mountPoint,
		UpperDir:   upperDir,
		Branch:     opts.Branch,
		Persistent: opts.Persistent,
		CreatedAt:  time.Now(),
	}

	// Mount the overlay
	if err := m.Mount(overlay); err != nil {
		// Cleanup on failure
		os.RemoveAll(overlayDir)
		os.RemoveAll(mountPoint)
		return nil, err
	}

	return overlay, nil
}

// Mount mounts an existing overlay
func (m *DarwinManager) Mount(overlay *api.Overlay) error {
	// Ensure mount point exists
	if err := os.MkdirAll(overlay.MountPoint, 0700); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to create mount point", err)
	}

	// Ensure upper directory exists
	if err := os.MkdirAll(overlay.UpperDir, 0700); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to create upper directory", err)
	}

	// Mount using unionfs-fuse
	// Syntax: unionfs-fuse -o cow,max_files=32768 -o allow_other,use_ino,suid,dev /upper=RW:/base=RO /mount-point

	// Prepare directories argument
	// precise order matters: upper directory (RW) first, then base directory (RO)
	dirs := fmt.Sprintf("%s=RW:%s=RO", overlay.UpperDir, overlay.BaseDir)

	args := []string{
		"-o", strings.Join(m.fuseOptions, ","), // Use configured options
		"-o", "allow_other,use_ino,suid,dev", // Standard options for overlay behavior
		dirs,
		overlay.MountPoint,
	}

	cmd := execCommand(m.unionfsPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start unionfs-fuse in background
	if err := cmd.Start(); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to start unionfs-fuse", err)
	}

	// Store PID for later management
	overlay.PID = cmd.Process.Pid

	// Poll for mount completion with timeout
	mounted := false
	for i := 0; i < 20; i++ { // 2 second timeout (20 * 100ms)
		time.Sleep(100 * time.Millisecond)
		if m, _ := m.IsMounted(overlay); m {
			mounted = true
			break
		}
	}

	if !mounted {
		cmd.Process.Kill()
		return api.NewError(api.ErrMountFailed, "mount verification failed after timeout", nil)
	}

	return nil
}

// Unmount unmounts an overlay
func (m *DarwinManager) Unmount(overlay *api.Overlay) error {
	// Check if mounted
	mounted, err := m.IsMounted(overlay)
	if err != nil {
		return err
	}
	if !mounted {
		return nil // Already unmounted
	}

	// Try normal unmount first
	cmd := execCommand("umount", overlay.MountPoint)
	if err := cmd.Run(); err != nil {
		// If normal unmount fails, try force unmount
		return m.ForceUnmount(overlay)
	}

	// Kill the unionfs-fuse process if we have its PID and it's verified
	m.killUnionFSProcess(overlay)

	return nil
}

// killUnionFSProcess safely kills the unionfs-fuse process after verifying it
func (m *DarwinManager) killUnionFSProcess(overlay *api.Overlay) {
	if overlay.PID <= 0 {
		return
	}

	// Verify the process is actually unionfs-fuse before killing
	if !m.isUnionFSProcess(overlay.PID) {
		overlay.PID = 0
		return
	}

	if process, err := os.FindProcess(overlay.PID); err == nil {
		process.Kill()
	}
	overlay.PID = 0
}

// isUnionFSProcess checks if a PID belongs to a unionfs-fuse process
func (m *DarwinManager) isUnionFSProcess(pid int) bool {
	// Use ps to check the process name
	cmd := execCommand("ps", "-p", strconv.Itoa(pid), "-o", "comm=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	procName := strings.TrimSpace(string(output))
	return strings.Contains(procName, "unionfs") || strings.Contains(procName, "fuse")
}

// IsMounted checks if an overlay is currently mounted
func (m *DarwinManager) IsMounted(overlay *api.Overlay) (bool, error) {
	// Use mount command to check if the mount point exists
	cmd := execCommand("mount")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	// Check if our mount point appears in mount output
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), overlay.MountPoint) {
			return true, nil
		}
	}

	return false, nil
}

// GetStatus returns the current status of an overlay
func (m *DarwinManager) GetStatus(overlay *api.Overlay) (*api.OverlayStatus, error) {
	mounted, err := m.IsMounted(overlay)
	if err != nil {
		return nil, err
	}

	status := &api.OverlayStatus{
		Name:    overlay.Name,
		Mounted: mounted,
		Uptime:  time.Since(overlay.CreatedAt),
	}

	if mounted {
		// Calculate size of upper directory
		var size int64
		filepath.Walk(overlay.UpperDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() {
				size += info.Size()
			}
			return nil
		})
		status.SizeBytes = size
	}

	return status, nil
}

// Cleanup removes all overlay data (upper dir, work dir)
func (m *DarwinManager) Cleanup(overlay *api.Overlay) error {
	// First unmount
	if err := m.Unmount(overlay); err != nil {
		return err
	}

	// Remove overlay directories
	overlayDir := filepath.Join(m.overlaysDir, overlay.Name)
	if err := os.RemoveAll(overlayDir); err != nil {
		return fmt.Errorf("failed to remove overlay data: %w", err)
	}

	// Remove mount point
	if err := os.RemoveAll(overlay.MountPoint); err != nil {
		return fmt.Errorf("failed to remove mount point: %w", err)
	}

	return nil
}

// Prune removes stale/unused overlay resources
func (m *DarwinManager) Prune() error {
	// Read all overlay directories
	entries, err := os.ReadDir(m.overlaysDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		mountPoint := filepath.Join(m.mountDir, name)

		// Check if mounted
		mounted, _ := m.IsMounted(&api.Overlay{MountPoint: mountPoint})
		if !mounted {
			// Not mounted, check if mount point is empty or doesn't exist
			if _, err := os.Stat(mountPoint); os.IsNotExist(err) {
				// Mount point doesn't exist, clean up overlay data
				overlayDir := filepath.Join(m.overlaysDir, name)
				os.RemoveAll(overlayDir)
			}
		}
	}

	return nil
}

// ForceUnmount forces unmount of a stuck overlay
func (m *DarwinManager) ForceUnmount(overlay *api.Overlay) error {
	// Use umount -f to force unmount
	cmd := execCommand("umount", "-f", overlay.MountPoint)
	if err := cmd.Run(); err != nil {
		return api.NewError(api.ErrUnmountFailed, "failed to force unmount overlay", err)
	}

	// Kill the unionfs-fuse process if we have its PID
	m.killUnionFSProcess(overlay)

	return nil
}
