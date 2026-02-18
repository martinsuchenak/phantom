//go:build linux

package overlay

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
	"golang.org/x/sys/unix"
)

// execCommand is a variable that points to exec.Command, but can be mocked for testing
var execCommand = exec.Command

// LinuxManager implements overlay filesystem management using native overlayfs or fuse-overlayfs
type LinuxManager struct {
	stateDir        string
	overlaysDir     string
	mountDir        string
	useFuse         bool
	fuseOverlayPath string
	fuseOptions     []string
}

// NewManager creates a new Linux overlay manager
func NewManager(stateDir string, fuseOverlayPath string, fuseOptions []string, useFuse bool) (*LinuxManager, error) {
	overlaysDir := filepath.Join(stateDir, "overlays")
	mountDir := filepath.Join(stateDir, "mnt")

	// Ensure directories exist
	for _, dir := range []string{overlaysDir, mountDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Auto-detect: if not explicitly set, decide based on privileges
	if !useFuse {
		// Check if we're running as root
		if os.Geteuid() != 0 {
			// Not root - try to find fuse-overlayfs
			if fuseOverlayPath == "" {
				fuseOverlayPath = findFuseOverlay()
			}
			if fuseOverlayPath != "" {
				// Found fuse-overlayfs, use it
				useFuse = true
			}
			// If not found, will try native and fail with helpful error
		}
	}

	// If explicitly using fuse, find the binary
	if useFuse {
		if fuseOverlayPath == "" {
			fuseOverlayPath = findFuseOverlay()
		}
		if fuseOverlayPath == "" {
			return nil, api.NewError(api.ErrFUSENotFound, "fuse-overlayfs not found (required for non-root operation)", nil)
		}
	}

	return &LinuxManager{
		stateDir:        stateDir,
		overlaysDir:     overlaysDir,
		mountDir:        mountDir,
		useFuse:         useFuse,
		fuseOverlayPath: fuseOverlayPath,
		fuseOptions:     fuseOptions,
	}, nil
}

// findFuseOverlay attempts to locate fuse-overlayfs binary
func findFuseOverlay() string {
	// Common paths to check
	paths := []string{
		"/usr/bin/fuse-overlayfs",
		"/usr/local/bin/fuse-overlayfs",
		"/bin/fuse-overlayfs",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Check PATH
	if path, err := exec.LookPath("fuse-overlayfs"); err == nil {
		return path
	}

	return ""
}

// Create creates and mounts a new overlay filesystem
func (m *LinuxManager) Create(opts *api.CreateOptions) (*api.Overlay, error) {
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

	// Check for special characters in paths
	if !m.useFuse && strings.Contains(opts.BaseDir, ",") {
		return nil, api.NewError(api.ErrMountFailed, "base directory path cannot contain commas", nil)
	}

	// Create overlay directories
	overlayDir := filepath.Join(m.overlaysDir, opts.Name)
	upperDir := filepath.Join(overlayDir, "upper")
	workDir := filepath.Join(overlayDir, "work")
	mountPoint := filepath.Join(m.mountDir, opts.Name)

	for _, dir := range []string{upperDir, workDir, mountPoint} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, api.NewError(api.ErrMountFailed, fmt.Sprintf("failed to create %s", dir), err)
		}
	}

	overlay := &api.Overlay{
		Name:       opts.Name,
		BaseDir:    opts.BaseDir,
		MountPoint: mountPoint,
		UpperDir:   upperDir,
		WorkDir:    workDir,
		Branch:     opts.Branch,
		Persistent: opts.Persistent,
		CreatedAt:  time.Now(),
		UseFuse:    m.useFuse,
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
func (m *LinuxManager) Mount(overlay *api.Overlay) error {
	// Ensure mount point exists
	if err := os.MkdirAll(overlay.MountPoint, 0700); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to create mount point", err)
	}

	// Ensure upper and work directories exist
	if err := os.MkdirAll(overlay.UpperDir, 0700); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to create upper directory", err)
	}
	if err := os.MkdirAll(overlay.WorkDir, 0700); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to create work directory", err)
	}

	if m.useFuse {
		return m.mountFuse(overlay)
	}
	return m.mountNative(overlay)
}

// mountNative mounts using native kernel overlayfs
func (m *LinuxManager) mountNative(overlay *api.Overlay) error {
	// Check paths for commas to prevent option injection
	if strings.Contains(overlay.BaseDir, ",") || strings.Contains(overlay.UpperDir, ",") || strings.Contains(overlay.WorkDir, ",") {
		return api.NewError(api.ErrMountFailed, "overlay paths cannot contain commas", nil)
	}

	// Mount options for overlayfs
	mountOpts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		overlay.BaseDir, overlay.UpperDir, overlay.WorkDir)

	// Perform mount
	err := unix.Mount("overlay", overlay.MountPoint, "overlay", 0, mountOpts)
	if err != nil {
		return api.NewError(api.ErrMountFailed, "failed to mount overlay filesystem (try with sudo or enable fuse mode)", err)
	}

	return nil
}

