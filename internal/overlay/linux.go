//go:build linux

package overlay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
	"golang.org/x/sys/unix"
)

// LinuxManager implements overlay filesystem management using native overlayfs
type LinuxManager struct {
	stateDir    string
	overlaysDir string
	mountDir    string
}

// NewManager creates a new Linux overlay manager
func NewManager(stateDir string) (*LinuxManager, error) {
	overlaysDir := filepath.Join(stateDir, "overlays")
	mountDir := filepath.Join(stateDir, "mnt")

	// Ensure directories exist
	for _, dir := range []string{overlaysDir, mountDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return &LinuxManager{
		stateDir:    stateDir,
		overlaysDir: overlaysDir,
		mountDir:    mountDir,
	}, nil
}

// Create creates and mounts a new overlay filesystem
func (m *LinuxManager) Create(opts *api.CreateOptions) (*api.Overlay, error) {
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
	workDir := filepath.Join(overlayDir, "work")
	mountPoint := filepath.Join(m.mountDir, opts.Name)

	for _, dir := range []string{upperDir, workDir, mountPoint} {
		if err := os.MkdirAll(dir, 0755); err != nil {
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
	if err := os.MkdirAll(overlay.MountPoint, 0755); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to create mount point", err)
	}

	// Ensure upper and work directories exist
	if err := os.MkdirAll(overlay.UpperDir, 0755); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to create upper directory", err)
	}
	if err := os.MkdirAll(overlay.WorkDir, 0755); err != nil {
		return api.NewError(api.ErrMountFailed, "failed to create work directory", err)
	}

	// Mount options for overlayfs
	mountOpts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		overlay.BaseDir, overlay.UpperDir, overlay.WorkDir)

	// Perform mount
	err := unix.Mount("overlay", overlay.MountPoint, "overlay", 0, mountOpts)
	if err != nil {
		return api.NewError(api.ErrMountFailed, "failed to mount overlay filesystem", err)
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

	// Unmount
	err = unix.Unmount(overlay.MountPoint, 0)
	if err != nil {
		// Try lazy unmount if regular unmount fails
		err = unix.Unmount(overlay.MountPoint, unix.MNT_DETACH)
		if err != nil {
			return api.NewError(api.ErrUnmountFailed, "failed to unmount overlay", err)
		}
	}

	return nil
}

// IsMounted checks if an overlay is currently mounted
func (m *LinuxManager) IsMounted(overlay *api.Overlay) (bool, error) {
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

// CheckDependencies verifies that required system capabilities are available
func CheckDependencies() error {
	// Check kernel support for overlayfs
	data, err := os.ReadFile("/proc/filesystems")
	if err != nil {
		return fmt.Errorf("cannot read /proc/filesystems: %w", err)
	}

	// Check for overlay filesystem support
	filesystems := string(data)
	if !strings.Contains(filesystems, "overlay") {
		return fmt.Errorf("overlay filesystem not supported by kernel")
	}

	return nil
}

// ForceUnmount uses lazy unmount to force unmount a stuck overlay
func (m *LinuxManager) ForceUnmount(overlay *api.Overlay) error {
	err := unix.Unmount(overlay.MountPoint, unix.MNT_DETACH)
	if err != nil {
		return api.NewError(api.ErrUnmountFailed, "failed to force unmount overlay", err)
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

// GetMounts returns all currently mounted overlays
func GetMounts() ([]MountInfo, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, err
	}

	var mounts []MountInfo
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 4 {
			continue
		}
		if parts[2] == "overlay" {
			mounts = append(mounts, MountInfo{
				Device:     parts[0],
				MountPoint: parts[1],
				FSType:     parts[2],
				Options:    parts[3],
			})
		}
	}

	return mounts, nil
}
