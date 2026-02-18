//go:build darwin

package overlay

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

// DarwinManager implements overlay filesystem management using unionfs-fuse
type DarwinManager struct {
	stateDir    string
	overlaysDir string
	mountDir    string
	unionfsPath string
	fuseOptions []string
}

// NewManager creates a new Darwin overlay manager
func NewManager(stateDir string, unionfsPath string, fuseOptions []string) (*DarwinManager, error) {
	overlaysDir := filepath.Join(stateDir, "overlays")
	mountDir := filepath.Join(stateDir, "mnt")

	// Ensure directories exist
	for _, dir := range []string{overlaysDir, mountDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Auto-detect unionfs-fuse path if not specified
	if unionfsPath == "" {
		unionfsPath = findUnionFS()
	}

	if unionfsPath == "" {
		return nil, api.NewError(api.ErrFUSENotFound, "unionfs-fuse not found in PATH", nil)
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

// findUnionFS attempts to locate unionfs-fuse binary
func findUnionFS() string {
	// Common paths to check
	paths := []string{
		"/usr/local/bin/unionfs-fuse",
		"/opt/homebrew/bin/unionfs-fuse",
		"/usr/bin/unionfs-fuse",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Check PATH
	if path, err := exec.LookPath("unionfs-fuse"); err == nil {
		return path
	}

	return ""
}

// Create creates and mounts a new overlay filesystem
func (m *DarwinManager) Create(opts *api.CreateOptions) (*api.Overlay, error) {
	// Validate base directory
	info, err := os.Stat(opts.BaseDir)
	if err != nil {
		return nil, api.NewError(api.ErrMountFailed, "base directory does not exist", err)
	}
	if !info.IsDir() {
		return nil, api.NewError(api.ErrMountFailed, "base path is not a directory", nil)
	}

	// Create overlay directories
	overlayDir := filepath.Join(m.overlaysDir, opts.Name)
	upperDir := filepath.Join(overlayDir, "upper")
	mountPoint := filepath.Join(m.mountDir, opts.Name)

	for _, dir := range []string{upperDir, mountPoint} {
		if err := os.MkdirAll(dir, 0755); err != nil {
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
	if err := os.MkdirAll(overlay.MountPoint, 0755); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to create mount point", err)
	}

	// Ensure upper directory exists
	if err := os.MkdirAll(overlay.UpperDir, 0755); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to create upper directory", err)
	}

	// Build unionfs command
	// unionfs-fuse -o cow upper=RW:lower=RO /mnt/point
	fuseOptStr := strings.Join(m.fuseOptions, ",")
	if fuseOptStr != "" {
		fuseOptStr = "-o," + fuseOptStr
	}

	// Union spec: upperdir=RW:lowerdir=RO
	unionSpec := fmt.Sprintf("%s=RW:%s=RO", overlay.UpperDir, overlay.BaseDir)

	args := []string{}
	if fuseOptStr != "" {
		args = append(args, strings.Split(fuseOptStr, ",")...)
	}
	args = append(args, unionSpec, overlay.MountPoint)

	cmd := exec.Command(m.unionfsPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start unionfs-fuse in background
	if err := cmd.Start(); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to start unionfs-fuse", err)
	}

	// Store PID for later management
	overlay.PID = cmd.Process.Pid

	// Wait a bit for mount to complete
	time.Sleep(100 * time.Millisecond)

	// Verify mount succeeded
	mounted, err := m.IsMounted(overlay)
	if err != nil {
		cmd.Process.Kill()
		return api.NewError(api.ErrMountFailed, "failed to verify mount", err)
	}
	if !mounted {
		cmd.Process.Kill()
		return api.NewError(api.ErrMountFailed, "mount verification failed", nil)
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

	// Use umount to unmount
	cmd := exec.Command("umount", overlay.MountPoint)
	if err := cmd.Run(); err != nil {
		// Try force unmount
		cmd = exec.Command("umount", "-f", overlay.MountPoint)
		if err := cmd.Run(); err != nil {
			return api.NewError(api.ErrUnmountFailed, "failed to unmount overlay", err)
		}
	}

	// Kill the unionfs-fuse process if we have its PID
	if overlay.PID > 0 {
		if process, err := os.FindProcess(overlay.PID); err == nil {
			process.Kill()
		}
		overlay.PID = 0
	}

	return nil
}

// IsMounted checks if an overlay is currently mounted
func (m *DarwinManager) IsMounted(overlay *api.Overlay) (bool, error) {
	// Use mount command to check if the mount point exists
	cmd := exec.Command("mount")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	// Check if our mount point appears in mount output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, overlay.MountPoint) {
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

// CheckDependencies verifies that required system capabilities are available
func CheckDependencies() error {
	// Check for unionfs-fuse
	unionfsPath := findUnionFS()
	if unionfsPath == "" {
		return fmt.Errorf("unionfs-fuse not found. Install with: brew install unionfs-fuse")
	}

	// Check for FUSE (macFUSE or FUSE-T)
	// Try to load the kernel extension or check if fuse-t is installed
	if _, err := exec.LookPath("fuse-t"); err == nil {
		// FUSE-T is installed
		return nil
	}

	// Check for macFUSE
	if _, err := os.Stat("/Library/Filesystems/macfuse.fs"); err == nil {
		return nil
	}

	return fmt.Errorf("no FUSE implementation found. Install macFUSE or FUSE-T")
}

// ForceUnmount forces unmount of a stuck overlay
func (m *DarwinManager) ForceUnmount(overlay *api.Overlay) error {
	// Use umount -f to force unmount
	cmd := exec.Command("umount", "-f", overlay.MountPoint)
	if err := cmd.Run(); err != nil {
		return api.NewError(api.ErrUnmountFailed, "failed to force unmount overlay", err)
	}

	// Kill the unionfs-fuse process if we have its PID
	if overlay.PID > 0 {
		if process, err := os.FindProcess(overlay.PID); err == nil {
			process.Kill()
		}
		overlay.PID = 0
	}

	return nil
}

// MountInfo returns information about a mount point
type MountInfo struct {
	Device     string
	MountPoint string
	FSType     string
	Options    string
}

// GetMounts returns all currently mounted FUSE filesystems
func GetMounts() ([]MountInfo, error) {
	cmd := exec.Command("mount")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var mounts []MountInfo
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Look for FUSE mounts (typically show as "macfuse" or "fuse-t")
		if strings.Contains(line, "fuse") || strings.Contains(line, "unionfs") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				mounts = append(mounts, MountInfo{
					Device:     parts[0],
					MountPoint: parts[2],
					FSType:     "fuse",
				})
			}
		}
	}

	return mounts, nil
}

// parsePIDFromFile reads a PID from a file (helper for persistent overlays)
func parsePIDFromFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// writePIDToFile writes a PID to a file (helper for persistent overlays)
func writePIDToFile(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
}