// mountFuse mounts using fuse-overlayfs
func (m *LinuxManager) mountFuse(overlay *api.Overlay) error {
	// Build fuse-overlayfs command
	// fuse-overlayfs -o lowerdir=/lower,upperdir=/upper,workdir=/work /mountpoint
	args := []string{
		"-o", fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
			overlay.BaseDir, overlay.UpperDir, overlay.WorkDir),
	}

	// Add custom fuse options
	for _, opt := range m.fuseOptions {
		args = append(args, "-o", opt)
	}

	args = append(args, overlay.MountPoint)

	cmd := execCommand(m.fuseOverlayPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start fuse-overlayfs
	if err := cmd.Start(); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to start fuse-overlayfs", err)
	}

	// Store PID for later management
	overlay.PID = cmd.Process.Pid

	// Poll for mount completion with timeout
	mounted := false
	for i := 0; i < 20; i++ { // 2 second timeout (20 * 100ms)
		time.Sleep(100 * time.Millisecond)
		if isMounted, _ := m.IsMounted(overlay); isMounted {
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
func (m *LinuxManager) Unmount(overlay *api.Overlay) error {
	// Check if mounted
	mounted, err := m.IsMounted(overlay)
	if err != nil {
		return err
	}
	if !mounted {
		return nil // Already unmounted
	}

	// Use the overlay's UseFuse flag if set
	useFuse := overlay.UseFuse || m.useFuse
	if useFuse {
		return m.unmountFuse(overlay)
	}
	return m.unmountNative(overlay)
}

// unmountNative unmounts using native syscall
func (m *LinuxManager) unmountNative(overlay *api.Overlay) error {
	err := unix.Unmount(overlay.MountPoint, 0)
	if err != nil {
		// Try lazy unmount if regular unmount fails
		err = unix.Unmount(overlay.MountPoint, unix.MNT_DETACH)
		if err != nil {
			return api.NewError(api.ErrUnmountFailed, "failed to unmount overlay", err)
		}
	}
	return nil
}

// unmountFuse unmounts fuse-overlayfs
func (m *LinuxManager) unmountFuse(overlay *api.Overlay) error {
	// Try fusermount -u first (preferred for FUSE)
	cmd := execCommand("fusermount", "-u", overlay.MountPoint)
	if err := cmd.Run(); err != nil {
		// Fallback to regular umount
		cmd = execCommand("umount", overlay.MountPoint)
		if err := cmd.Run(); err != nil {
			return api.NewError(api.ErrUnmountFailed, "failed to unmount fuse overlay", err)
		}
	}

	// Kill the fuse-overlayfs process if we have its PID
	m.killFuseProcess(overlay)

	return nil
}

// killFuseProcess safely kills the fuse-overlayfs process after verifying it
func (m *LinuxManager) killFuseProcess(overlay *api.Overlay) {
	if overlay.PID <= 0 {
		return
	}

	// Verify the process is actually fuse-overlayfs before killing
	if !m.isFuseOverlayProcess(overlay.PID) {
		overlay.PID = 0
		return
	}

	if process, err := os.FindProcess(overlay.PID); err == nil {
		process.Kill()
	}
	overlay.PID = 0
}

// isFuseOverlayProcess checks if a PID belongs to a fuse-overlayfs process
func (m *LinuxManager) isFuseOverlayProcess(pid int) bool {
	// Read /proc/PID/comm to check the process name
	commPath := fmt.Sprintf("/proc/%d/comm", pid)
	data, err := os.ReadFile(commPath)
	if err != nil {
		return false
	}

	procName := strings.TrimSpace(string(data))
	return strings.Contains(procName, "fuse-overlay") || strings.Contains(procName, "fuse")
}

// IsMounted checks if an overlay is currently mounted
func (m *LinuxManager) IsMounted(overlay *api.Overlay) (bool, error) {
	// Use the overlay's UseFuse flag if set (for loaded overlays),
	// otherwise fall back to manager's setting (for new overlays)
	useFuse := overlay.UseFuse || m.useFuse
	if useFuse {
		return m.isMountedFuse(overlay)
	}
	return m.isMountedNative(overlay)
}

// isMountedNative checks mount status using statfs
func (m *LinuxManager) isMountedNative(overlay *api.Overlay) (bool, error) {
	var stat unix.Statfs_t
	err := unix.Statfs(overlay.MountPoint, &stat)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// Check if it's an overlay filesystem (type 0x794c7630)
	return stat.Type == unix.OVERLAYFS_SUPER_MAGIC, nil
}

// isMountedFuse checks mount status by reading /proc/mounts
func (m *LinuxManager) isMountedFuse(overlay *api.Overlay) (bool, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Clean the mount point path for comparison
	cleanMountPoint := filepath.Clean(overlay.MountPoint)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			mountPoint := fields[1]
			fsType := fields[2]
			// Check both the mount point and that it's a fuse overlay
			if filepath.Clean(mountPoint) == cleanMountPoint &&
				(fsType == "fuse.fuse-overlayfs" || fsType == "fuse" || strings.HasPrefix(fsType, "fuse.")) {
				return true, nil
			}
		}
	}

	return false, scanner.Err()
}

// GetStatus returns the current status of an overlay
func (m *LinuxManager) GetStatus(overlay *api.Overlay) (*api.OverlayStatus, error) {
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
func (m *LinuxManager) Cleanup(overlay *api.Overlay) error {
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
func (m *LinuxManager) Prune() error {
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
func (m *LinuxManager) ForceUnmount(overlay *api.Overlay) error {
	// Use the overlay's UseFuse flag if set
	useFuse := overlay.UseFuse || m.useFuse
	if useFuse {
		// Try fusermount -uz (lazy unmount)
		cmd := execCommand("fusermount", "-uz", overlay.MountPoint)
		if err := cmd.Run(); err != nil {
			// Fallback to umount -l
			cmd = execCommand("umount", "-l", overlay.MountPoint)
			if err := cmd.Run(); err != nil {
				return api.NewError(api.ErrUnmountFailed, "failed to force unmount fuse overlay", err)
			}
		}
		m.killFuseProcess(overlay)
		return nil
	}

	err := unix.Unmount(overlay.MountPoint, unix.MNT_DETACH)
	if err != nil {
		return api.NewError(api.ErrUnmountFailed, "failed to force unmount overlay", err)
	}
	return nil
}

// UseFuse returns whether fuse mode is enabled
func (m *LinuxManager) UseFuse() bool {
	return m.useFuse
}
